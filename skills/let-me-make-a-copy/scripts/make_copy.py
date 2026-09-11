#!/usr/bin/env python3
"""Standalone ViceMe Website Replica buyer workflow (Python 3.9+)."""

from __future__ import annotations

import argparse
import base64
import csv
import datetime as dt
import hashlib
import json
import os
import re
import secrets
import shutil
import stat
import struct
import zlib
import subprocess
import sys
import time
import unicodedata
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zipfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable, Dict, Iterable, Optional, Tuple

MAX_ARCHIVE_BYTES = 100 * 1024 * 1024
MAX_EXPANDED_BYTES = 500 * 1024 * 1024
MAX_FILE_BYTES = 100 * 1024 * 1024
MAX_FILE_COUNT = 10_000
MAX_ENTRY_COUNT = 20_000
MAX_PATH_BYTES = 4096
MAX_PATH_DEPTH = 128
MAX_SEGMENT_BYTES = 255
MAX_COMPRESSION_RATIO = 100
MAX_GUIDE_BYTES = 256 * 1024
LICENSE_SCHEMA = "website-replica-license/v2"
LICENSE_TYPE = "viceme-replica-license+jws"
INSTRUCTION_PATTERN = re.compile(r"VICEME-REPLICA:VMR-[A-Z0-9]{20}")
UUID_PATTERN = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$",
    re.IGNORECASE,
)
SECRET_PATTERN = re.compile(r"^[A-Za-z0-9_-]{43}$")
WINDOWS_RESERVED = re.compile(
    r"^(?:con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\..*)?$", re.IGNORECASE
)
ED25519_SPKI_PREFIX = bytes.fromhex("302a300506032b6570032100")


# Generated from the canonical CLI widgets by make trial-runtime.
PAYMENT_RESOURCE_SHA256 = {"payment.html": "59999b6d68ae84941b782b1aaf1a9194feb67f5891c8051d99b8cdded5c4c3b1", "qrcodegen.py": "b0df257ae06c83f79ac8fa408f5ae635f44a2ae0702e2fdbf6f2fe32cff33b05"}  # generated-payment-resources


class WorkflowError(Exception):
    def __init__(
        self,
        code: str,
        message: str,
        details: Optional[Dict[str, Any]] = None,
        exit_code: int = 1,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.message = message
        self.details = details or {}
        self.exit_code = exit_code


@dataclass(frozen=True)
class Authority:
    work_url: str
    web_origin: str
    api_base_url: str


@dataclass(frozen=True)
class HttpResponse:
    status: int
    body: bytes


RequestFn = Callable[..., HttpResponse]


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):  # noqa: ANN001
        return None


def _json_output(value: Dict[str, Any]) -> None:
    sys.stdout.write(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def result(data: Dict[str, Any]) -> None:
    _json_output({"ok": True, "data": data})


def fail(error: Exception) -> int:
    normalized = (
        error
        if isinstance(error, WorkflowError)
        else WorkflowError(
            "MAKE_COPY_INTERNAL", "The let-me-make-a-copy workflow failed"
        )
    )
    payload: Dict[str, Any] = {
        "ok": False,
        "error": {"code": normalized.code, "message": normalized.message},
    }
    if normalized.details:
        payload["error"]["details"] = normalized.details
    _json_output(payload)
    return normalized.exit_code


def authority_for_work_url(raw: str) -> Authority:
    try:
        parsed = urllib.parse.urlsplit(raw)
        port = parsed.port
    except (TypeError, ValueError):
        raise WorkflowError("MAKE_COPY_WORK_URL_INVALID", "Work URL is invalid")
    host = (parsed.hostname or "").lower()
    if (
        parsed.scheme != "https"
        or not parsed.path.endswith(".md")
        or parsed.username
        or parsed.password
        or port is not None
        or host not in {"viceme.cn", "www.viceme.cn", "viceme.ai", "www.viceme.ai"}
    ):
        raise WorkflowError(
            "MAKE_COPY_WORK_URL_INVALID",
            "Work URL must be an official ViceMe HTTPS .md URL",
        )
    canonical_host = "viceme.ai" if host.endswith("viceme.ai") else "viceme.cn"
    work_url = urllib.parse.urlunsplit(
        (parsed.scheme, parsed.netloc, parsed.path, parsed.query, "")
    )
    return Authority(
        work_url=work_url,
        web_origin="https://" + canonical_host,
        api_base_url="https://" + canonical_host + "/api/v1",
    )


def http_request(
    method: str,
    url: str,
    *,
    headers: Optional[Dict[str, str]] = None,
    body: Optional[bytes] = None,
    timeout: float = 20,
) -> HttpResponse:
    request = urllib.request.Request(
        url, data=body, headers=headers or {}, method=method
    )
    opener = urllib.request.build_opener(_NoRedirect())
    try:
        with opener.open(request, timeout=timeout) as response:
            return HttpResponse(response.status, response.read())
    except urllib.error.HTTPError as error:
        return HttpResponse(error.code, error.read())


def fetch_public_work_entry(
    authority: Authority, request_fn: RequestFn = http_request
) -> Tuple[str, bool]:
    parsed = urllib.parse.urlsplit(authority.work_url)
    segments = [
        urllib.parse.unquote(value) for value in parsed.path.split("/") if value
    ]
    if len(segments) == 3 and segments[0] in {"zh-CN", "en-US"}:
        segments = segments[1:]
    if len(segments) != 2 or not segments[1].endswith(".md"):
        raise WorkflowError("MAKE_COPY_WORK_URL_INVALID", "Work URL is invalid")
    handle, slug = segments[0], segments[1][:-3]
    work = api_request(
        authority,
        "/public/creators/"
        + urllib.parse.quote(handle, safe="")
        + "/works/"
        + urllib.parse.quote(slug, safe=""),
        request_fn=request_fn,
    )
    projection = work if isinstance(work, dict) else None
    public_work = projection.get("work") if isinstance(projection, dict) else None
    action = (
        public_work.get("websiteReplicaAction")
        if isinstance(public_work, dict)
        else None
    )
    instruction = action.get("instruction") if isinstance(action, dict) else None
    if instruction is None and isinstance(public_work, dict):
        public_replica = public_work.get("websiteReplica")
        # A delisted public work retains an authoritative code for discovery
        # and existing rights. The server still denies any new checkout.
        if (public_work.get("kind") == "WEBSITE" and public_work.get("status") == "PUBLISHED"
                and isinstance(public_replica, dict) and public_replica.get("availability") == "DELISTED"
                and isinstance(public_replica.get("shortCode"), str)):
            instruction = "VICEME-REPLICA:" + public_replica["shortCode"]
    if not isinstance(instruction, str) or not INSTRUCTION_PATTERN.fullmatch(
        instruction
    ):
        raise WorkflowError(
            "MAKE_COPY_ENTRY_INVALID",
            "The Work has no platform-controlled let-me-make-a-copy entry",
        )
    return instruction, public_work_has_active_page(projection)


def fetch_work_instruction(
    authority: Authority, request_fn: RequestFn = http_request
) -> str:
    instruction, _hosted = fetch_public_work_entry(authority, request_fn)
    return instruction


def api_request(
    authority: Authority,
    endpoint: str,
    *,
    method: str = "GET",
    body: Optional[Dict[str, Any]] = None,
    token: Optional[str] = None,
    timeout: float = 20,
    request_fn: RequestFn = http_request,
) -> Any:
    encoded = None if body is None else json.dumps(body).encode("utf-8")
    headers = {"Accept": "application/json"}
    if encoded is not None:
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    response = request_fn(
        method,
        authority.api_base_url + endpoint,
        headers=headers,
        body=encoded,
        timeout=timeout,
    )
    try:
        decoded = json.loads(response.body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        decoded = None
    if not 200 <= response.status < 300:
        code = (
            decoded.get("code")
            if isinstance(decoded, dict) and isinstance(decoded.get("code"), str)
            else "MAKE_COPY_API_FAILED"
        )
        message = (
            decoded.get("message")
            if isinstance(decoded, dict)
            and isinstance(decoded.get("message"), str)
            else "ViceMe returned an invalid response"
        )
        raise WorkflowError(code, message, {"statusCode": response.status})
    return decoded


def assert_resolution(value: Any) -> Dict[str, Any]:
    valid = isinstance(value, dict)
    creator = value.get("creator", {}) if valid else {}
    product = value.get("product", {}) if valid else {}
    valid = bool(
        valid
        and UUID_PATTERN.fullmatch(str(value.get("replicaId", "")))
        and re.fullmatch(r"VMR-[A-Z0-9]{20}", str(value.get("shortCode", "")))
        and isinstance(value.get("title"), str)
        and isinstance(creator, dict)
        and isinstance(creator.get("displayName"), str)
        and isinstance(value.get("viceMeWorkUrl"), str)
        and isinstance(product, dict)
        and UUID_PATTERN.fullmatch(str(product.get("id", "")))
        and UUID_PATTERN.fullmatch(str(product.get("skuId", "")))
        and product.get("currency") in {"CNY", "USD"}
        and isinstance(product.get("priceCents"), int)
        and not isinstance(product.get("priceCents"), bool)
        and product["priceCents"] >= 0
    )
    if not valid:
        raise WorkflowError(
            "MAKE_COPY_RESPONSE_INVALID",
            "ViceMe returned an invalid Replica description",
        )
    return value


def resolve_work(
    authority: Authority, request_fn: RequestFn = http_request
) -> Tuple[str, Dict[str, Any]]:
    instruction, replica, _hosted = resolve_work_with_hosting(authority, request_fn)
    return instruction, replica


def resolve_work_with_hosting(
    authority: Authority, request_fn: RequestFn = http_request
) -> Tuple[str, Dict[str, Any], bool]:
    instruction, has_active_page = fetch_public_work_entry(authority, request_fn)
    replica = assert_resolution(
        api_request(
            authority,
            "/website-replicas/resolve",
            method="POST",
            body={"instruction": instruction},
            request_fn=request_fn,
        )
    )
    resolved = urllib.parse.urlsplit(replica["viceMeWorkUrl"])
    expected = urllib.parse.urlsplit(authority.work_url).path[:-3]
    if (
        resolved.scheme + "://" + resolved.netloc != authority.web_origin
        or resolved.path != expected
        or resolved.query
        or resolved.fragment
    ):
        raise WorkflowError(
            "MAKE_COPY_RESPONSE_INVALID",
            "Replica Work belongs to a different ViceMe authority",
        )
    return instruction, replica, has_active_page


def public_work_has_active_page(projection: Any) -> bool:
    presentation = projection.get("presentation") if isinstance(projection, dict) else None
    return isinstance(presentation, dict) and presentation.get("mode") == "ACTIVE"


def work_presentation(has_active_page: bool, work_url: str) -> Dict[str, Any]:
    if has_active_page is True and isinstance(work_url, str) and work_url:
        return {"mode": "CREATOR_PAGE", "url": work_url}
    return {"mode": "WORKSPACE_TEXT"}


def safe_target_name(title: str) -> str:
    normalized = unicodedata.normalize("NFKC", title).lower()
    value = re.sub(r"-+", "-", "".join(c if c.isalnum() else "-" for c in normalized))
    return value.strip("-")[:48] or "website-copy"


def resolve_target(raw_target: Optional[str], title: str) -> Path:
    candidate = Path(raw_target or (Path.cwd() / safe_target_name(title))).absolute()
    try:
        parent = candidate.parent.resolve(strict=True)
    except OSError:
        raise WorkflowError(
            "REPLICA_TARGET_PARENT_INVALID",
            "Target parent must be a real existing directory",
        )
    if not parent.is_dir() or parent.is_symlink() or not candidate.name:
        raise WorkflowError(
            "REPLICA_TARGET_PARENT_INVALID",
            "Target parent must be a real existing directory",
        )
    return parent / candidate.name


def state_root() -> Path:
    return Path.home() / ".viceme-cli" / "replica-purchases"


def current_windows_sid() -> str:
    completed = subprocess.run(
        ["whoami", "/user", "/fo", "csv", "/nh"],
        check=True,
        capture_output=True,
        text=True,
        timeout=10,
        creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
    )
    rows = list(csv.reader(completed.stdout.splitlines()))
    if not rows or len(rows[0]) < 2 or not re.fullmatch(r"S-1-[0-9-]+", rows[0][1]):
        raise RuntimeError("could not resolve current Windows SID")
    return rows[0][1]


def protect_windows(target: Path, directory: bool) -> None:
    sid = current_windows_sid()
    inheritance = "(OI)(CI)F" if directory else "F"
    subprocess.run(
        [
            "icacls",
            str(target),
            "/inheritance:r",
            "/grant:r",
            "*" + sid + ":" + inheritance,
            "*S-1-5-18:" + inheritance,
        ],
        check=True,
        capture_output=True,
        timeout=15,
        creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
    )


def ensure_private_directory(directory: Path) -> None:
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    info = directory.lstat()
    if not stat.S_ISDIR(info.st_mode) or directory.is_symlink():
        raise WorkflowError(
            "MAKE_COPY_PRIVATE_STATE_UNAVAILABLE",
            "Recovery state path is not a real directory",
        )
    try:
        if os.name == "nt":
            protect_windows(directory, True)
        else:
            directory.chmod(0o700)
    except (OSError, subprocess.SubprocessError):
        raise WorkflowError(
            "MAKE_COPY_PRIVATE_STATE_UNAVAILABLE",
            "Could not establish private recovery state permissions",
        )


def atomic_private_write(filename: Path, value: Dict[str, Any]) -> None:
    temporary = filename.with_name(filename.name + ".tmp-" + str(uuid.uuid4()))
    descriptor = os.open(str(temporary), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            json.dump(value, handle, separators=(",", ":"), ensure_ascii=False)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        if os.name == "nt":
            protect_windows(temporary, False)
        else:
            temporary.chmod(0o600)
        os.replace(temporary, filename)
        if os.name == "nt":
            protect_windows(filename, False)
    except Exception:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass
        raise


def state_identity(authority: Authority, short_code: str, target: Path) -> str:
    value = f"{authority.api_base_url}\n{short_code}\n{target}".encode("utf-8")
    return hashlib.sha256(value).hexdigest()


def standalone_receipt_path(
    authority: Authority, short_code: str, root: Optional[Path] = None
) -> Path:
    root = root or state_root()
    value = f"{authority.api_base_url}\n{short_code}".encode("utf-8")
    return root / ("standalone-" + hashlib.sha256(value).hexdigest() + ".json")


def read_state(filename: Path) -> Optional[Dict[str, Any]]:
    try:
        info = filename.lstat()
    except FileNotFoundError:
        return None
    if (
        not stat.S_ISREG(info.st_mode)
        or filename.is_symlink()
        or info.st_size > 64 * 1024
        or (os.name != "nt" and info.st_mode & 0o077)
    ):
        raise WorkflowError("MAKE_COPY_STATE_INVALID", "Recovery state is invalid")
    try:
        value = json.loads(filename.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError):
        raise WorkflowError("MAKE_COPY_STATE_INVALID", "Recovery state is invalid")
    if not isinstance(value, dict):
        raise WorkflowError("MAKE_COPY_STATE_INVALID", "Recovery state is invalid")
    return value


def recoverable_paid_receipt(
    authority: Authority, replica: Dict[str, Any], root: Optional[Path] = None
) -> Optional[Dict[str, Any]]:
    receipt = recoverable_paid_receipt_by_code(authority, replica["shortCode"], root)
    if receipt is None:
        return None
    if receipt.get("replicaId") != replica["replicaId"]:
        raise WorkflowError(
            "MAKE_COPY_STATE_INVALID", "Paid Replica recovery state is invalid"
        )
    return receipt


def recoverable_paid_receipt_by_code(
    authority: Authority, short_code: str, root: Optional[Path] = None
) -> Optional[Dict[str, Any]]:
    receipt = read_state(standalone_receipt_path(authority, short_code, root))
    if receipt is None:
        return None
    if (
        receipt.get("schemaVersion") != 1
        or not UUID_PATTERN.fullmatch(str(receipt.get("replicaId", "")))
        or not isinstance(receipt.get("orderNo"), str)
        or not receipt["orderNo"]
        or not SECRET_PATTERN.fullmatch(str(receipt.get("recoverySecret", "")))
    ):
        raise WorkflowError(
            "MAKE_COPY_STATE_INVALID", "Paid Replica recovery state is invalid"
        )
    return receipt


def state_store(authority: Authority, short_code: str, target: Path) -> Dict[str, Path]:
    root = state_root()
    ensure_private_directory(root)
    identity = state_identity(authority, short_code, target)
    receipt = standalone_receipt_path(authority, short_code, root)
    return {
        "filename": root / (identity + ".json"),
        "completionFilename": root / ("completed-" + identity + ".json"),
        "paidFilename": receipt.with_suffix(".zip"),
        "paidReceiptFilename": receipt,
        "lockDirectory": root / (identity + ".lock"),
    }


def process_exists(pid: int) -> bool:
    try:
        os.kill(pid, 0)
        return True
    except PermissionError:
        return True
    except ProcessLookupError:
        return False


def with_lock(store: Dict[str, Path], run: Callable[[], Any]) -> Any:
    lock_directory = store["lockDirectory"]
    owner_filename = lock_directory / "owner.json"
    try:
        lock_directory.mkdir(mode=0o700)
    except FileExistsError:
        owner = read_state(owner_filename)
        pid = owner.get("pid") if owner else None
        if isinstance(pid, int) and pid > 0 and process_exists(pid):
            raise WorkflowError(
                "MAKE_COPY_ACTIVE",
                "Another let-me-make-a-copy operation is active for this target",
            )
        stale = lock_directory.with_name(
            lock_directory.name + ".stale-" + str(uuid.uuid4())
        )
        try:
            os.replace(lock_directory, stale)
            shutil.rmtree(stale)
            lock_directory.mkdir(mode=0o700)
        except OSError:
            raise WorkflowError(
                "MAKE_COPY_ACTIVE",
                "Another let-me-make-a-copy operation is active for this target",
            )
    try:
        ensure_private_directory(lock_directory)
        atomic_private_write(owner_filename, {"pid": os.getpid()})
        return run()
    finally:
        shutil.rmtree(lock_directory, ignore_errors=True)


def new_secret() -> str:
    return base64.urlsafe_b64encode(secrets.token_bytes(32)).rstrip(b"=").decode("ascii")


def iso_now() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat(timespec="milliseconds").replace(
        "+00:00", "Z"
    )


def initial_state(
    authority: Authority,
    instruction: str,
    replica: Dict[str, Any],
    target: Path,
) -> Dict[str, Any]:
    now = iso_now()
    return {
        "schemaVersion": 1,
        "apiBaseUrl": authority.api_base_url,
        "instruction": instruction,
        "shortCode": replica["shortCode"],
        "replicaId": replica["replicaId"],
        "productId": replica["product"]["id"],
        "skuId": replica["product"]["skuId"],
        "priceCents": replica["product"]["priceCents"],
        "target": str(target),
        "sessionClientRequestId": str(uuid.uuid4()),
        "sessionReplaySecret": new_secret(),
        "quoteClientRequestId": str(uuid.uuid4()),
        "orderClientRequestId": str(uuid.uuid4()),
        "downloadRecoverySecret": new_secret(),
        "createdAt": now,
        "updatedAt": now,
    }


def validate_state(
    value: Dict[str, Any],
    authority: Authority,
    replica: Dict[str, Any],
    target: Path,
) -> Dict[str, Any]:
    if not (
        value.get("schemaVersion") == 1
        and value.get("apiBaseUrl") == authority.api_base_url
        and value.get("shortCode") == replica["shortCode"]
        and value.get("replicaId") == replica["replicaId"]
        and value.get("productId") == replica["product"]["id"]
        and value.get("skuId") == replica["product"]["skuId"]
        and type(value.get("priceCents")) is int
        and value["priceCents"] >= 0
        and value.get("target") == str(target)
        and UUID_PATTERN.fullmatch(str(value.get("sessionClientRequestId", "")))
        and UUID_PATTERN.fullmatch(str(value.get("quoteClientRequestId", "")))
        and isinstance(value.get("orderClientRequestId"), str)
        and SECRET_PATTERN.fullmatch(str(value.get("sessionReplaySecret", "")))
        and SECRET_PATTERN.fullmatch(str(value.get("downloadRecoverySecret", "")))
    ):
        raise WorkflowError("MAKE_COPY_STATE_INVALID", "Recovery state is invalid")
    return value


def persist_state(store: Dict[str, Path], state: Dict[str, Any]) -> None:
    state["updatedAt"] = iso_now()
    atomic_private_write(store["filename"], state)


def checkout_response(value: Any) -> Dict[str, Any]:
    if not (
        isinstance(value, dict)
        and isinstance(value.get("orderNo"), str)
        and value.get("status") in {"PENDING", "PAID", "CLOSED", "FAILED", "CANCELLED"}
        and isinstance(value.get("checkoutUrl"), str)
    ):
        raise WorkflowError(
            "MAKE_COPY_RESPONSE_INVALID", "ViceMe returned an invalid checkout"
        )
    return value


def ensure_checkout(
    authority: Authority,
    state: Dict[str, Any],
    store: Dict[str, Path],
    request_fn: RequestFn = http_request,
) -> Dict[str, Any]:
    session = api_request(
        authority,
        "/website-replica-sessions",
        method="POST",
        body={
            "instruction": state["instruction"],
            "clientRequestId": state["sessionClientRequestId"],
            "replaySecret": state["sessionReplaySecret"],
        },
        request_fn=request_fn,
    )
    if not (
        isinstance(session, dict)
        and UUID_PATTERN.fullmatch(str(session.get("sessionId", "")))
        and isinstance(session.get("token"), str)
    ):
        raise WorkflowError(
            "MAKE_COPY_RESPONSE_INVALID",
            "ViceMe returned an invalid anonymous session",
        )
    state.update(
        sessionId=session["sessionId"],
        sessionToken=session["token"],
        sessionExpiresAt=session.get("expiresAt"),
    )
    persist_state(store, state)
    checkout = checkout_response(
        api_request(
            authority,
            "/website-replica-sessions/"
            + urllib.parse.quote(state["sessionId"], safe="")
            + "/checkout",
            method="POST",
            token=state["sessionToken"],
            body={
                "acceptedPriceCents": state["priceCents"],
                "quoteClientRequestId": state["quoteClientRequestId"],
                "orderClientRequestId": state["orderClientRequestId"],
                "downloadRecoverySecret": state["downloadRecoverySecret"],
                "locale": "zh-CN",
            },
            request_fn=request_fn,
        )
    )
    if state.get("orderNo") and state["orderNo"] != checkout["orderNo"]:
        raise WorkflowError("REPLICA_PURCHASE_RECOVERY_CONFLICT", "Checkout no longer matches the original payment attempt", {"nextAction": "STOP_AND_REPORT"})
    state.update(
        orderNo=checkout["orderNo"],
        orderExpiresAt=checkout.get("expiresAt"),
        checkoutUrl=checkout["checkoutUrl"],
    )
    persist_state(store, state)
    atomic_private_write(
        store["paidReceiptFilename"],
        {
            "schemaVersion": 1,
            "replicaId": state["replicaId"],
            "orderNo": state["orderNo"],
            "recoverySecret": state["downloadRecoverySecret"],
            "updatedAt": iso_now(),
        },
    )
    if checkout["status"] == "PENDING":
        # Checkout creates the order but intentionally omits its amount. Read
        # the authenticated immutable order view; never substitute today's price.
        view = api_request(
            authority, "/website-replica-sessions/" + urllib.parse.quote(state["sessionId"], safe="")
            + "/orders/" + urllib.parse.quote(checkout["orderNo"], safe=""),
            token=state["sessionToken"], request_fn=request_fn,
        )
        order = view.get("order") if isinstance(view, dict) else None
        if (not isinstance(order, dict) or not isinstance(view.get("replica"), dict)
                or view["replica"].get("replicaId") != state["replicaId"]
                or order.get("orderNo") != checkout["orderNo"]
                or type(order.get("amountCents")) is not int or order["amountCents"] < 0
                or order.get("currency") != "CNY"
                or order.get("status") not in {"PENDING", "PAID", "CLOSED", "FAILED", "CANCELLED"}):
            raise WorkflowError("MAKE_COPY_RESPONSE_INVALID", "ViceMe returned an invalid order view; keep the original order")
        checkout = {**checkout, **order}
        state.update(orderAmountCents=order["amountCents"], orderCurrency=order["currency"])
        persist_state(store, state)
    return checkout


def try_recover_download(
    authority: Authority,
    state: Dict[str, Any],
    request_fn: RequestFn = http_request,
) -> Optional[Dict[str, Any]]:
    if not state.get("orderNo"):
        return None
    try:
        return api_request(
            authority,
            "/website-replica-sessions/recover-download-v2",
            method="POST",
            body={
                "orderNo": state["orderNo"],
                "recoverySecret": state["downloadRecoverySecret"],
            },
            request_fn=request_fn,
        )
    except WorkflowError as error:
        if error.code == "WEBSITE_REPLICA_NOT_FOUND" and error.details.get(
            "statusCode"
        ) == 404:
            return None
        raise


def recover_order_status(
    authority: Authority,
    order_no: str,
    recovery_secret: str,
    request_fn: RequestFn = http_request,
) -> Dict[str, Any]:
    status = api_request(
        authority,
        "/website-replica-sessions/recover-status",
        method="POST",
        body={"orderNo": order_no, "recoverySecret": recovery_secret},
        request_fn=request_fn,
    )
    payment = status.get("payment", {}) if isinstance(status, dict) else {}
    if not isinstance(status, dict) or status.get("orderNo") != order_no or payment.get("status") not in {
        "PENDING",
        "PAID",
        "CLOSED",
    }:
        raise WorkflowError(
            "MAKE_COPY_RESPONSE_INVALID", "ViceMe returned an invalid order status"
        )
    return status


def cancel_order_attempt(
    authority: Authority,
    order_no: str,
    recovery_secret: str,
    request_fn: RequestFn = http_request,
) -> Dict[str, Any]:
    status = api_request(
        authority,
        "/website-replica-sessions/cancel-order",
        method="POST",
        body={"orderNo": order_no, "recoverySecret": recovery_secret},
        request_fn=request_fn,
    )
    payment = status.get("payment", {}) if isinstance(status, dict) else {}
    if not isinstance(status, dict) or status.get("orderNo") != order_no or payment.get("status") != "CLOSED":
        raise WorkflowError(
            "MAKE_COPY_RESPONSE_INVALID",
            "ViceMe did not definitively close the previous payment attempt",
        )
    return status


def payment_restart_required(authority: Authority, state: Dict[str, Any], status: str) -> WorkflowError:
    details = {"nextAction": "STOP_AND_REPORT", "orderNo": state["orderNo"], "status": status}
    if state.get("instruction") and state.get("target"):
        details["recovery"] = {"mode": "RECOVERY_ONLY", "requiresUserRequest": True,
                               "args": ["install", "--work-url", authority.work_url, "--replica-code", state["instruction"],
                                        "--recovery-only", "--expected-order-no", state["orderNo"], "--target", state["target"]]}
    return WorkflowError("REPLICA_PAYMENT_RESTART_REQUIRED",
                         "The original payment attempt is retained; replacing it requires a new explicit user request", details)


def wait_for_payment(
    authority: Authority,
    state: Dict[str, Any],
    request_fn: RequestFn = http_request,
    sleep_fn: Callable[[float], None] = time.sleep,
) -> Dict[str, Any]:
    for _ in range(60):
        sleep_fn(3)
        status = api_request(
            authority,
            "/website-replica-sessions/"
            + urllib.parse.quote(state["sessionId"], safe="")
            + "/orders/"
            + urllib.parse.quote(state["orderNo"], safe="")
            + "/status",
            token=state["sessionToken"],
            request_fn=request_fn,
        )
        payment = status.get("payment", {}) if isinstance(status, dict) else {}
        if (not isinstance(status, dict) or status.get("orderNo") != state["orderNo"]
                or not isinstance(payment, dict)
                or payment.get("status") not in {"PENDING", "PAID", "CLOSED", "FAILED", "CANCELLED"}):
            raise WorkflowError("MAKE_COPY_RESPONSE_INVALID", "ViceMe returned an invalid payment status; keep the original order")
        if payment.get("status") == "PAID":
            return payment
        if payment.get("status") in {"CLOSED", "FAILED", "CANCELLED"}:
            raise WorkflowError(
                "REPLICA_PAYMENT_TERMINAL",
                "Website Replica payment did not complete",
                {"orderNo": state["orderNo"], "status": payment.get("status")},
            )
    raise WorkflowError(
        "REPLICA_PAYMENT_TIMEOUT",
        "Website Replica payment was not observed before the wait deadline",
        {"nextAction": "STOP_AND_REPORT", "orderNo": state["orderNo"]},
    )


# RFC 8032 section 5.1 verification, specialized to Ed25519.
_P = 2**255 - 19
_L = 2**252 + 27742317777372353535851937790883648493
_D = (-121665 * pow(121666, _P - 2, _P)) % _P
_I = pow(2, (_P - 1) // 4, _P)


def _x_recover(y: int) -> int:
    xx = (y * y - 1) * pow(_D * y * y + 1, _P - 2, _P) % _P
    x = pow(xx, (_P + 3) // 8, _P)
    if (x * x - xx) % _P != 0:
        x = x * _I % _P
    if (x * x - xx) % _P != 0:
        raise ValueError("invalid Ed25519 point")
    return _P - x if x & 1 else x


_BY = 4 * pow(5, _P - 2, _P) % _P
_B = (_x_recover(_BY), _BY)
_IDENTITY = (0, 1)


def _edwards_add(left: Tuple[int, int], right: Tuple[int, int]) -> Tuple[int, int]:
    x1, y1 = left
    x2, y2 = right
    product = _D * x1 * x2 * y1 * y2
    x3 = (x1 * y2 + x2 * y1) * pow(1 + product, _P - 2, _P) % _P
    y3 = (y1 * y2 + x1 * x2) * pow(1 - product, _P - 2, _P) % _P
    return x3, y3


def _scalar_mult(point: Tuple[int, int], scalar: int) -> Tuple[int, int]:
    result = _IDENTITY
    addend = point
    while scalar:
        if scalar & 1:
            result = _edwards_add(result, addend)
        addend = _edwards_add(addend, addend)
        scalar >>= 1
    return result


def _decode_point(encoded: bytes) -> Tuple[int, int]:
    if len(encoded) != 32:
        raise ValueError("invalid Ed25519 point length")
    raw = int.from_bytes(encoded, "little")
    y = raw & ((1 << 255) - 1)
    if y >= _P:
        raise ValueError("non-canonical Ed25519 point")
    x = _x_recover(y)
    sign = raw >> 255
    if x == 0 and sign:
        raise ValueError("non-canonical Ed25519 point")
    if (x & 1) != sign:
        x = _P - x
    if (-x * x + y * y - 1 - _D * x * x * y * y) % _P != 0:
        raise ValueError("Ed25519 point is not on curve")
    return x, y


def verify_ed25519(public_key: bytes, message: bytes, signature: bytes) -> bool:
    if len(public_key) != 32 or len(signature) != 64:
        return False
    try:
        public_point = _decode_point(public_key)
        encoded_r = signature[:32]
        point_r = _decode_point(encoded_r)
        scalar_s = int.from_bytes(signature[32:], "little")
        if scalar_s >= _L:
            return False
        challenge = int.from_bytes(
            hashlib.sha512(encoded_r + public_key + message).digest(), "little"
        ) % _L
        return _scalar_mult(_B, scalar_s) == _edwards_add(
            point_r, _scalar_mult(public_point, challenge)
        )
    except (ValueError, ZeroDivisionError):
        return False


def b64url_decode(value: str) -> bytes:
    if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9_-]*", value):
        raise ValueError("invalid base64url")
    encoded = (value + "=" * (-len(value) % 4)).encode("ascii")
    return base64.b64decode(encoded, altchars=b"-_", validate=True)


def decode_jws_part(value: str) -> Dict[str, Any]:
    decoded = json.loads(b64url_decode(value).decode("utf-8"))
    if not isinstance(decoded, dict):
        raise ValueError("JWS part must be an object")
    return decoded


def _valid_timestamp(value: Any) -> bool:
    if not isinstance(value, str):
        return False
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
        return parsed.tzinfo is not None
    except ValueError:
        return False


def verify_license(
    authority: Authority,
    download: Dict[str, Any],
    state: Dict[str, Any],
    request_fn: RequestFn = http_request,
) -> Dict[str, Any]:
    parts = str(download.get("licenseJws", "")).split(".")
    if len(parts) != 3:
        raise WorkflowError("REPLICA_LICENSE_INVALID", "Replica license is invalid")
    try:
        header = decode_jws_part(parts[0])
        claims = decode_jws_part(parts[1])
    except (ValueError, UnicodeDecodeError, json.JSONDecodeError):
        raise WorkflowError("REPLICA_LICENSE_INVALID", "Replica license is invalid")
    if not (
        header.get("alg") == "EdDSA"
        and header.get("typ") == LICENSE_TYPE
        and isinstance(header.get("kid"), str)
        and claims.get("schemaVersion") == LICENSE_SCHEMA
        and claims.get("entitlementId") is not None
        and claims.get("replicaId") == download.get("replicaId")
        and claims.get("replicaId") == state.get("replicaId")
        and claims.get("versionId") == download.get("versionId")
        and claims.get("version") == download.get("version")
        and claims.get("orderNo") == state.get("orderNo")
        and claims.get("artifactDigest") == download.get("artifactDigest")
        and isinstance(claims.get("licenseTermsVersion"), str)
        and _valid_timestamp(claims.get("issuedAt"))
    ):
        raise WorkflowError(
            "REPLICA_LICENSE_IDENTITY_MISMATCH",
            "Replica license does not match the purchased source",
        )
    trust = api_request(
        authority,
        "/commerce-skill-trust-keys/"
        + urllib.parse.quote(header["kid"], safe=""),
        request_fn=request_fn,
    )
    if not (
        isinstance(trust, dict)
        and trust.get("keyId") == header["kid"]
        and trust.get("algorithm") == "Ed25519"
        and isinstance(trust.get("publicKey"), str)
    ):
        raise WorkflowError(
            "REPLICA_LICENSE_SIGNING_KEY_UNTRUSTED",
            "Replica signing key is not trusted",
        )
    try:
        spki = b64url_decode(trust["publicKey"])
        signature = b64url_decode(parts[2])
    except (ValueError, TypeError):
        spki = b""
        signature = b""
    public_key = (
        spki[len(ED25519_SPKI_PREFIX) :]
        if spki.startswith(ED25519_SPKI_PREFIX)
        and len(spki) == len(ED25519_SPKI_PREFIX) + 32
        else b""
    )
    if not verify_ed25519(
        public_key, (parts[0] + "." + parts[1]).encode("ascii"), signature
    ):
        raise WorkflowError(
            "REPLICA_LICENSE_SIGNATURE_INVALID",
            "Replica license signature is invalid",
        )
    return claims


def download_archive(download: Dict[str, Any], store: Dict[str, Path]) -> Path:
    size = download.get("sizeBytes")
    digest = download.get("artifactDigest")
    filename = download.get("fileName")
    if not (
        isinstance(size, int)
        and not isinstance(size, bool)
        and 0 < size <= MAX_ARCHIVE_BYTES
        and isinstance(digest, str)
        and re.fullmatch(r"[a-f0-9]{64}", digest)
        and isinstance(filename, str)
        and re.fullmatch(r"[^/\\\x00]+\.zip", filename, re.IGNORECASE)
    ):
        raise WorkflowError(
            "REPLICA_DOWNLOAD_RESPONSE_INVALID",
            "Replica download metadata is invalid",
        )
    parsed = urllib.parse.urlsplit(str(download.get("downloadUrl", "")))
    if parsed.scheme != "https":
        raise WorkflowError(
            "REPLICA_DOWNLOAD_RESPONSE_INVALID",
            "Replica download URL must use HTTPS",
        )
    temporary = store["paidFilename"].with_name(
        store["paidFilename"].name + ".download-" + str(uuid.uuid4())
    )
    request = urllib.request.Request(download["downloadUrl"], method="GET")
    opener = urllib.request.build_opener(_NoRedirect())
    written = 0
    hasher = hashlib.sha256()
    try:
        with opener.open(request, timeout=120) as response, open(temporary, "xb") as handle:
            if not 200 <= response.status < 300:
                raise WorkflowError(
                    "REPLICA_DOWNLOAD_FAILED", "Replica source download failed"
                )
            while True:
                chunk = response.read(64 * 1024)
                if not chunk:
                    break
                written += len(chunk)
                if written > size or written > MAX_ARCHIVE_BYTES:
                    raise WorkflowError(
                        "REPLICA_DOWNLOAD_INVALID", "Replica source size changed"
                    )
                hasher.update(chunk)
                handle.write(chunk)
            handle.flush()
            os.fsync(handle.fileno())
    except Exception:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass
        raise
    if written != size or hasher.hexdigest() != digest:
        temporary.unlink(missing_ok=True)
        raise WorkflowError(
            "REPLICA_ARTIFACT_DIGEST_MISMATCH",
            "Replica source does not match its signed digest",
        )
    if os.name == "nt":
        protect_windows(temporary, False)
    else:
        temporary.chmod(0o600)
    store["paidFilename"].unlink(missing_ok=True)
    os.replace(temporary, store["paidFilename"])
    return store["paidFilename"]


def validate_archive_path(name: str) -> str:
    if (
        not name
        or "\\" in name
        or "\x00" in name
        or name.startswith("/")
        or unicodedata.normalize("NFC", name) != name
        or len(name.encode("utf-8")) > MAX_PATH_BYTES
    ):
        raise WorkflowError("REPLICA_ARCHIVE_INVALID", "Replica ZIP path is unsafe")
    trimmed = name[:-1] if name.endswith("/") else name
    segments = trimmed.split("/")
    if (
        not segments
        or len(segments) > MAX_PATH_DEPTH
        or any(
            not segment
            or segment in {".", ".."}
            or len(segment.encode("utf-8")) > MAX_SEGMENT_BYTES
            or segment.endswith((".", " "))
            or ":" in segment
            or WINDOWS_RESERVED.fullmatch(segment)
            for segment in segments
        )
        or segments[0].lower() == ".viceme"
    ):
        raise WorkflowError("REPLICA_ARCHIVE_INVALID", "Replica ZIP path is unsafe")
    return "/".join(segments)


def _zip_mode(info: zipfile.ZipInfo) -> int:
    return info.external_attr >> 16


def install_archive(
    archive_path: Path, target: Path, download: Dict[str, Any]
) -> Dict[str, Any]:
    if target.exists() or target.is_symlink():
        raise WorkflowError(
            "REPLICA_TARGET_EXISTS",
            "Refusing to overwrite target",
            {"target": str(target)},
        )
    try:
        archive = zipfile.ZipFile(archive_path)
    except (OSError, zipfile.BadZipFile):
        raise WorkflowError("REPLICA_ARCHIVE_INVALID", "Replica ZIP is invalid")
    with archive:
        entries = archive.infolist()
        if len(entries) > MAX_ENTRY_COUNT:
            raise WorkflowError(
                "REPLICA_ARCHIVE_INVALID", "Replica ZIP has too many entries"
            )
        planned = []
        collision_keys = set()
        file_count = 0
        expanded_bytes = 0
        for info in entries:
            relative = validate_archive_path(info.filename)
            key = unicodedata.normalize("NFC", relative).casefold()
            if key in collision_keys:
                raise WorkflowError(
                    "REPLICA_ARCHIVE_INVALID", "Replica ZIP paths collide"
                )
            collision_keys.add(key)
            mode = _zip_mode(info)
            file_type = stat.S_IFMT(mode)
            if file_type not in {0, stat.S_IFREG, stat.S_IFDIR} or info.flag_bits & 1:
                raise WorkflowError(
                    "REPLICA_ARCHIVE_INVALID",
                    "Replica ZIP contains an unsupported file type",
                )
            if not info.is_dir():
                file_count += 1
                if (
                    file_count > MAX_FILE_COUNT
                    or info.file_size > MAX_FILE_BYTES
                    or (info.compress_size == 0 and info.file_size > 0)
                    or (
                        info.compress_size > 0
                        and info.file_size > info.compress_size * MAX_COMPRESSION_RATIO
                    )
                ):
                    raise WorkflowError(
                        "REPLICA_ARCHIVE_INVALID", "Replica ZIP exceeds limits"
                    )
                expanded_bytes += info.file_size
                if expanded_bytes > MAX_EXPANDED_BYTES:
                    raise WorkflowError(
                        "REPLICA_ARCHIVE_INVALID", "Replica ZIP expands too large"
                    )
            planned.append((info, relative))
        if not any(
            not info.is_dir() and relative == "VICEME-REPLICA.md"
            for info, relative in planned
        ):
            raise WorkflowError(
                "REPLICA_DEPLOYMENT_GUIDE_INVALID",
                "Replica ZIP has no root VICEME-REPLICA.md",
            )
        staging = target.parent / (".viceme-replica-stage-" + str(uuid.uuid4()))
        staging.mkdir(mode=0o700)
        try:
            for info, relative in planned:
                destination = staging.joinpath(*relative.split("/"))
                if info.is_dir():
                    destination.mkdir(mode=0o755, parents=True, exist_ok=True)
                    continue
                destination.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
                with archive.open(info) as source, open(destination, "xb") as output:
                    copied = 0
                    while True:
                        chunk = source.read(64 * 1024)
                        if not chunk:
                            break
                        copied += len(chunk)
                        if copied > info.file_size:
                            raise WorkflowError(
                                "REPLICA_ARCHIVE_INVALID",
                                "Replica ZIP entry size changed",
                            )
                        output.write(chunk)
                if copied != info.file_size:
                    raise WorkflowError(
                        "REPLICA_ARCHIVE_INVALID", "Replica ZIP entry size changed"
                    )
                if os.name != "nt":
                    destination.chmod(0o755 if mode & 0o111 else 0o644)
            guide_path = staging / "VICEME-REPLICA.md"
            guide_bytes = guide_path.read_bytes()
            try:
                guide = guide_bytes.decode("utf-8")
            except UnicodeDecodeError:
                guide = ""
            if (
                not guide_bytes
                or len(guide_bytes) > MAX_GUIDE_BYTES
                or not guide.strip()
            ):
                raise WorkflowError(
                    "REPLICA_DEPLOYMENT_GUIDE_INVALID",
                    "Replica deployment guide is invalid",
                )
            license_directory = staging / ".viceme"
            license_directory.mkdir(mode=0o700)
            license_file = license_directory / "replica-license.json"
            descriptor = os.open(
                str(license_file), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600
            )
            with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
                json.dump(
                    {
                        "schemaVersion": 1,
                        "replicaId": download["replicaId"],
                        "versionId": download["versionId"],
                        "version": download["version"],
                        "artifactDigest": download["artifactDigest"],
                        "licenseJws": download["licenseJws"],
                    },
                    handle,
                    separators=(",", ":"),
                )
                handle.write("\n")
            os.replace(staging, target)
        except Exception:
            shutil.rmtree(staging, ignore_errors=True)
            raise
    return {
        "target": str(target),
        "fileCount": file_count,
        "expandedBytes": expanded_bytes,
    }


def complete_install(
    authority: Authority,
    state: Dict[str, Any],
    store: Dict[str, Path],
    download: Dict[str, Any],
    request_fn: RequestFn = http_request,
) -> Dict[str, Any]:
    claims = verify_license(authority, download, state, request_fn)
    archive_path = download_archive(download, store)
    atomic_private_write(
        store["paidReceiptFilename"],
        {
            "schemaVersion": 1,
            **({"invitationFlowId": state["invitationFlowId"]} if state.get("invitationFlowId") else {}),
            "replicaId": download["replicaId"],
            "versionId": download["versionId"],
            "version": download["version"],
            "orderNo": state["orderNo"],
            "artifactDigest": download["artifactDigest"],
            "sizeBytes": download["sizeBytes"],
            "licenseJws": download["licenseJws"],
            "recoverySecret": state["downloadRecoverySecret"],
            "paidAt": claims["issuedAt"],
            "entitlementId": claims["entitlementId"],
        },
    )
    installed = install_archive(archive_path, Path(state["target"]), download)
    completion = {
        **installed,
        **({"invitationFlowId": state["invitationFlowId"]} if state.get("invitationFlowId") else {}),
        "schemaVersion": 1,
        "replicaId": download["replicaId"],
        "versionId": download["versionId"],
        "version": download["version"],
        "orderNo": state["orderNo"],
        "artifactDigest": download["artifactDigest"],
        "completedAt": iso_now(),
    }
    atomic_private_write(store["completionFilename"], completion)
    store["filename"].unlink(missing_ok=True)
    return completion


def public_https_url(value: Any) -> bool:
    try:
        parsed = urllib.parse.urlsplit(value)
        return (isinstance(value, str) and len(value) <= 2048
                and parsed.scheme == "https" and bool(parsed.hostname)
                and parsed.username is None and parsed.password is None)
    except (TypeError, ValueError):
        return False


def discovery(authority: Authority, replica: Dict[str, Any],
              request_fn: RequestFn = http_request) -> Dict[str, Any]:
    value = api_request(authority, "/website-replicas/" + replica["shortCode"] + "/discovery",
                        request_fn=request_fn)
    valid = (isinstance(value, dict) and value.get("replicaId") == replica["replicaId"]
             and value.get("shortCode") == replica["shortCode"]
             and value.get("viceMeWorkUrl") == replica["viceMeWorkUrl"]
             and public_https_url(value.get("previewUrl"))
             and public_https_url(value.get("discoveryUrl"))
             and isinstance(value.get("title"), str)
             and isinstance(value.get("summary"), str)
             and isinstance(value.get("bodyMarkdown"), str)
             and isinstance(value.get("creator"), dict)
             and isinstance(value.get("statistics"), dict)
             and all(type(value["statistics"].get(key)) is int and value["statistics"][key] >= 0
                     for key in ("acquisitionCount", "commentCount")))
    if not valid:
        raise WorkflowError("MAKE_COPY_RESPONSE_INVALID", "ViceMe returned invalid discovery information")
    return value


def payment_resource(authority: Authority, name: str,
                     request_fn: RequestFn = http_request) -> bytes:
    digest = PAYMENT_RESOURCE_SHA256[name]
    directory = state_root() / "payment-resources"
    ensure_private_directory(directory)
    filename = directory / (digest + "-" + name)
    if filename.exists() or filename.is_symlink():
        info = filename.lstat()
        if not stat.S_ISREG(info.st_mode) or filename.is_symlink() or info.st_size > 262144:
            raise WorkflowError("PAYMENT_RESOURCE_INVALID", "Payment resource is invalid")
        content = filename.read_bytes()
    else:
        origin = "https://s3.viceme.ai" if authority.web_origin.endswith("viceme.ai") else "https://s3.viceme.cn"
        response = request_fn("GET", origin + "/skills/_widgets/sha256-" + digest + "/" + name, timeout=20)
        content = response.body
        if response.status != 200 or len(content) > 262144:
            raise WorkflowError("PAYMENT_RESOURCE_INVALID", "Payment resource is unavailable; keep the original order")
    if hashlib.sha256(content).hexdigest() != digest:
        raise WorkflowError("PAYMENT_RESOURCE_INVALID", "Payment resource integrity check failed")
    if not filename.exists():
        write_private_bytes(filename, content)
    return content


def write_private_bytes(filename: Path, content: bytes) -> str:
    ensure_private_directory(filename.parent)
    temporary = filename.with_name(filename.name + ".tmp-" + str(uuid.uuid4()))
    descriptor = os.open(str(temporary), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(content)
            handle.flush()
            os.fsync(handle.fileno())
        if os.name == "nt":
            protect_windows(temporary, False)
        os.replace(temporary, filename)
    finally:
        temporary.unlink(missing_ok=True)
    return str(filename.absolute())


def payment_presentation(authority: Authority, replica: Dict[str, Any], checkout: Dict[str, Any],
                         request_fn: RequestFn = http_request) -> Dict[str, Any]:
    if type(checkout.get("amountCents")) is not int or checkout["amountCents"] < 0 or checkout.get("currency") != "CNY":
        raise WorkflowError("MAKE_COPY_RESPONSE_INVALID", "The authoritative payment amount is unavailable; keep the original order")
    action = checkout.get("paymentAction")
    uri = action.get("content") if isinstance(action, dict) else None
    if (not isinstance(uri, str) or len(uri) > 4096 or action.get("type") != "QR_CODE"
            or not uri.startswith("weixin://") or not _valid_timestamp(checkout.get("expiresAt"))):
        raise WorkflowError("PAYMENT_QR_INVALID", "The order has no valid WeChat QR code; keep the original order")
    namespace = {"__name__": "viceme_qrcodegen"}
    exec(compile(payment_resource(authority, "qrcodegen.py", request_fn), "qrcodegen.py", "exec"), namespace)
    code = namespace["QrCode"].encode_text(uri, namespace["QrCode"].Ecc.MEDIUM)
    size = code.get_size()
    cells = "".join("M%d %dh1v1h-1z" % (x + 4, y + 4) for y in range(size)
                    for x in range(size) if code.get_module(x, y))
    dimension = size + 8
    svg = ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" '
           'role="img" aria-label="WeChat Pay QR" shape-rendering="crispEdges">'
           '<path fill="white" d="M0 0h%dv%dH0z"/><path fill="black" d="%s"/></svg>') % (dimension, dimension, dimension, dimension, cells)
    scale = max(4, 512 // dimension)
    pixels = dimension * scale
    rows = []
    for y in range(pixels):
        row = bytearray(1 + pixels)
        for x in range(pixels):
            qx, qy = x // scale - 4, y // scale - 4
            row[x + 1] = 0 if 0 <= qx < size and 0 <= qy < size and code.get_module(qx, qy) else 255
        rows.append(bytes(row))
    def chunk(tag, content):
        return struct.pack(">I", len(content)) + tag + content + struct.pack(">I", zlib.crc32(tag + content) & 0xffffffff)
    png = (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", pixels, pixels, 8, 0, 0, 0, 0))
           + chunk(b"IDAT", zlib.compress(b"".join(rows), 9)) + chunk(b"IEND", b""))
    data = {"title": replica["title"], "amountCents": checkout["amountCents"],
            "currency": checkout["currency"], "status": checkout["status"],
            "expiresAt": checkout["expiresAt"], "locale": "zh-CN" if authority.web_origin.endswith("viceme.cn") else "en-US",
            "paymentMethodLabel": "微信支付"}
    encoded = json.dumps(data, ensure_ascii=True).replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")
    html = payment_resource(authority, "payment.html", request_fn).decode("utf-8").replace("__QR_SVG__", svg).replace("__WIDGET_DATA__", encoded)
    stem = hashlib.sha256(checkout["orderNo"].encode()).hexdigest()
    directory = state_root() / "payment-presentations"
    widget = write_private_bytes(directory / (stem + ".html"), html.encode())
    image = write_private_bytes(directory / (stem + ".png"), png)
    return {"type": "LOCAL_IMAGE", "purpose": "PAYMENT_QR_CODE", "mimeType": "image/png",
            "widgetPath": widget, "widgetMimeType": "text/html", "imagePath": image,
            "imageChatSrc": "local-file://" + urllib.parse.quote(Path(image).as_posix(), safe="/:"),
            "expiresAt": checkout["expiresAt"], "altText": "微信支付二维码"}


def support_result(authority: Authority, state: Dict[str, Any], replica: Dict[str, Any],
                   payment: Dict[str, Any], request_fn: RequestFn = http_request) -> Dict[str, Any]:
    if payment.get("status") != "PAID" or not state.get("orderNo") or state.get("priceCents", 0) <= 0:
        raise WorkflowError("MAKE_COPY_RESPONSE_INVALID", "Support presentation requires a confirmed paid order")
    en = authority.web_origin.endswith("viceme.ai")
    confirmed = {"status": "PAID", "paidAt": payment.get("paidAt")}
    continuation = {"mode": "RECOVERY_ONLY", "args": ["install", "--work-url", authority.work_url,
                    "--replica-code", state["instruction"], "--recovery-only", "--expected-order-no", state["orderNo"], "--target", state["target"]]}
    data = {"status": "PAID", "locale": "en-US" if en else "zh-CN", "title": replica["title"],
            "resultTitle": "The creator has received your support" if en else "创作者已收到你的支持",
            "resultDescription": "Thank you for supporting this idea. Your work is being prepared." if en else "感谢你支持这个创意，正在为你准备作品。"}
    try:
        template = payment_resource(authority, "payment.html", request_fn).decode("utf-8")
        encoded = json.dumps(data, ensure_ascii=True).replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")
        html = template.replace("__QR_SVG__", "").replace("__WIDGET_DATA__", encoded)
        stem = hashlib.sha256(state["orderNo"].encode()).hexdigest()
        directory = state_root() / "payment-presentations"
        widget = write_private_bytes(directory / (stem + ".support.html"), html.encode())
    except Exception as error:
        raise WorkflowError("REPLICA_SUPPORT_PRESENTATION_FAILED", "Confirmed support result could not be prepared",
                            {"orderNo": state["orderNo"], "payment": confirmed, "stage": "PRESENT_SUPPORT_RESULT",
                             "nextAction": "STOP_AND_REPORT",
                             "recovery": {**continuation, "requiresUserRequest": True}}) from error
    result = {"nextAction": "PRESENT_SUPPORT_RESULT", "orderNo": state["orderNo"], "payment": confirmed,
              "title": replica["title"], "target": state["target"], "continuation": continuation,
              "presentation": {"widgetPath": widget, "widgetMimeType": "text/html",
                               "replacesWidgetPath": str((directory / (stem + ".html")).absolute())}}
    # Older records may lack the immutable order amount. Never use today's price.
    if type(state.get("orderAmountCents")) is int and state["orderAmountCents"] >= 0 and state.get("orderCurrency") in {"CNY", "USD"}:
        result.update(amountCents=state["orderAmountCents"], currency=state["orderCurrency"])
    return result


class InvitationFlow:
    """Optional analytics; never changes a purchase request or its outcome."""

    def __init__(self, authority: Authority, flow_id: Optional[str], request_fn: RequestFn):
        self.authority = authority
        self.id = flow_id if isinstance(flow_id, str) and UUID_PATTERN.fullmatch(flow_id) else None
        self.request_fn = request_fn
        self.state: Optional[Dict[str, Any]] = None
        self.restored = False

    def send(self, endpoint: str, body: Dict[str, Any], token: Optional[str] = None) -> bool:
        try:
            response = api_request(self.authority, endpoint, method="POST", body=body,
                                   token=token, timeout=1, request_fn=self.request_fn)
            return isinstance(response, dict) and response.get("recorded") is True
        except Exception:
            return False

    def start(self, short_code: str) -> Optional[str]:
        if self.id and self.send("/website-replica-invitation-flows", {
            "flowId": self.id, "shortCode": short_code, "engine": "PYTHON",
            "clientVersion": "standalone-invitation-v1",
        }):
            return self.id
        return None

    def report(self, event: str) -> None:
        if self.id:
            self.send("/website-replica-invitation-flows/events", {"flowId": self.id, "event": event})

    def use_saved(self, saved_id: Optional[str], existing: bool = True) -> None:
        if not self.id and isinstance(saved_id, str) and UUID_PATTERN.fullmatch(saved_id):
            self.id = saved_id
        if existing and self.id and self.id != saved_id and not self.restored:
            self.report("RESTORED")
            self.restored = True
        if existing and isinstance(saved_id, str) and UUID_PATTERN.fullmatch(saved_id):
            self.id = saved_id

    def bind(self, state: Dict[str, Any]) -> None:
        self.use_saved(state.get("invitationFlowId"), bool(state.get("orderNo") or state.get("invitationFlowId")))
        if self.id and not state.get("orderNo") and not state.get("invitationFlowId"):
            state["invitationFlowId"] = self.id
        self.state = state

    def associate(self) -> None:
        state = self.state or {}
        flow_id = state.get("invitationFlowId")
        if not isinstance(flow_id, str) or not UUID_PATTERN.fullmatch(flow_id):
            return
        if not all(state.get(key) for key in ("sessionId", "sessionToken", "orderNo")):
            return
        endpoint = "/website-replica-sessions/" + urllib.parse.quote(state["sessionId"], safe="") + "/orders/" + urllib.parse.quote(state["orderNo"], safe="") + "/invitation-flow"
        self.send(endpoint, {"flowId": flow_id}, state["sessionToken"])


def inspect(
    work_url: str,
    *,
    invitation_flow_id: Optional[str] = None,
    request_fn: RequestFn = http_request,
) -> Dict[str, Any]:
    authority = authority_for_work_url(work_url)
    instruction, replica, has_active_page = resolve_work_with_hosting(
        authority, request_fn
    )
    discovered = discovery(authority, replica, request_fn)
    recorded_flow_id = InvitationFlow(authority, invitation_flow_id, request_fn).start(replica["shortCode"])
    return {
        **({"invitationFlowId": recorded_flow_id} if recorded_flow_id else {}),
        "nextAction": "PRESENT_WORK",
        "workUrl": replica["viceMeWorkUrl"],
        "workPresentation": work_presentation(
            has_active_page, replica["viceMeWorkUrl"]
        ),
        "instruction": instruction,
        "replica": replica,
        "discovery": discovered,
        "presentationTarget": "AGENT_PLATFORM",
        "presentationPlacement": "RIGHT",
    }


def install(work_url: str, accepted_price_cents: Optional[int] = None, *,
            invitation_flow_id: Optional[str] = None, **kwargs: Any) -> Dict[str, Any]:
    flow = InvitationFlow(authority_for_work_url(work_url), invitation_flow_id, kwargs.get("request_fn", http_request))
    completed = None
    try:
        completed = _install(work_url, accepted_price_cents, flow=flow, **kwargs)
        continuation = completed.get("continuation", {})
        if flow.id and isinstance(continuation.get("args"), list):
            continuation["args"] += ["--invitation-flow-id", flow.id]
        return completed
    except WorkflowError as error:
        recovery = error.details.get("recovery", {})
        if flow.id and isinstance(recovery.get("args"), list) and "--invitation-flow-id" not in recovery["args"]:
            recovery["args"] += ["--invitation-flow-id", flow.id]
        raise
    finally:
        flow.associate()
        if completed and completed.get("nextAction") == "DEPLOY":
            flow.report("INSTALL_COMPLETED")


def _install(
    work_url: str,
    accepted_price_cents: Optional[int] = None,
    *,
    flow: InvitationFlow,
    target_path: Optional[str] = None,
    payment_presented: bool = False,
    replica_code: Optional[str] = None,
    recovery_only: bool = False,
    payment_result_first: bool = False,
    expected_order_no: Optional[str] = None,
    replace_unpaid_order: Optional[str] = None,
    request_fn: RequestFn = http_request,
    sleep_fn: Callable[[float], None] = time.sleep,
) -> Dict[str, Any]:
    authority = authority_for_work_url(work_url)
    if replace_unpaid_order and (recovery_only or payment_presented or payment_result_first or accepted_price_cents is None):
        raise WorkflowError("REPLICA_PAYMENT_REPLACEMENT_INVALID", "Replacing an unpaid order requires a new accepted payment request")
    if expected_order_no and not recovery_only:
        raise WorkflowError("REPLICA_RECOVERY_ONLY_CONFLICT", "Expected order requires recovery-only mode")
    if payment_result_first and not payment_presented:
        raise WorkflowError("REPLICA_PAYMENT_RESULT_INVALID", "Payment result requires the existing presented payment flow")
    if recovery_only:
        if accepted_price_cents is not None or payment_presented or payment_result_first:
            raise WorkflowError(
                "REPLICA_RECOVERY_ONLY_CONFLICT",
                "Recovery-only mode cannot accept a price or present payment",
            )
        if not isinstance(replica_code, str) or not INSTRUCTION_PATTERN.fullmatch(replica_code):
            raise WorkflowError(
                "REPLICA_RECOVERY_CODE_REQUIRED",
                "Recovery-only mode requires the platform-generated Replica instruction",
            )
        short_code = replica_code.split(":", 1)[1]
        target = resolve_target(target_path, short_code)
        store = state_store(authority, short_code, target)

        def recover_only() -> Dict[str, Any]:
            completion = read_state(store["completionFilename"])
            if completion is not None:
                if expected_order_no and completion.get("orderNo") != expected_order_no:
                    raise WorkflowError("REPLICA_PURCHASE_RECOVERY_CONFLICT", "Completed work does not match the original purchase")
                completed_target = Path(str(completion.get("target", "")))
                if not completed_target.is_dir():
                    raise WorkflowError(
                        "REPLICA_COMPLETION_TARGET_INVALID",
                        "Completed Replica target is unavailable",
                    )
                flow.use_saved(completion.get("invitationFlowId"))
                return {**completion, "nextAction": "DEPLOY"}
            # A normal post-payment continuation is bound to the original local
            # attempt, even if another target has replaced the shared code receipt.
            saved = read_state(store["filename"]) if expected_order_no else None
            if saved is not None:
                if (saved.get("schemaVersion") != 1 or saved.get("apiBaseUrl") != authority.api_base_url
                        or saved.get("shortCode") != short_code or saved.get("instruction") != replica_code
                        or saved.get("target") != str(target) or saved.get("orderNo") != expected_order_no
                        or not UUID_PATTERN.fullmatch(str(saved.get("replicaId", "")))
                        or not SECRET_PATTERN.fullmatch(str(saved.get("downloadRecoverySecret", "")))):
                    raise WorkflowError("REPLICA_PURCHASE_RECOVERY_CONFLICT", "Continuation does not match the original purchase")
                flow.bind(saved)
                receipt = {"replicaId": saved["replicaId"], "orderNo": saved["orderNo"], "recoverySecret": saved["downloadRecoverySecret"], "invitationFlowId": saved.get("invitationFlowId")}
            else:
                receipt = recoverable_paid_receipt_by_code(authority, short_code)
            if expected_order_no and (receipt is None or receipt.get("orderNo") != expected_order_no):
                raise WorkflowError("REPLICA_PURCHASE_RECOVERY_CONFLICT", "Continuation does not match the original purchase")
            if receipt is None:
                raise WorkflowError(
                    "REPLICA_RECOVERY_NOT_FOUND",
                    "No recoverable Website Replica entitlement exists for this code",
                )
            status = recover_order_status(
                authority, receipt["orderNo"], receipt["recoverySecret"], request_fn
            )
            if status["payment"]["status"] != "PAID":
                raise WorkflowError(
                    "REPLICA_RECOVERY_NOT_FOUND",
                    "No paid Website Replica entitlement can be recovered for this code",
                )
            flow.use_saved(receipt.get("invitationFlowId"))
            state = {
                "replicaId": receipt["replicaId"],
                **({"invitationFlowId": flow.id} if flow.id else {}),
                "orderNo": receipt["orderNo"],
                "downloadRecoverySecret": receipt["recoverySecret"],
                "target": str(target),
            }
            try:
                download = try_recover_download(authority, state, request_fn)
                if download is None:
                    raise WorkflowError(
                        "REPLICA_RECOVERY_NOT_FOUND",
                        "The paid Website Replica source is not recoverable yet",
                    )
                return {
                    **complete_install(authority, state, store, download, request_fn),
                    "nextAction": "DEPLOY",
                }
            except Exception as error:
                failure = error if isinstance(error, WorkflowError) else WorkflowError("MAKE_COPY_INTERNAL", "Paid work could not be prepared")
                raise WorkflowError(failure.code, failure.message,
                                    {**failure.details, "orderNo": state["orderNo"],
                                     "payment": {"status": "PAID", "paidAt": status["payment"].get("paidAt")},
                                     "stage": "INSTALL_REPLICA", "nextAction": "STOP_AND_REPORT",
                                     "recovery": {"mode": "RECOVERY_ONLY", "requiresUserRequest": True,
                                                  "args": ["install", "--work-url", authority.work_url, "--replica-code", replica_code,
                                                           "--recovery-only", "--expected-order-no", state["orderNo"], "--target", state["target"]]}}, failure.exit_code) from error

        return with_lock(store, recover_only)
    instruction, replica = resolve_work(authority, request_fn)
    target = resolve_target(target_path, replica["title"])
    store = state_store(authority, replica["shortCode"], target)

    def price_confirmation() -> WorkflowError:
        return WorkflowError(
            "REPLICA_PURCHASE_CONFIRMATION_REQUIRED",
            "Accept the displayed source price before creating or replacing an order",
            {"nextAction": "CONFIRM_PRICE", "replicaCode": instruction, "productId": replica["product"]["id"],
             "title": replica["title"], "currency": replica["product"]["currency"],
             "totalAmountCents": replica["product"]["priceCents"], "workUrl": replica["viceMeWorkUrl"], "target": str(target)},
            10,
        )

    def run() -> Dict[str, Any]:
        completion = read_state(store["completionFilename"])
        if completion is not None:
            completed_target = Path(str(completion.get("target", "")))
            if not completed_target.is_dir():
                raise WorkflowError(
                    "REPLICA_COMPLETION_TARGET_INVALID",
                    "Completed Replica target is unavailable",
                )
            flow.use_saved(completion.get("invitationFlowId"))
            return {**completion, "nextAction": "DEPLOY"}
        state = read_state(store["filename"])
        presented_order_no = state.get("orderNo") if state is not None else None
        if replace_unpaid_order and (state is None or state.get("orderNo") != replace_unpaid_order):
            raise WorkflowError("REPLICA_PURCHASE_RECOVERY_CONFLICT", "Payment replacement does not match the original attempt", {"nextAction": "STOP_AND_REPORT"})
        if state is not None:
            state = validate_state(state, authority, replica, target)
            flow.bind(state)
            if state.get("orderNo"):
                status = recover_order_status(
                    authority,
                    state["orderNo"],
                    state["downloadRecoverySecret"],
                    request_fn,
                )
                if status["payment"]["status"] == "PAID":
                    if payment_result_first and state.get("priceCents", 0) > 0:
                        return support_result(authority, state, replica, status["payment"], request_fn)
                    download = try_recover_download(authority, state, request_fn)
                    if not download:
                        raise WorkflowError(
                            "REPLICA_DOWNLOAD_PENDING",
                            "Paid Replica download is not available yet",
                        )
                    return {
                        **complete_install(
                            authority, state, store, download, request_fn
                        ),
                        "nextAction": "DEPLOY",
                    }
                if not payment_presented:
                    if not replace_unpaid_order:
                        raise payment_restart_required(authority, state, status["payment"]["status"])
                    if accepted_price_cents != replica["product"]["priceCents"]:
                        raise price_confirmation()
                    if status["payment"]["status"] == "PENDING":
                        cancel_order_attempt(authority, state["orderNo"], state["downloadRecoverySecret"], request_fn)
                    store["filename"].unlink(missing_ok=True)
                    receipt = read_state(store["paidReceiptFilename"])
                    if receipt and receipt.get("orderNo") == state["orderNo"]:
                        store["paidReceiptFilename"].unlink(missing_ok=True)
                    state = initial_state(authority, instruction, replica, target)
                    flow.bind(state)
                    persist_state(store, state)
                elif status["payment"]["status"] != "PENDING":
                    raise payment_restart_required(authority, state, status["payment"]["status"])
        else:
            if target.exists() or target.is_symlink():
                raise WorkflowError(
                    "REPLICA_TARGET_EXISTS",
                    "Refusing to overwrite target",
                    {"target": str(target)},
                )
            state = initial_state(authority, instruction, replica, target)
            flow.bind(state)
            persist_state(store, state)
        receipt = recoverable_paid_receipt(authority, replica)
        if not state.get("orderNo") and receipt:
            receipt_status = recover_order_status(authority, receipt["orderNo"], receipt["recoverySecret"], request_fn)
            if receipt_status["payment"]["status"] == "PAID":
                flow.use_saved(receipt.get("invitationFlowId"))
                state["orderNo"] = receipt["orderNo"]
                state["downloadRecoverySecret"] = receipt["recoverySecret"]
                persist_state(store, state)
            else:
                raise payment_restart_required(authority, {"orderNo": receipt["orderNo"]}, receipt_status["payment"]["status"])
        download = None if payment_result_first and state.get("orderNo") else try_recover_download(authority, state, request_fn)
        if download:
            return {
                **complete_install(authority, state, store, download, request_fn),
                "nextAction": "DEPLOY",
            }
        if accepted_price_cents is None and replica["product"]["priceCents"] > 0 and not (payment_presented and state.get("orderNo")):
            raise price_confirmation()
        if accepted_price_cents is not None and not (payment_presented and state.get("orderNo")) and replica["product"]["priceCents"] != accepted_price_cents:
            raise WorkflowError(
                "REPLICA_PRICE_CHANGED",
                "Replica price changed; show the Work again and ask for confirmation",
                {
                    "nextAction": "CONFIRM_PRICE",
                    "workUrl": replica["viceMeWorkUrl"],
                    "priceCents": replica["product"]["priceCents"],
                },
                10,
            )
        if not state.get("orderNo"):
            state["priceCents"] = replica["product"]["priceCents"]
            persist_state(store, state)
        if payment_presented and state.get("orderNo") and state.get("sessionId") and state.get("sessionToken"):
            checkout = {"orderNo": state["orderNo"], "status": "PENDING"}
        else:
            checkout = ensure_checkout(authority, state, store, request_fn)
        if checkout["status"] == "PAID":
            if payment_result_first and state.get("priceCents", 0) > 0:
                return support_result(authority, state, replica, {"status": "PAID"}, request_fn)
            download = try_recover_download(authority, state, request_fn)
            if not download:
                raise WorkflowError(
                    "REPLICA_DOWNLOAD_PENDING",
                    "Paid Replica download is not available yet",
                )
            return {
                **complete_install(authority, state, store, download, request_fn),
                "nextAction": "DEPLOY",
            }
        if checkout["status"] != "PENDING":
            raise WorkflowError(
                "REPLICA_PAYMENT_TERMINAL",
                "Website Replica payment did not complete",
            )
        # A new checkout cannot have been presented by a previous invocation.
        if not payment_presented or checkout["orderNo"] != presented_order_no:
            raise WorkflowError(
                "REPLICA_PAYMENT_REQUIRED",
                "Support the creator; present the local payment page inside the Agent platform",
                {
                    "nextAction": "PRESENT_PAYMENT_QR",
                    "presentationTarget": "AGENT_PLATFORM",
                    "paymentPresentation": payment_presentation(authority, replica, checkout, request_fn),
                },
                10,
            )
        try:
            payment = wait_for_payment(authority, state, request_fn, sleep_fn)
        except WorkflowError as error:
            if error.code not in {"REPLICA_PAYMENT_TIMEOUT", "REPLICA_PAYMENT_TERMINAL", "REPLICA_PAYMENT_INTERRUPTED"}:
                raise
            raise WorkflowError(error.code, error.message,
                                {**error.details, "nextAction": "STOP_AND_REPORT", "orderNo": state["orderNo"],
                                 "recovery": {"mode": "RECOVERY_ONLY", "requiresUserRequest": True,
                                              "args": ["install", "--work-url", authority.work_url, "--replica-code", state["instruction"],
                                                       "--recovery-only", "--expected-order-no", state["orderNo"], "--target", state["target"]]}}, error.exit_code) from error
        if payment_result_first and state.get("priceCents", 0) > 0:
            return support_result(authority, state, replica, payment, request_fn)
        download = try_recover_download(authority, state, request_fn)
        if not download:
            raise WorkflowError(
                "REPLICA_DOWNLOAD_PENDING",
                "Paid Replica download is not available yet",
            )
        return {
            **complete_install(authority, state, store, download, request_fn),
            "nextAction": "DEPLOY",
        }

    return with_lock(store, run)


def parse_args(argv: Optional[Iterable[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)
    subparsers.add_parser("flow-id")
    start_parser = subparsers.add_parser("start")
    start_parser.add_argument("--work-url", required=True)
    start_parser.add_argument("--invitation-flow-id")
    install_parser = subparsers.add_parser("install")
    install_parser.add_argument("--work-url", required=True)
    install_parser.add_argument("--target")
    install_parser.add_argument("--invitation-flow-id")
    install_parser.add_argument("--accept-price-cents", type=int)
    install_parser.add_argument("--payment-presented", action="store_true")
    install_parser.add_argument("--payment-result-first", action="store_true")
    install_parser.add_argument("--replica-code")
    install_parser.add_argument("--recovery-only", action="store_true")
    install_parser.add_argument("--expected-order-no")
    install_parser.add_argument("--replace-unpaid-order")
    args = parser.parse_args(argv)
    if args.command == "install" and args.accept_price_cents is not None and args.accept_price_cents < 0:
        parser.error("--accept-price-cents must be a non-negative integer")
    return args


def main(argv: Optional[Iterable[str]] = None) -> int:
    try:
        if sys.version_info < (3, 9):
            raise WorkflowError(
                "MAKE_COPY_PYTHON_UNSUPPORTED",
                "Python 3.9 or newer is required",
            )
        args = parse_args(argv)
        if args.command == "flow-id":
            data = {"invitationFlowId": str(uuid.uuid4())}
        elif args.command == "start":
            data = inspect(args.work_url, **({"invitation_flow_id": args.invitation_flow_id} if args.invitation_flow_id else {}))
        else:
            data = install(
                args.work_url,
                args.accept_price_cents,
                invitation_flow_id=args.invitation_flow_id,
                target_path=args.target,
                payment_presented=args.payment_presented,
                payment_result_first=args.payment_result_first,
                replica_code=args.replica_code,
                recovery_only=args.recovery_only,
                expected_order_no=args.expected_order_no,
                replace_unpaid_order=args.replace_unpaid_order,
            )
        result(data)
        return 0
    except Exception as error:  # protocol boundary
        return fail(error)


if __name__ == "__main__":
    raise SystemExit(main())

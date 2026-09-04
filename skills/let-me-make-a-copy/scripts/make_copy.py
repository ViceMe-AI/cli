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


def fetch_work_instruction(
    authority: Authority, request_fn: RequestFn = http_request
) -> str:
    response = request_fn(
        "GET",
        authority.work_url,
        headers={"Accept": "text/markdown"},
        timeout=15,
    )
    if not 200 <= response.status < 300:
        raise WorkflowError(
            "MAKE_COPY_WORK_UNAVAILABLE", "The ViceMe Work could not be read"
        )
    try:
        lines = response.body.decode("utf-8").splitlines()
    except UnicodeDecodeError:
        raise WorkflowError(
            "MAKE_COPY_WORK_UNAVAILABLE", "The ViceMe Work could not be read"
        )
    headings = {
        "## Platform-controlled complete-source replica entry",
        "## 平台控制的完整源码做同款入口",
    }
    try:
        section_start = next(i for i, line in enumerate(lines) if line in headings)
    except StopIteration:
        raise WorkflowError(
            "MAKE_COPY_ENTRY_INVALID",
            "The Work has no platform-controlled let-me-make-a-copy entry",
        )
    section_end = next(
        (
            i
            for i in range(section_start + 1, len(lines))
            if lines[i].startswith("## ")
        ),
        len(lines),
    )
    controlled = "\n".join(lines[section_start + 1 : section_end])
    instructions = sorted(set(INSTRUCTION_PATTERN.findall(controlled)))
    if len(instructions) != 1:
        raise WorkflowError(
            "MAKE_COPY_ENTRY_INVALID",
            "The Work must expose exactly one Replica instruction",
        )
    return instructions[0]


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
        and product.get("currency") == "CNY"
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
    instruction = fetch_work_instruction(authority, request_fn)
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
    return instruction, replica


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
    receipt = read_state(
        standalone_receipt_path(authority, replica["shortCode"], root)
    )
    if receipt is None:
        return None
    if (
        receipt.get("schemaVersion") != 1
        or receipt.get("replicaId") != replica["replicaId"]
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
        and value.get("priceCents") == replica["product"]["priceCents"]
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


def wait_for_payment(
    authority: Authority,
    state: Dict[str, Any],
    request_fn: RequestFn = http_request,
    sleep_fn: Callable[[float], None] = time.sleep,
) -> None:
    for _ in range(6):
        sleep_fn(30)
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
        if payment.get("status") == "PAID":
            return
        if payment.get("status") in {"CLOSED", "FAILED", "CANCELLED"}:
            raise WorkflowError(
                "REPLICA_PAYMENT_TERMINAL",
                "Website Replica payment did not complete",
                {"orderNo": state["orderNo"], "status": payment.get("status")},
            )
    raise WorkflowError(
        "REPLICA_PAYMENT_TIMEOUT",
        "Website Replica payment was not observed before the wait deadline",
        {"nextAction": "PAYMENT_PENDING", "orderNo": state["orderNo"]},
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
            "replicaId": download["replicaId"],
            "versionId": download["versionId"],
            "version": download["version"],
            "orderNo": state["orderNo"],
            "artifactDigest": download["artifactDigest"],
            "sizeBytes": download["sizeBytes"],
            "licenseJws": download["licenseJws"],
            "recoverySecret": state["downloadRecoverySecret"],
            "paidAt": claims["issuedAt"],
        },
    )
    installed = install_archive(archive_path, Path(state["target"]), download)
    completion = {
        **installed,
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


def inspect(
    work_url: str,
    *,
    request_fn: RequestFn = http_request,
    recovery_root: Optional[Path] = None,
) -> Dict[str, Any]:
    authority = authority_for_work_url(work_url)
    instruction, replica = resolve_work(authority, request_fn)
    receipt = recoverable_paid_receipt(authority, replica, recovery_root)
    recovery_available = False
    if receipt:
        status = recover_order_status(
            authority,
            receipt["orderNo"],
            receipt["recoverySecret"],
            request_fn,
        )
        recovery_available = status["payment"]["status"] == "PAID"
    return {
        "nextAction": "CONFIRM_INLINE_PREVIEW",
        "workUrl": replica["viceMeWorkUrl"],
        "instruction": instruction,
        "standaloneRecoveryAvailable": recovery_available,
        "replica": replica,
    }


def install(
    work_url: str,
    accepted_price_cents: int,
    *,
    target_path: Optional[str] = None,
    payment_presented: bool = False,
    request_fn: RequestFn = http_request,
    sleep_fn: Callable[[float], None] = time.sleep,
) -> Dict[str, Any]:
    authority = authority_for_work_url(work_url)
    instruction, replica = resolve_work(authority, request_fn)
    if replica["product"]["priceCents"] != accepted_price_cents:
        raise WorkflowError(
            "REPLICA_PRICE_CHANGED",
            "Replica price changed; show the Work again and ask for confirmation",
            {
                "nextAction": "CONFIRM_INLINE_PREVIEW",
                "workUrl": replica["viceMeWorkUrl"],
                "priceCents": replica["product"]["priceCents"],
            },
            10,
        )
    target = resolve_target(target_path, replica["title"])
    store = state_store(authority, replica["shortCode"], target)

    def run() -> Dict[str, Any]:
        completion = read_state(store["completionFilename"])
        if completion is not None:
            completed_target = Path(str(completion.get("target", "")))
            if not completed_target.is_dir():
                raise WorkflowError(
                    "REPLICA_COMPLETION_TARGET_INVALID",
                    "Completed Replica target is unavailable",
                )
            return {**completion, "nextAction": "DEPLOY"}
        state = read_state(store["filename"])
        if state is not None:
            state = validate_state(state, authority, replica, target)
            if state.get("orderNo"):
                status = recover_order_status(
                    authority,
                    state["orderNo"],
                    state["downloadRecoverySecret"],
                    request_fn,
                )
                if status["payment"]["status"] == "PAID":
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
                    if status["payment"]["status"] == "PENDING":
                        cancel_order_attempt(
                            authority,
                            state["orderNo"],
                            state["downloadRecoverySecret"],
                            request_fn,
                        )
                    store["filename"].unlink(missing_ok=True)
                    receipt = read_state(store["paidReceiptFilename"])
                    if receipt and receipt.get("orderNo") == state["orderNo"]:
                        store["paidReceiptFilename"].unlink(missing_ok=True)
                    state = initial_state(authority, instruction, replica, target)
                    persist_state(store, state)
        else:
            if target.exists() or target.is_symlink():
                raise WorkflowError(
                    "REPLICA_TARGET_EXISTS",
                    "Refusing to overwrite target",
                    {"target": str(target)},
                )
            state = initial_state(authority, instruction, replica, target)
            persist_state(store, state)
        receipt = recoverable_paid_receipt(authority, replica)
        if not state.get("orderNo") and receipt:
            state["orderNo"] = receipt["orderNo"]
            state["downloadRecoverySecret"] = receipt["recoverySecret"]
            persist_state(store, state)
        download = try_recover_download(authority, state, request_fn)
        if download:
            return {
                **complete_install(authority, state, store, download, request_fn),
                "nextAction": "DEPLOY",
            }
        checkout = ensure_checkout(authority, state, store, request_fn)
        if checkout["status"] == "PAID":
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
        if not payment_presented:
            raise WorkflowError(
                "REPLICA_PAYMENT_REQUIRED",
                "Open the hosted ViceMe payment page",
                {
                    "nextAction": "OPEN_PAYMENT_PAGE",
                    "checkoutUrl": checkout["checkoutUrl"],
                },
                10,
            )
        wait_for_payment(authority, state, request_fn, sleep_fn)
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
    start_parser = subparsers.add_parser("start")
    start_parser.add_argument("--work-url", required=True)
    install_parser = subparsers.add_parser("install")
    install_parser.add_argument("--work-url", required=True)
    install_parser.add_argument("--target")
    install_parser.add_argument("--accept-price-cents", required=True, type=int)
    install_parser.add_argument("--payment-presented", action="store_true")
    args = parser.parse_args(argv)
    if args.command == "install" and args.accept_price_cents < 0:
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
        if args.command == "start":
            data = inspect(args.work_url)
        else:
            data = install(
                args.work_url,
                args.accept_price_cents,
                target_path=args.target,
                payment_presented=args.payment_presented,
            )
        result(data)
        return 0
    except Exception as error:  # protocol boundary
        return fail(error)


if __name__ == "__main__":
    raise SystemExit(main())

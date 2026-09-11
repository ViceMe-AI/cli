import base64
import hashlib
import importlib.util
import json
import os
import stat
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest import mock


SCRIPT = (
    Path(__file__).parents[2]
    / "skills"
    / "let-me-make-a-copy"
    / "scripts"
    / "make_copy.py"
)
SPEC = importlib.util.spec_from_file_location("viceme_make_copy", SCRIPT)
make_copy = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = make_copy
SPEC.loader.exec_module(make_copy)


REPLICA_ID = "11111111-1111-4111-8111-111111111111"
VERSION_ID = "22222222-2222-4222-8222-222222222222"
ENTITLEMENT_ID = "33333333-3333-4333-8333-333333333333"
SHORT_CODE = "VMR-ABCDEFGHIJKLMNOPQRST"
ORDER_NO = "VMO-20260904-000001"
SECRET = "A" * 43


def response(status, value):
    body = value if isinstance(value, bytes) else json.dumps(value).encode()
    return make_copy.HttpResponse(status, body)


def replica():
    return {
        "replicaId": REPLICA_ID,
        "shortCode": SHORT_CODE,
        "title": "Replica",
        "creator": {"displayName": "Creator"},
        "viceMeWorkUrl": "https://viceme.cn/alice/site",
        "product": {
            "id": VERSION_ID,
            "skuId": ENTITLEMENT_ID,
            "title": "Replica",
            "currency": "CNY",
            "priceCents": 100,
        },
    }


def discovery(preview_url=None):
    work_url = "https://viceme.cn/alice/site"
    return {"replicaId": REPLICA_ID, "shortCode": SHORT_CODE,
            "title": "Replica", "summary": "Make a portfolio", "bodyMarkdown": "Useful for artists",
            "previewUrl": preview_url or work_url, "discoveryUrl": work_url + "/discover",
            "creator": {"handle": "alice", "displayName": "Creator"},
            "viceMeWorkUrl": work_url, "statistics": {"acquisitionCount": 2, "commentCount": 1}}


def public_work(is_hosted_page=None, presentation=None):
    work = {"websiteReplicaAction": {"instruction": f"VICEME-REPLICA:{SHORT_CODE}"}}
    if is_hosted_page is True:
        work["isHostedPage"] = True
    payload = {"work": work}
    if presentation is not None:
        payload["presentation"] = presentation
    return payload


def active_presentation(mode="ACTIVE"):
    return {
        "kind": "CUSTOM",
        "mode": mode,
        "releaseId": "99999999-9999-4999-8999-999999999999",
        "documentUrl": "https://viceme.cn/page-releases/test/index.html",
        "sdkVersion": "1",
        "capabilities": ["context.read"],
    }


def inspect_request(work=None, discovered=None, item=None):
    work = public_work() if work is None else work
    discovered = discovery() if discovered is None else discovered
    item = replica() if item is None else item

    def request(_method, url, **_kwargs):
        if "/public/creators/" in url:
            return response(200, work)
        if url.endswith("/website-replicas/resolve"):
            return response(200, item)
        if url.endswith("/discovery"):
            return response(200, discovered)
        raise AssertionError(url)

    return request


def checkout():
    return {"orderNo": ORDER_NO, "status": "PENDING", "checkoutUrl": "https://viceme.cn/replica-checkout/test",
            "expiresAt": "2099-09-04T00:00:00.000Z",
            "paymentAction": {"type": "QR_CODE", "content": "weixin://wxpay/bizpayurl?pr=test-only"}}


def order_view():
    return {**{key: value for key, value in checkout().items() if key != "checkoutUrl"}, "amountCents": 100, "currency": "CNY"}


def download():
    return {
        "replicaId": REPLICA_ID,
        "versionId": VERSION_ID,
        "version": 1,
        "artifactDigest": "a" * 64,
        "licenseJws": "header.payload.signature",
    }


def b64url(value):
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode()


def encode_point(point):
    x, y = point
    return (y | ((x & 1) << 255)).to_bytes(32, "little")


def sign_with_rfc8032_seed(message):
    seed = bytes.fromhex(
        "9d61b19deffd5a60ba844af492ec2cc4"
        "4449c5697b326919703bac031cae7f60"
    )
    expanded = hashlib.sha512(seed).digest()
    scalar = int.from_bytes(expanded[:32], "little")
    scalar &= (1 << 254) - 8
    scalar |= 1 << 254
    public_key = encode_point(make_copy._scalar_mult(make_copy._B, scalar))
    nonce = int.from_bytes(hashlib.sha512(expanded[32:] + message).digest(), "little")
    nonce %= make_copy._L
    encoded_r = encode_point(make_copy._scalar_mult(make_copy._B, nonce))
    challenge = int.from_bytes(
        hashlib.sha512(encoded_r + public_key + message).digest(), "little"
    ) % make_copy._L
    scalar_s = (nonce + challenge * scalar) % make_copy._L
    return public_key, encoded_r + scalar_s.to_bytes(32, "little")


class MakeCopyTest(unittest.TestCase):
    def test_discovery_rejects_cross_replica_and_unsafe_preview(self):
        authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
        for change in ({"replicaId": VERSION_ID}, {"previewUrl": "javascript:alert(1)"}):
            with self.subTest(change=change), self.assertRaises(make_copy.WorkflowError):
                make_copy.discovery(authority, replica(), lambda *_a, **_k: response(200, {**discovery(), **change}))

    def test_payment_uses_canonical_widget_and_private_encoded_artifacts(self):
        authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
        repository = SCRIPT.parents[3]
        def request(_method, url, **_kwargs):
            if url.endswith("/discovery"):
                return response(200, discovery())
            filename = url.rsplit("/", 1)[1]
            content = (repository / "widgets" / filename).read_bytes()
            self.assertIn(hashlib.sha256(content).hexdigest(), url)
            return response(200, content)
        with tempfile.TemporaryDirectory() as temporary, mock.patch.object(make_copy, "state_root", return_value=Path(temporary)):
            item = {**replica(), "title": "</script><script>alert(1)</script>"}
            display = make_copy.payment_presentation(authority, item, order_view(), request)
            html = Path(display["widgetPath"]).read_text()
            self.assertNotIn(item["title"], html)
            self.assertNotIn(checkout()["paymentAction"]["content"], html)
            self.assertNotIn('"supportCreator"', html)
            self.assertIn("推荐使用微信支付", html)
            self.assertNotIn("__QR_SVG__", html)
            self.assertTrue(Path(display["imagePath"]).read_bytes().startswith(b"\x89PNG"))
            self.assertNotIn("checkoutUrl", display)
            if os.name != "nt":
                self.assertEqual(stat.S_IMODE(Path(display["widgetPath"]).stat().st_mode), 0o600)

    def test_payment_resource_rejects_tampering_before_executing_encoder(self):
        authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
        with tempfile.TemporaryDirectory() as temporary, mock.patch.object(make_copy, "state_root", return_value=Path(temporary)):
            with self.assertRaises(make_copy.WorkflowError) as raised:
                make_copy.payment_resource(authority, "qrcodegen.py", lambda *_a, **_k: response(200, b"raise Exception('bad')"))
            self.assertEqual(raised.exception.code, "PAYMENT_RESOURCE_INVALID")

    def test_paid_first_install_checks_recovery_then_asks_price_without_checkout(self):
        with tempfile.TemporaryDirectory() as temporary, mock.patch.object(make_copy, "state_root", return_value=Path(temporary) / "state"), mock.patch.object(
            make_copy, "resolve_work", return_value=(f"VICEME-REPLICA:{SHORT_CODE}", replica())
        ), mock.patch.object(make_copy, "try_recover_download", return_value=None) as recovery, mock.patch.object(
            make_copy, "ensure_checkout", side_effect=AssertionError("order before consent")
        ), self.assertRaises(make_copy.WorkflowError) as raised:
            make_copy.install("https://viceme.cn/alice/site.md", target_path=str(Path(temporary) / "copy"))
        recovery.assert_called_once()
        self.assertEqual(raised.exception.code, "REPLICA_PURCHASE_CONFIRMATION_REQUIRED")
        self.assertEqual(raised.exception.details["nextAction"], "CONFIRM_PRICE")
        self.assertEqual(raised.exception.details["totalAmountCents"], 100)

    def test_recovery_only_uses_direct_code_without_public_discovery_or_checkout(self):
        receipt = {"schemaVersion": 1, "replicaId": REPLICA_ID, "orderNo": ORDER_NO, "recoverySecret": SECRET}
        with tempfile.TemporaryDirectory() as temporary, mock.patch.object(
            make_copy, "state_root", return_value=Path(temporary) / "state"
        ), mock.patch.object(
            make_copy, "resolve_work", side_effect=AssertionError("public discovery")
        ), mock.patch.object(
            make_copy, "recoverable_paid_receipt_by_code", return_value=receipt
        ), mock.patch.object(
            make_copy, "recover_order_status", return_value={"payment": {"status": "PAID"}}
        ), mock.patch.object(
            make_copy, "try_recover_download", return_value={"replicaId": REPLICA_ID}
        ), mock.patch.object(
            make_copy, "complete_install", return_value={"target": str(Path(temporary) / "copy")}
        ), mock.patch.object(
            make_copy, "ensure_checkout", side_effect=AssertionError("new checkout")
        ):
            installed = make_copy.install(
                "https://viceme.cn/alice/site.md",
                target_path=str(Path(temporary) / "copy"),
                replica_code=f"VICEME-REPLICA:{SHORT_CODE}",
                recovery_only=True,
            )
        self.assertEqual(installed["nextAction"], "DEPLOY")

    def test_global_free_resolution_preserves_usd(self):
        value = replica()
        value["viceMeWorkUrl"] = "https://viceme.ai/alice/site"
        value["product"].update(currency="USD", priceCents=0)
        self.assertEqual(make_copy.assert_resolution(value)["product"], value["product"])
        value["product"]["currency"] = "EUR"
        with self.assertRaises(make_copy.WorkflowError):
            make_copy.assert_resolution(value)

    def test_free_install_does_not_require_price_acceptance(self):
        for host, currency in [("viceme.cn", "CNY"), ("viceme.ai", "USD")]:
            free = replica()
            free["product"].update(priceCents=0, currency=currency)
            free["viceMeWorkUrl"] = f"https://{host}/alice/site"
            with tempfile.TemporaryDirectory() as temporary, mock.patch.object(make_copy, "state_root", return_value=Path(temporary) / "state"), mock.patch.object(
                make_copy, "resolve_work", return_value=(f"VICEME-REPLICA:{SHORT_CODE}", free)
            ), mock.patch.object(make_copy, "try_recover_download", side_effect=[None, download()]), mock.patch.object(
                make_copy, "ensure_checkout", return_value={**checkout(), "status": "PAID"}
            ), mock.patch.object(make_copy, "complete_install", return_value={"target": str(Path(temporary) / "copy")}), mock.patch.object(
                make_copy, "payment_presentation", side_effect=AssertionError("payment for free work")
            ):
                result = make_copy.install(f"https://{host}/alice/site.md", target_path=str(Path(temporary) / "copy"))
            self.assertEqual(result["nextAction"], "DEPLOY")

    def test_delisted_public_work_retains_authoritative_discovery_and_recovery_entry(self):
        authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
        work = {"work": {"kind": "WEBSITE", "status": "PUBLISHED", "websiteReplica": {"shortCode": SHORT_CODE, "availability": "DELISTED"}}}
        self.assertEqual(make_copy.fetch_work_instruction(authority, lambda *_a, **_k: response(200, work)), "VICEME-REPLICA:" + SHORT_CODE)
        work["work"]["websiteReplica"]["shortCode"] = "invalid-code"
        with self.assertRaises(make_copy.WorkflowError):
            make_copy.fetch_work_instruction(authority, lambda *_a, **_k: response(200, work))

    def test_payment_rejects_raw_checkout_without_authoritative_amount(self):
        authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
        with self.assertRaises(make_copy.WorkflowError) as raised:
            make_copy.payment_presentation(authority, replica(), checkout(), lambda *_a, **_k: self.fail("network before validation"))
        self.assertEqual(raised.exception.code, "MAKE_COPY_RESPONSE_INVALID")

    def test_recovery_only_keeps_unpaid_order_until_price_is_accepted(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            target = root / "copy"
            authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
            with mock.patch.object(make_copy, "state_root", return_value=root / "state"):
                store = make_copy.state_store(authority, SHORT_CODE, target)
                state = make_copy.initial_state(authority, f"VICEME-REPLICA:{SHORT_CODE}", replica(), target)
                state.update(orderNo=ORDER_NO, sessionId=VERSION_ID, sessionToken="token")
                make_copy.persist_state(store, state)
                with mock.patch.object(make_copy, "resolve_work", return_value=(state["instruction"], replica())), mock.patch.object(
                    make_copy, "recover_order_status", return_value={"payment": {"status": "PENDING"}}
                ), mock.patch.object(make_copy, "try_recover_download", side_effect=AssertionError("download before paid")), mock.patch.object(
                    make_copy, "ensure_checkout", side_effect=AssertionError("checkout during recovery check")
                ), mock.patch.object(make_copy, "cancel_order_attempt", side_effect=AssertionError("cancel before consent")), self.assertRaises(make_copy.WorkflowError) as raised:
                    make_copy.install(authority.work_url, target_path=str(target))
                self.assertEqual(raised.exception.code, "REPLICA_PAYMENT_RESTART_REQUIRED")
                self.assertEqual(make_copy.read_state(store["filename"])["orderNo"], ORDER_NO)

    def test_pending_receipt_for_another_target_does_not_create_or_cancel_without_consent(self):
        receipt = {"schemaVersion": 1, "replicaId": REPLICA_ID, "orderNo": ORDER_NO, "recoverySecret": SECRET}
        for current_price in (0, 100):
            current = replica()
            current["product"]["priceCents"] = current_price
            with self.subTest(price=current_price), tempfile.TemporaryDirectory() as temporary, mock.patch.object(
                make_copy, "state_root", return_value=Path(temporary) / "state"
            ), mock.patch.object(make_copy, "resolve_work", return_value=(f"VICEME-REPLICA:{SHORT_CODE}", current)), mock.patch.object(
                make_copy, "recoverable_paid_receipt", return_value=receipt
            ), mock.patch.object(make_copy, "recover_order_status", return_value={"payment": {"status": "PENDING"}}), mock.patch.object(
                make_copy, "cancel_order_attempt", side_effect=AssertionError("cancel before consent")
            ), mock.patch.object(make_copy, "ensure_checkout", side_effect=AssertionError("new order before consent")), mock.patch.object(
                make_copy, "try_recover_download", side_effect=AssertionError("download pending order")
            ), self.assertRaises(make_copy.WorkflowError) as raised:
                make_copy.install("https://viceme.cn/alice/site.md", target_path=str(Path(temporary) / "new-copy"))
            self.assertEqual(raised.exception.code, "REPLICA_PAYMENT_RESTART_REQUIRED")
            self.assertEqual(raised.exception.details["nextAction"], "STOP_AND_REPORT")

    def test_start_is_the_single_public_preview_entrypoint(self):
        work_url = "https://viceme.cn/alice/site.md"
        args = make_copy.parse_args(["start", "--work-url", work_url])
        self.assertEqual(args.command, "start")
        self.assertEqual(args.work_url, work_url)

        preview = {"nextAction": "PRESENT_WORK", "workUrl": work_url}
        with mock.patch.object(
            make_copy, "inspect", return_value=preview
        ) as inspect, mock.patch.object(make_copy, "result") as result:
            self.assertEqual(make_copy.main(["start", "--work-url", work_url]), 0)
        inspect.assert_called_once_with(work_url)
        result.assert_called_once_with(preview)

    def test_accepts_only_official_work_markdown_authorities(self):
        authority = make_copy.authority_for_work_url(
            "https://viceme.cn/alice/site.md"
        )
        self.assertEqual(authority.api_base_url, "https://viceme.cn/api/v1")
        with self.assertRaisesRegex(
            make_copy.WorkflowError, "official ViceMe HTTPS"
        ) as raised:
            make_copy.authority_for_work_url("https://example.com/alice/site.md")
        self.assertEqual(raised.exception.code, "MAKE_COPY_WORK_URL_INVALID")

    def test_resolves_instruction_from_the_public_work_api(self):
        authority = make_copy.authority_for_work_url(
            "https://viceme.cn/alice/site.md"
        )

        def request(method, url, **_kwargs):
            self.assertEqual(method, "GET")
            self.assertEqual(
                url,
                "https://viceme.cn/api/v1/public/creators/alice/works/site",
            )
            return response(
                200,
                {
                    "work": {
                        "websiteReplicaAction": {
                            "instruction": f"VICEME-REPLICA:{SHORT_CODE}"
                        }
                    }
                },
            )

        self.assertEqual(
            make_copy.fetch_work_instruction(authority, request),
            f"VICEME-REPLICA:{SHORT_CODE}",
        )

    def test_rejects_work_without_structured_replica_entry(self):
        authority = make_copy.authority_for_work_url(
            "https://viceme.cn/alice/site.md"
        )

        with self.assertRaises(make_copy.WorkflowError) as raised:
            make_copy.fetch_work_instruction(
                authority, lambda *_args, **_kwargs: response(200, {"work": {}})
            )

        self.assertEqual(raised.exception.code, "MAKE_COPY_ENTRY_INVALID")

    def test_preview_never_reads_private_recovery_or_queries_order(self):
        work_url = "https://viceme.cn/alice/site.md"
        with mock.patch.object(
            make_copy, "resolve_work_with_hosting",
            return_value=(f"VICEME-REPLICA:{SHORT_CODE}", replica(), False),
        ), mock.patch.object(
            make_copy, "read_state", side_effect=AssertionError("private state read")
        ), mock.patch.object(
            make_copy, "recover_order_status", side_effect=AssertionError("order query")
        ):
            inspected = make_copy.inspect(work_url, request_fn=lambda *_args, **_kwargs: response(200, discovery()))
        self.assertEqual(inspected["nextAction"], "PRESENT_WORK")
        self.assertEqual(inspected["workPresentation"], {"mode": "WORKSPACE_TEXT"})
        self.assertNotIn("standaloneRecoveryAvailable", inspected)

    def test_inspect_selects_work_presentation(self):
        work_url = "https://viceme.cn/alice/site.md"
        official = "https://viceme.cn/alice/site"
        cases = (
            ("active hosted page", public_work(presentation=active_presentation()), None,
             {"mode": "CREATOR_PAGE", "url": official}),
            ("replica publish without isHostedPage", public_work(presentation=active_presentation()), None,
             {"mode": "CREATOR_PAGE", "url": official}),
            ("isHostedPage without active presentation", public_work(True), None, {"mode": "WORKSPACE_TEXT"}),
            ("verified creator site without hosted page", public_work(), "https://original.example.com",
             {"mode": "WORKSPACE_TEXT"}),
            ("hosted page prefers work url over external preview",
             public_work(presentation=active_presentation()), "https://original.example.com",
             {"mode": "CREATOR_PAGE", "url": official}),
            ("preview presentation is not public hosted page",
             public_work(presentation=active_presentation("PREVIEW")), None, {"mode": "WORKSPACE_TEXT"}),
            ("ordinary work", public_work(False), None, {"mode": "WORKSPACE_TEXT"}),
            ("unknown hosted flag", public_work(), None, {"mode": "WORKSPACE_TEXT"}),
        )
        for name, work, preview_url, expected in cases:
            with self.subTest(name):
                inspected = make_copy.inspect(
                    work_url,
                    request_fn=inspect_request(work=work, discovered=discovery(preview_url)),
                )
                self.assertEqual(inspected["nextAction"], "PRESENT_WORK")
                self.assertEqual(inspected["workPresentation"], expected)
                self.assertEqual(inspected["discovery"]["previewUrl"], preview_url or official)

    def test_fetch_public_work_entry_uses_active_presentation(self):
        authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
        instruction, active = make_copy.fetch_public_work_entry(
            authority, lambda *_args, **_kwargs: response(200, public_work(presentation=active_presentation()))
        )
        self.assertEqual(instruction, f"VICEME-REPLICA:{SHORT_CODE}")
        self.assertTrue(active)
        instruction, active = make_copy.fetch_public_work_entry(
            authority, lambda *_args, **_kwargs: response(200, public_work(True))
        )
        self.assertEqual(instruction, f"VICEME-REPLICA:{SHORT_CODE}")
        self.assertFalse(active)
        instruction, active = make_copy.fetch_public_work_entry(
            authority, lambda *_args, **_kwargs: response(200, public_work())
        )
        self.assertEqual(instruction, f"VICEME-REPLICA:{SHORT_CODE}")
        self.assertFalse(active)

    def test_confirmed_install_recovers_paid_order_without_checkout(self):
        with tempfile.TemporaryDirectory() as temporary:
            target = Path(temporary) / "copy"
            authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
            store = {"filename": Path(temporary) / "state.json", "completionFilename": Path(temporary) / "complete.json"}
            state = {"orderNo": ORDER_NO, "downloadRecoverySecret": SECRET}
            with mock.patch.object(make_copy, "resolve_work", return_value=("instruction", replica())), mock.patch.object(
                make_copy, "state_store", return_value=store
            ), mock.patch.object(make_copy, "with_lock", side_effect=lambda _store, run: run()), mock.patch.object(
                make_copy, "read_state", side_effect=[None, state]
            ), mock.patch.object(make_copy, "validate_state", return_value=state), mock.patch.object(
                make_copy, "recover_order_status", return_value={"payment": {"status": "PAID"}}
            ) as status, mock.patch.object(make_copy, "try_recover_download", return_value={"download": True}), mock.patch.object(
                make_copy, "complete_install", return_value={"target": str(target)}
            ), mock.patch.object(make_copy, "ensure_checkout", side_effect=AssertionError("new checkout")):
                installed = make_copy.install(authority.work_url, 0, target_path=str(target))
            status.assert_called_once()
            self.assertEqual(installed["nextAction"], "DEPLOY")

    def test_order_number_alone_does_not_authorize_attempt_cancellation(self):
        authority = make_copy.authority_for_work_url(
            "https://viceme.cn/alice/site.md"
        )

        def request(method, url, **kwargs):
            self.assertTrue(url.endswith("/cancel-order"))
            self.assertEqual(
                json.loads(kwargs["body"]),
                {"orderNo": ORDER_NO, "recoverySecret": SECRET},
            )
            return response(
                200,
                {
                    "orderNo": ORDER_NO,
                    "payment": {
                        "status": "CLOSED",
                        "paidAt": None,
                        "closedAt": "2026-09-04T00:01:00.000Z",
                    },
                    "fulfillment": None,
                },
            )

        make_copy.cancel_order_attempt(authority, ORDER_NO, SECRET, request)

    def test_installs_bounded_archive_and_records_license(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / "source.zip"
            target = root / "copy"
            with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as output:
                output.writestr("VICEME-REPLICA.md", "# Deploy\n\nRun build.\n")
                output.writestr("src/index.js", "console.log('ok');\n")
            installed = make_copy.install_archive(archive, target, download())
            self.assertEqual(installed["fileCount"], 2)
            self.assertEqual(
                (target / "src" / "index.js").read_text(), "console.log('ok');\n"
            )
            self.assertIn(
                "header.payload.signature",
                (target / ".viceme" / "replica-license.json").read_text(),
            )

    def test_rejects_parent_path_in_zip(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / "source.zip"
            with zipfile.ZipFile(archive, "w") as output:
                output.writestr("VICEME-REPLICA.md", "# Deploy\n")
                output.writestr("../escape.txt", "escape")
            with self.assertRaises(make_copy.WorkflowError) as raised:
                make_copy.install_archive(archive, root / "copy", download())
            self.assertEqual(raised.exception.code, "REPLICA_ARCHIVE_INVALID")

    def test_verifies_platform_jws_and_exact_purchase(self):
        header = b64url(
            json.dumps(
                {
                    "alg": "EdDSA",
                    "kid": "test-key",
                    "typ": "viceme-replica-license+jws",
                },
                separators=(",", ":"),
            ).encode()
        )
        claims = b64url(
            json.dumps(
                {
                    "schemaVersion": "website-replica-license/v2",
                    "entitlementId": ENTITLEMENT_ID,
                    "replicaId": REPLICA_ID,
                    "versionId": VERSION_ID,
                    "version": 1,
                    "orderNo": ORDER_NO,
                    "artifactDigest": "a" * 64,
                    "licenseTermsVersion": "website-replica-license/v1",
                    "issuedAt": "2026-09-04T00:00:00.000Z",
                },
                separators=(",", ":"),
            ).encode()
        )
        public_key, signature = sign_with_rfc8032_seed(f"{header}.{claims}".encode())
        license_jws = f"{header}.{claims}.{b64url(signature)}"
        purchased = download()
        purchased["licenseJws"] = license_jws

        def request(*_args, **_kwargs):
            return response(
                200,
                {
                    "keyId": "test-key",
                    "algorithm": "Ed25519",
                    "publicKey": (
                        b64url(make_copy.ED25519_SPKI_PREFIX + public_key)
                    ),
                },
            )

        claims = make_copy.verify_license(
            make_copy.Authority("", "", "https://viceme.cn/api/v1"),
            purchased,
            {"replicaId": REPLICA_ID, "orderNo": ORDER_NO},
            request,
        )
        self.assertEqual(claims["entitlementId"], ENTITLEMENT_ID)

    def test_rfc8032_empty_message_vector(self):
        public_key = bytes.fromhex(
            "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a"
        )
        signature = bytes.fromhex(
            "e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e06522490155"
            "5fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b"
        )
        self.assertTrue(make_copy.verify_ed25519(public_key, b"", signature))
        self.assertFalse(make_copy.verify_ed25519(public_key, b"changed", signature))

    def test_waits_before_each_bounded_payment_check(self):
        sleeps = []
        requests = []

        def request(*_args, **_kwargs):
            requests.append(True)
            return response(200, {"orderNo": "order", "payment": {"status": "PENDING"}})

        with self.assertRaises(make_copy.WorkflowError) as raised:
            make_copy.wait_for_payment(
                make_copy.Authority("", "", "https://viceme.cn/api/v1"),
                {"sessionId": "session", "sessionToken": "token", "orderNo": "order"},
                request,
                sleeps.append,
            )
        self.assertEqual(raised.exception.code, "REPLICA_PAYMENT_TIMEOUT")
        self.assertEqual(sleeps, [3] * 60)
        self.assertEqual(len(requests), 60)

    def test_payment_is_detected_on_the_next_poll_without_waiting_for_timeout(self):
        sleeps = []
        replies = iter(["PENDING", "PAID"])
        def request(*_args, **_kwargs):
            return response(200, {"orderNo": "order", "payment": {"status": next(replies)}})
        make_copy.wait_for_payment(
            make_copy.Authority("", "", "https://viceme.cn/api/v1"),
            {"sessionId": "session", "sessionToken": "token", "orderNo": "order"},
            request, sleeps.append,
        )
        self.assertEqual(sleeps, [3, 3])

    def test_timeout_repeats_keep_original_order_and_late_payment_recovers(self):
        for terminal in (False, True):
            with self.subTest(terminal=terminal), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary).resolve()
                target = root / "copy"
                authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
                with mock.patch.object(make_copy, "state_root", return_value=root / "state"):
                    store = make_copy.state_store(authority, SHORT_CODE, target)
                    state = make_copy.initial_state(authority, f"VICEME-REPLICA:{SHORT_CODE}", replica(), target)
                    state.update(orderNo=ORDER_NO, sessionId=VERSION_ID, sessionToken="token")
                    make_copy.persist_state(store, state)
                    with mock.patch.object(make_copy, "resolve_work", return_value=(state["instruction"], replica())), mock.patch.object(
                        make_copy, "recover_order_status", return_value={"payment": {"status": "PENDING"}}
                    ) as status, mock.patch.object(make_copy, "ensure_checkout", side_effect=AssertionError("another checkout after timeout")), mock.patch.object(
                        make_copy, "cancel_order_attempt", side_effect=AssertionError("automatic cancellation after timeout")
                    ), mock.patch.object(make_copy, "try_recover_download", return_value=download()), mock.patch.object(
                        make_copy, "complete_install", return_value={"orderNo": ORDER_NO, "target": str(target)}
                    ) as complete:
                        with self.assertRaises(make_copy.WorkflowError) as timed_out:
                            make_copy.install(authority.work_url, 100, target_path=str(target), payment_presented=True,
                                              payment_result_first=True, sleep_fn=lambda _: None,
                                              request_fn=lambda *_a, **_k: response(200, {"orderNo": ORDER_NO, "payment": {"status": "PENDING"}}))
                        self.assertEqual(timed_out.exception.code, "REPLICA_PAYMENT_TIMEOUT")
                        self.assertEqual(timed_out.exception.details["nextAction"], "STOP_AND_REPORT")
                        for price in (None, 100):
                            with self.assertRaises(make_copy.WorkflowError) as repeated:
                                make_copy.install(authority.work_url, price, target_path=str(target))
                            self.assertEqual(repeated.exception.code, "REPLICA_PAYMENT_RESTART_REQUIRED")
                        if terminal:
                            status.return_value = {"payment": {"status": "CLOSED"}}
                            for presented in (True, False):
                                with self.assertRaises(make_copy.WorkflowError) as closed:
                                    make_copy.install(authority.work_url, 100, target_path=str(target), payment_presented=presented)
                                self.assertEqual(closed.exception.code, "REPLICA_PAYMENT_RESTART_REQUIRED")
                            self.assertEqual(make_copy.read_state(store["filename"])["orderNo"], ORDER_NO)
                            complete.assert_not_called()
                        else:
                            status.return_value = {"payment": {"status": "PAID"}}
                            args = make_copy.parse_args(timed_out.exception.details["recovery"]["args"])
                            result = make_copy.install(args.work_url, target_path=args.target, replica_code=args.replica_code,
                                                       recovery_only=True, expected_order_no=args.expected_order_no)
                            self.assertEqual(result["nextAction"], "DEPLOY")
                            self.assertEqual(result["orderNo"], ORDER_NO)

    def test_explicit_order_replacement_cannot_be_replayed_for_another_order(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            target = root / "copy"
            authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
            with mock.patch.object(make_copy, "state_root", return_value=root / "state"):
                store = make_copy.state_store(authority, SHORT_CODE, target)
                state = make_copy.initial_state(authority, f"VICEME-REPLICA:{SHORT_CODE}", replica(), target)
                state.update(orderNo=ORDER_NO, sessionId=VERSION_ID, sessionToken="token")
                make_copy.persist_state(store, state)
                def new_checkout(_authority, attempt, current_store, _request):
                    attempt.update(orderNo="VMO-SECOND", sessionId=VERSION_ID, sessionToken="new-token")
                    make_copy.persist_state(current_store, attempt)
                    return {**checkout(), "orderNo": "VMO-SECOND"}
                with mock.patch.object(make_copy, "resolve_work", return_value=(state["instruction"], replica())), mock.patch.object(
                    make_copy, "recover_order_status", return_value={"payment": {"status": "PENDING"}}
                ), mock.patch.object(make_copy, "cancel_order_attempt") as cancel, mock.patch.object(
                    make_copy, "ensure_checkout", side_effect=new_checkout
                ) as checkout_call, mock.patch.object(make_copy, "try_recover_download", return_value=None), mock.patch.object(
                    make_copy, "payment_presentation", return_value={"widgetPath": "test.html"}
                ):
                    with self.assertRaises(make_copy.WorkflowError) as fresh:
                        make_copy.install(authority.work_url, 100, target_path=str(target), replace_unpaid_order=ORDER_NO)
                    self.assertEqual(fresh.exception.code, "REPLICA_PAYMENT_REQUIRED")
                    with self.assertRaises(make_copy.WorkflowError) as repeat:
                        make_copy.install(authority.work_url, 100, target_path=str(target), replace_unpaid_order=ORDER_NO)
                    self.assertEqual(repeat.exception.code, "REPLICA_PURCHASE_RECOVERY_CONFLICT")
                    cancel.assert_called_once()
                    checkout_call.assert_called_once()
                    self.assertEqual(make_copy.read_state(store["filename"])["orderNo"], "VMO-SECOND")

    def test_payment_presented_cannot_skip_a_new_checkout_page(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            def checkout(_authority, state, _store, _request):
                state.update(orderNo=ORDER_NO, sessionId=VERSION_ID, sessionToken="token")
                return globals()["checkout"]()
            sleeps = []
            with mock.patch.object(make_copy, "state_root", return_value=root / "state"), mock.patch.object(
                make_copy, "resolve_work", return_value=(f"VICEME-REPLICA:{SHORT_CODE}", replica())
            ), mock.patch.object(make_copy, "try_recover_download", return_value=None), mock.patch.object(
                make_copy, "ensure_checkout", side_effect=checkout
            ), mock.patch.object(make_copy, "payment_presentation", return_value={"widgetPath": "/tmp/payment.html"}), self.assertRaises(make_copy.WorkflowError) as raised:
                make_copy.install(
                    "https://viceme.cn/alice/site.md", 100, target_path=str(root / "copy"),
                    payment_presented=True, sleep_fn=sleeps.append,
                    request_fn=lambda *_args, **_kwargs: response(200, {"payment": {"status": "PENDING"}}),
                )
            self.assertEqual(raised.exception.code, "REPLICA_PAYMENT_REQUIRED")
            self.assertEqual(raised.exception.details["nextAction"], "PRESENT_PAYMENT_QR")
            self.assertEqual(raised.exception.details["presentationTarget"], "AGENT_PLATFORM")
            self.assertEqual(sleeps, [])

    def test_presented_existing_order_installs_as_soon_as_payment_is_detected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            target = root / "copy"
            authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
            sleeps = []
            replies = iter(["PENDING", "PAID"])
            def request(*_args, **_kwargs):
                return response(200, {"orderNo": ORDER_NO, "payment": {"status": next(replies)}})
            with mock.patch.object(make_copy, "state_root", return_value=root / "state"):
                store = make_copy.state_store(authority, SHORT_CODE, target)
                state = make_copy.initial_state(authority, f"VICEME-REPLICA:{SHORT_CODE}", replica(), target)
                state.update(orderNo=ORDER_NO, sessionId=VERSION_ID, sessionToken="token")
                make_copy.persist_state(store, state)
                with mock.patch.object(make_copy, "resolve_work", return_value=(state["instruction"], replica())), mock.patch.object(
                    make_copy, "recover_order_status", return_value={"payment": {"status": "PENDING"}}
                ), mock.patch.object(make_copy, "try_recover_download", side_effect=[None, download()]), mock.patch.object(
                    make_copy, "ensure_checkout", return_value={"orderNo": ORDER_NO, "status": "PENDING", "checkoutUrl": "https://viceme.cn/replica-checkout/test"}
                ), mock.patch.object(make_copy, "complete_install", return_value={"target": str(target)}) as complete:
                    result = make_copy.install(authority.work_url, 100, target_path=str(target), payment_presented=True, sleep_fn=sleeps.append, request_fn=request)
            self.assertEqual(result["nextAction"], "DEPLOY")
            self.assertEqual(sleeps, [3, 3])
            complete.assert_called_once()

    def test_support_result_precedes_delivery_and_continues_without_checkout(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            target = root / "copy"
            authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
            payment = {"status": "PAID", "paidAt": "2026-09-10T00:00:00Z"}
            template = (SCRIPT.parents[3] / "widgets/payment.html").read_bytes()
            with mock.patch.object(make_copy, "state_root", return_value=root / "state"):
                store = make_copy.state_store(authority, SHORT_CODE, target)
                state = make_copy.initial_state(authority, f"VICEME-REPLICA:{SHORT_CODE}", replica(), target)
                state.update(orderNo=ORDER_NO, sessionId=VERSION_ID, sessionToken="token", orderAmountCents=70, orderCurrency="CNY")
                make_copy.persist_state(store, state)
                make_copy.atomic_private_write(store["paidReceiptFilename"], {"schemaVersion": 1, "replicaId": REPLICA_ID,
                                             "orderNo": ORDER_NO, "recoverySecret": state["downloadRecoverySecret"]})
                with mock.patch.object(make_copy, "resolve_work", return_value=(state["instruction"], replica())), mock.patch.object(
                    make_copy, "recover_order_status", return_value={"payment": {"status": "PENDING"}}
                ), mock.patch.object(make_copy, "try_recover_download", side_effect=AssertionError("delivery before result")), mock.patch.object(
                    make_copy, "complete_install", side_effect=AssertionError("installation before result")
                ), mock.patch.object(make_copy, "ensure_checkout", return_value={"orderNo": ORDER_NO, "status": "PENDING"}), mock.patch.object(
                    make_copy, "payment_resource", return_value=template
                ):
                    result = make_copy.install(authority.work_url, 100, target_path=str(target), payment_presented=True,
                                               payment_result_first=True, sleep_fn=lambda _: None,
                                               request_fn=lambda *_a, **_k: response(200, {"orderNo": ORDER_NO, "payment": payment}))
                self.assertEqual(result["nextAction"], "PRESENT_SUPPORT_RESULT")
                self.assertEqual(result["payment"], payment)
                self.assertEqual(result["amountCents"], 70)
                self.assertFalse(target.exists())
                display = result["presentation"]
                self.assertNotEqual(display["widgetPath"], display["replacesWidgetPath"])
                html = Path(display["widgetPath"]).read_text()
                self.assertIn('"status": "PAID"', html)
                self.assertNotIn("weixin://", html)
                self.assertNotIn(state["downloadRecoverySecret"], json.dumps(result))
                args = make_copy.parse_args(result["continuation"]["args"])
                self.assertTrue(args.recovery_only)
                self.assertIsNone(args.accept_price_cents)
                self.assertFalse(args.payment_result_first)
                self.assertEqual(args.expected_order_no, ORDER_NO)
                with self.assertRaises(make_copy.WorkflowError) as mismatch:
                    make_copy.install(args.work_url, target_path=args.target, replica_code=args.replica_code,
                                      recovery_only=True, expected_order_no="VMO-OTHER")
                self.assertEqual(mismatch.exception.code, "REPLICA_PURCHASE_RECOVERY_CONFLICT")
                # Another target's shared receipt must not redirect this continuation.
                make_copy.atomic_private_write(store["paidReceiptFilename"], {"schemaVersion": 1, "replicaId": REPLICA_ID,
                                             "orderNo": "VMO-OTHER", "recoverySecret": SECRET})
                with mock.patch.object(make_copy, "resolve_work", side_effect=AssertionError("public discovery during continuation")), mock.patch.object(
                    make_copy, "ensure_checkout", side_effect=AssertionError("checkout during continuation")
                ), mock.patch.object(make_copy, "recover_order_status", return_value={"payment": payment}), mock.patch.object(
                    make_copy, "try_recover_download", return_value=download()
                ), mock.patch.object(make_copy, "complete_install", side_effect=make_copy.WorkflowError("REPLICA_ARCHIVE_INVALID", "Invalid archive")) as complete:
                    with self.assertRaises(make_copy.WorkflowError) as raised:
                        make_copy.install(args.work_url, target_path=args.target, replica_code=args.replica_code, recovery_only=True, expected_order_no=args.expected_order_no)
                    self.assertEqual(raised.exception.details["payment"], payment)
                    self.assertEqual(raised.exception.details["nextAction"], "STOP_AND_REPORT")
                    self.assertEqual(raised.exception.details["orderNo"], ORDER_NO)
                    self.assertTrue(Path(display["widgetPath"]).exists())
                    complete.side_effect = None
                    complete.return_value = {"target": str(target), "orderNo": ORDER_NO}
                    done = make_copy.install(args.work_url, target_path=args.target, replica_code=args.replica_code, recovery_only=True, expected_order_no=args.expected_order_no)
                    self.assertEqual(done["nextAction"], "DEPLOY")
                    self.assertEqual(complete.call_args.args[1]["orderNo"], ORDER_NO)

    def test_support_result_rejects_unconfirmed_or_wrong_order_payment(self):
        authority = make_copy.Authority("", "", "https://viceme.cn/api/v1")
        state = {"sessionId": "session", "sessionToken": "token", "orderNo": "original"}
        for value in [{"orderNo": "another", "payment": {"status": "PAID"}},
                      {"orderNo": "original", "payment": {"status": "UNKNOWN"}},
                      {"orderNo": "original", "payment": []}]:
            with self.subTest(value=value), self.assertRaises(make_copy.WorkflowError) as raised:
                make_copy.wait_for_payment(authority, state, lambda *_a, **_k: response(200, value), lambda _: None)
            self.assertEqual(raised.exception.code, "MAKE_COPY_RESPONSE_INVALID")

    def test_support_render_failure_retains_payment_and_safe_recovery(self):
        authority = make_copy.authority_for_work_url("https://viceme.cn/alice/site.md")
        state = {"orderNo": ORDER_NO, "priceCents": 100, "instruction": f"VICEME-REPLICA:{SHORT_CODE}", "target": "/copy"}
        with mock.patch.object(make_copy, "payment_resource", side_effect=OSError("private error")), self.assertRaises(make_copy.WorkflowError) as raised:
            make_copy.support_result(authority, state, replica(), {"status": "PAID"})
        self.assertEqual(raised.exception.details["payment"]["status"], "PAID")
        self.assertEqual(raised.exception.details["nextAction"], "STOP_AND_REPORT")
        self.assertNotIn("private error", raised.exception.message)
        self.assertTrue(raised.exception.details["recovery"]["requiresUserRequest"])

    @unittest.skipIf(os.name == "nt", "Unix process liveness fixture")
    def test_recovers_lock_left_by_terminated_process(self):
        with tempfile.TemporaryDirectory() as temporary:
            lock_directory = Path(temporary) / "target.lock"
            lock_directory.mkdir()
            owner = lock_directory / "owner.json"
            owner.write_text(json.dumps({"pid": 2_147_483_647}) + "\n")
            owner.chmod(0o600)
            store = {"lockDirectory": lock_directory}
            self.assertEqual(
                make_copy.with_lock(store, lambda: "recovered"), "recovered"
            )
            self.assertFalse(lock_directory.exists())

    def test_persists_order_recovery_before_payment(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            store = {
                "filename": root / "target.json",
                "paidReceiptFilename": root / "recovery.json",
            }
            state = {
                "instruction": f"VICEME-REPLICA:{SHORT_CODE}",
                "sessionClientRequestId": "44444444-4444-4444-8444-444444444444",
                "sessionReplaySecret": SECRET,
                "quoteClientRequestId": "55555555-5555-4555-8555-555555555555",
                "orderClientRequestId": "66666666-6666-4666-8666-666666666666",
                "downloadRecoverySecret": SECRET,
                "priceCents": 100,
                "replicaId": REPLICA_ID,
            }
            replies = iter(
                [
                    {
                        "sessionId": VERSION_ID,
                        "token": "session-token",
                        "expiresAt": "2026-09-04T01:00:00.000Z",
                    },
                    {
                        "orderNo": ORDER_NO,
                        "status": "PENDING",
                        "checkoutUrl": "https://viceme.cn/replica-checkout/session",
                        "expiresAt": "2026-09-04T00:30:00.000Z",
                    },
                    {"replica": replica(), "order": order_view()},
                ]
            )

            def request(*_args, **_kwargs):
                return response(201, next(replies))

            enriched = make_copy.ensure_checkout(
                make_copy.Authority("", "", "https://viceme.cn/api/v1"),
                state,
                store,
                request,
            )
            self.assertEqual(enriched["amountCents"], 100)
            self.assertEqual(enriched["currency"], "CNY")
            recovery = json.loads(store["paidReceiptFilename"].read_text())
            self.assertEqual(recovery["orderNo"], ORDER_NO)
            self.assertEqual(recovery["replicaId"], REPLICA_ID)
            self.assertEqual(recovery["recoverySecret"], SECRET)
            if os.name != "nt":
                self.assertEqual(stat.S_IMODE(store["filename"].stat().st_mode), 0o600)
                self.assertEqual(
                    stat.S_IMODE(store["paidReceiptFilename"].stat().st_mode), 0o600
                )

    def test_windows_private_write_protects_temporary_and_target(self):
        with tempfile.TemporaryDirectory() as temporary:
            target = Path(temporary) / "state.json"
            with mock.patch.object(make_copy.os, "name", "nt"), mock.patch.object(
                make_copy, "protect_windows"
            ) as protect:
                make_copy.atomic_private_write(target, {"schemaVersion": 1})
            self.assertEqual(protect.call_count, 2)
            self.assertNotEqual(protect.call_args_list[0].args[0], target)
            self.assertEqual(protect.call_args_list[1].args, (target, False))


if __name__ == "__main__":
    unittest.main()

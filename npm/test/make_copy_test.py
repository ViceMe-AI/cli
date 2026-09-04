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
    def test_start_is_the_single_public_preview_entrypoint(self):
        work_url = "https://viceme.cn/alice/site.md"
        args = make_copy.parse_args(["start", "--work-url", work_url])
        self.assertEqual(args.command, "start")
        self.assertEqual(args.work_url, work_url)

        preview = {"nextAction": "OPEN_WORK_PREVIEW", "workUrl": work_url}
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

    def test_extracts_instruction_only_from_controlled_block(self):
        authority = make_copy.authority_for_work_url(
            "https://viceme.cn/alice/site.md"
        )

        def request(*_args, **_kwargs):
            return response(
                200,
                (
                    "## 平台控制的完整源码做同款入口\n\n"
                    f"Instruction: VICEME-REPLICA:{SHORT_CODE}"
                ).encode(),
            )

        self.assertEqual(
            make_copy.fetch_work_instruction(authority, request),
            f"VICEME-REPLICA:{SHORT_CODE}",
        )

    def test_reports_paid_recovery_without_exposing_secret(self):
        authority = make_copy.authority_for_work_url(
            "https://viceme.cn/alice/site.md"
        )
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            receipt = make_copy.paid_receipt_path(authority, SHORT_CODE, root)
            receipt.write_text(
                json.dumps(
                    {
                        "schemaVersion": 1,
                        "replicaId": REPLICA_ID,
                        "orderNo": ORDER_NO,
                        "recoverySecret": SECRET,
                    }
                )
                + "\n"
            )
            receipt.chmod(0o600)

            def request(method, url, **_kwargs):
                if url.endswith(".md"):
                    return response(
                        200,
                        (
                            "## 平台控制的完整源码做同款入口\n\n"
                            f"Instruction: VICEME-REPLICA:{SHORT_CODE}"
                        ).encode(),
                    )
                return response(200, replica())

            inspected = make_copy.inspect(
                authority.work_url, request_fn=request, recovery_root=root
            )
            self.assertTrue(inspected["standaloneRecoveryAvailable"])
            self.assertNotIn("recoverySecret", json.dumps(inspected))

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
            return response(200, {"payment": {"status": "PENDING"}})

        with self.assertRaises(make_copy.WorkflowError) as raised:
            make_copy.wait_for_payment(
                make_copy.Authority("", "", "https://viceme.cn/api/v1"),
                {"sessionId": "session", "sessionToken": "token", "orderNo": "order"},
                request,
                sleeps.append,
            )
        self.assertEqual(raised.exception.code, "REPLICA_PAYMENT_TIMEOUT")
        self.assertEqual(sleeps, [60, 60, 60])
        self.assertEqual(len(requests), 3)

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
                ]
            )

            def request(*_args, **_kwargs):
                return response(201, next(replies))

            make_copy.ensure_checkout(
                make_copy.Authority("", "", "https://viceme.cn/api/v1"),
                state,
                store,
                request,
            )
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

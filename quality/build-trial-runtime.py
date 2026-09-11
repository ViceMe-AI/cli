#!/usr/bin/env python3
"""Build the deterministic, self-contained no-CLI runtime and its bootstrap.

Only the bootstrap contains the archive digest, avoiding a self-hash cycle.
The installed runtime never downloads its own executable or presentation assets.
"""
import argparse
import hashlib
import io
import json
import re
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[1]
SCRIPTS = ROOT / "skills/use-a-skill/scripts"


def canonical_text_bytes(path):
    return path.read_text(encoding="utf-8").encode("utf-8")


def artifacts():
    sources = {
        "scripts/trial.py": SCRIPTS / "trial_runtime.py",
        "scripts/qrcodegen.py": ROOT / "widgets/qrcodegen.py",
        "widgets/onboarding.html": ROOT / "widgets/onboarding.html",
        "widgets/payment.html": ROOT / "widgets/payment.html",
        "guides/widgets.md": ROOT / "widgets/README.md",
        "guides/trial-usage.md": ROOT / "skills/use-a-skill/references/trial-usage.md",
    }
    buffer = io.BytesIO()
    # Keep the published runtime byte-for-byte reproducible across Python and
    # zlib versions; the small archive does not need compression.
    with zipfile.ZipFile(buffer, "w", compression=zipfile.ZIP_STORED) as archive:
        for name, source in sorted(sources.items()):
            info = zipfile.ZipInfo(name, (1980, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_STORED
            info.external_attr = 0o100644 << 16
            archive.writestr(info, canonical_text_bytes(source))
    content = buffer.getvalue()
    bootstrap = (ROOT / "release/trial-bootstrap.py.tmpl").read_text(encoding="utf-8").replace(
        "__RUNTIME_SHA256__", hashlib.sha256(content).hexdigest())
    replica_path = ROOT / "skills/let-me-make-a-copy/scripts/make_copy.py"
    resources = {name: hashlib.sha256(canonical_text_bytes(ROOT / "widgets" / name)).hexdigest()
                 for name in ("payment.html", "qrcodegen.py")}
    replica = re.sub(r"^PAYMENT_RESOURCE_SHA256 = .*  # generated-payment-resources$",
                     "PAYMENT_RESOURCE_SHA256 = " + json.dumps(resources, sort_keys=True) + "  # generated-payment-resources",
                     replica_path.read_text(encoding="utf-8"), flags=re.MULTILINE)
    return {SCRIPTS / "trial-runtime.zip": content, SCRIPTS / "trial.py": bootstrap.encode(),
            replica_path: replica.encode()}


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    for path, content in artifacts().items():
        if args.check:
            if not path.exists() or path.read_bytes() != content:
                raise SystemExit("runtime bundle is stale; run make trial-runtime")
        else:
            path.write_bytes(content)

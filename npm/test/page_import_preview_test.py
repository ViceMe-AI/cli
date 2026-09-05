import importlib.util
import json
import os
import tempfile
import threading
import unittest
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen


SCRIPT = (
    Path(__file__).resolve().parents[2]
    / "skills"
    / "customize-your-page"
    / "scripts"
    / "preview_import.py"
)
SPEC = importlib.util.spec_from_file_location("preview_import", SCRIPT)
preview_import = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(preview_import)


class PageImportPreviewTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        (self.root / "assets").mkdir()
        (self.root / "index.html").write_text(
            '<script type="module" src="./assets/app.js"></script>', encoding="utf-8"
        )
        (self.root / "assets" / "app.js").write_text(
            'document.body.dataset.loaded = "yes";', encoding="utf-8"
        )
        (self.root / "viceme-page.json").write_text(
            json.dumps({"spec": {"entry": "index.html"}}), encoding="utf-8"
        )
        self.server = preview_import.create_server(self.root)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.base = f"http://127.0.0.1:{self.server.server_port}"

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)
        self.tempdir.cleanup()

    def request(self, path, method="GET"):
        with urlopen(Request(self.base + path, method=method), timeout=2) as response:
            return response.status, response.headers, response.read()

    def test_serves_shell_and_exact_nested_assets(self):
        status, headers, shell = self.request("/")
        self.assertEqual(status, 200)
        self.assertIn(b'sandbox="allow-scripts ', shell)
        self.assertIn(
            b'/api/v1/public/page-assets/releases/local-import/index.html', shell
        )
        self.assertEqual(headers["Cache-Control"], "no-store")

        status, headers, body = self.request(
            "/api/v1/public/page-assets/releases/local-import/assets/app.js"
        )
        self.assertEqual(status, 200)
        self.assertEqual(headers.get_content_type(), "application/javascript")
        self.assertEqual(headers["Access-Control-Allow-Origin"], "*")
        self.assertIn(b"dataset.loaded", body)

    def test_does_not_hide_missing_routes_with_history_fallback(self):
        with self.assertRaises(HTTPError) as raised:
            self.request("/about")
        self.assertEqual(raised.exception.code, 404)
        raised.exception.close()

        with self.assertRaises(HTTPError) as raised:
            self.request(
                "/api/v1/public/page-assets/releases/local-import/missing.html"
            )
        self.assertEqual(raised.exception.code, 404)
        raised.exception.close()

    def test_head_returns_headers_without_body(self):
        status, headers, body = self.request(
            "/api/v1/public/page-assets/releases/local-import/index.html", "HEAD"
        )
        self.assertEqual(status, 200)
        self.assertEqual(headers.get_content_type(), "text/html")
        self.assertEqual(body, b"")

    def test_rejects_traversal_and_outside_symlink(self):
        with self.assertRaises(ValueError):
            preview_import.package_path(self.root, "../outside.html")

        outside = Path(self.tempdir.name).parent / "outside-page.html"
        outside.write_text("outside", encoding="utf-8")
        link = self.root / "outside.html"
        try:
            os.symlink(outside, link)
            with self.assertRaises(ValueError):
                preview_import.package_path(self.root, "outside.html")
        finally:
            link.unlink(missing_ok=True)
            outside.unlink(missing_ok=True)


if __name__ == "__main__":
    unittest.main()

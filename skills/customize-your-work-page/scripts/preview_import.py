"""Serve a built page inside the same nested-path sandbox used by ViceMe."""

import argparse
import html
import json
import mimetypes
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path, PurePosixPath
from urllib.parse import quote, unquote, urlsplit


PREFIX = "/api/v1/public/page-assets/releases/local-import/"
SANDBOX = "allow-scripts allow-forms allow-modals allow-popups allow-popups-to-escape-sandbox allow-downloads"


def package_path(root, name):
    """Only exact package paths; no normalization, traversal or outside symlinks."""
    if not name or "\\" in name or "\0" in name:
        raise ValueError("Invalid package path")
    parts = name.split("/")
    if any(part in ("", ".", "..") for part in parts):
        raise ValueError("Invalid package path")
    path = root.joinpath(*parts).resolve()
    if not path.is_relative_to(root) or not path.is_file():
        raise ValueError("Package file not found")
    return path


def create_server(directory, port=0):
    root = Path(directory).resolve(strict=True)
    manifest = json.loads(package_path(root, "viceme-page.json").read_text("utf-8"))
    if not isinstance(manifest, dict) or not isinstance(manifest.get("spec"), dict):
        raise ValueError("Manifest must contain a spec object")
    entry = manifest["spec"].get("entry")
    if not isinstance(entry, str) or PurePosixPath(entry).suffix.lower() != ".html":
        raise ValueError("Manifest must identify an HTML entry")
    package_path(root, entry)
    entry_url = PREFIX + quote(entry, safe="/")
    shell = (
        '<!doctype html><html><head><meta charset="utf-8">'
        '<meta name="viewport" content="width=device-width, initial-scale=1">'
        '<link rel="icon" href="data:,">'
        '<title>ViceMe local import preview</title></head>'
        '<body style="margin:0"><iframe title="Imported page" '
        f'sandbox="{SANDBOX}" src="{html.escape(entry_url, quote=True)}" '
        'style="width:100%;height:100dvh;border:0;display:block"></iframe></body></html>'
    ).encode()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_GET(self):
            self.respond(False)

        def do_HEAD(self):
            self.respond(True)

        def respond(self, head_only):
            request_path = urlsplit(self.path).path
            if request_path == "/":
                body, content_type = shell, "text/html; charset=utf-8"
            elif request_path.startswith(PREFIX):
                try:
                    asset = package_path(root, unquote(request_path[len(PREFIX):]))
                    body = asset.read_bytes()
                except (ValueError, OSError):
                    self.send_error(404)
                    return
                content_type = mimetypes.guess_type(asset.name)[0] or "application/octet-stream"
                if asset.suffix.lower() in (".js", ".mjs"):
                    content_type = "application/javascript"
            else:
                self.send_error(404)
                return
            self.send_response(200)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Cache-Control", "no-store")
            # Sandboxed ES modules have an opaque origin, as on the real asset host.
            self.send_header("Access-Control-Allow-Origin", "*")
            self.send_header("X-Content-Type-Options", "nosniff")
            self.end_headers()
            if not head_only:
                self.wfile.write(body)

    return ThreadingHTTPServer(("127.0.0.1", port), Handler)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", required=True, help="Built package directory containing viceme-page.json")
    parser.add_argument("--port", type=int, default=0)
    args = parser.parse_args()
    try:
        server = create_server(args.root, args.port)
    except (ValueError, OSError, json.JSONDecodeError) as error:
        parser.error(str(error))
    print(f"http://127.0.0.1:{server.server_port}/", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()

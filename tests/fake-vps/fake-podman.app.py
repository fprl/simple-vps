"""Fake app container listener used by tests/fake-vps/fake-podman.

Binds to an OS-assigned localhost port and writes that port number to
the file path given as argv[1] so fake-podman state can record where
this container is reachable from the host. Answers 200 "ok" to every
GET/HEAD/POST so the helper's deploy-time healthcheck and any
Caddy-proxied request both succeed.
"""

import sys
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        return

    def _ok(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", "2")
        self.end_headers()
        self.wfile.write(b"ok")

    def do_GET(self):
        self._ok()

    def do_HEAD(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.end_headers()

    def do_POST(self):
        self._ok()


def main():
    if len(sys.argv) != 2:
        print("usage: fake-podman.app.py <port_file>", file=sys.stderr)
        sys.exit(2)
    port_file = sys.argv[1]
    server = HTTPServer(("127.0.0.1", 0), Handler)
    port = server.server_address[1]
    with open(port_file, "w") as f:
        f.write(str(port) + "\n")
    server.serve_forever()


if __name__ == "__main__":
    main()

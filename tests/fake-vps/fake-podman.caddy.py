"""Fake Caddy reverse proxy used by tests/fake-vps/fake-podman.

Stands in for `caddy:2-alpine` inside the fake-vps container. Listens
on argv[1] (the --publish host port) and on every request:

  1. Reads /etc/caddy/conf.d/*.caddy fragments.
  2. Looks for a site block whose label matches the request's Host
     header.
  3. Pulls the `reverse_proxy http://<container_name>:<container_port>`
     directive out of that block.
  4. Resolves <container_name> to a localhost port via
     <state_dir>/containers/<container_name>.port (written by
     fake-podman when it started the app container).
  5. Proxies the request to that local port.

`redir` directives are honored as 301s. Anything unmatched returns
404. The proxy re-reads conf.d/ on every request, so a deploy that
updates fragments takes effect without any explicit reload signal —
which is why fake-podman can leave `podman exec caddy caddy reload`
as a no-op.
"""

import http.client
import os
import re
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


SITE_RE = re.compile(r'^\s*"(?P<host>[^"]+)"\s*\{\s*$')
PROXY_RE = re.compile(r'^\s*reverse_proxy\s+http://(?P<upstream>[A-Za-z0-9_.-]+):(?P<port>\d+)\s*$')
REDIR_RE = re.compile(r'^\s*redir\s+"(?P<to>[^"]+)"\s+permanent\s*$')
ROOT_RE = re.compile(r'^\s*root\s+\*\s+"(?P<root>[^"]+)"\s*$')

CONF_DIR = "/etc/caddy/conf.d"


def load_routes():
    """Return {host: {"proxy": ...} | {"redir": ...} | {"static": ...}}."""
    routes = {}
    if not os.path.isdir(CONF_DIR):
        return routes
    for name in sorted(os.listdir(CONF_DIR)):
        if not name.endswith(".caddy"):
            continue
        path = os.path.join(CONF_DIR, name)
        try:
            with open(path) as f:
                lines = f.readlines()
        except OSError:
            continue

        current_host = None
        for raw in lines:
            line = raw.rstrip("\n")
            stripped = line.strip()
            if stripped.startswith("#") or stripped == "":
                continue
            if current_host is None:
                m = SITE_RE.match(line)
                if m:
                    current_host = m.group("host")
                continue
            if stripped == "}":
                current_host = None
                continue
            m = PROXY_RE.match(line)
            if m:
                routes[current_host] = {
                    "proxy": (m.group("upstream"), int(m.group("port"))),
                }
                continue
            m = REDIR_RE.match(line)
            if m:
                routes[current_host] = {"redir": m.group("to")}
                continue
            m = ROOT_RE.match(line)
            if m:
                routes[current_host] = {"static": m.group("root")}
                continue
    return routes


def container_port(state_dir, name):
    path = os.path.join(state_dir, "containers", name + ".port")
    try:
        with open(path) as f:
            return int(f.read().strip())
    except (OSError, ValueError):
        return None


class Handler(BaseHTTPRequestHandler):
    state_dir = ""  # set in main()

    def log_message(self, *args):
        return

    def host_header(self):
        raw = self.headers.get("Host", "")
        return raw.split(":", 1)[0].lower().rstrip(".")

    def not_found(self, msg=b"not found"):
        self.send_response(404)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(msg)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(msg)

    def proxy(self, upstream_name, _upstream_port):
        # The Caddy fragment names an in-container port. In the fake we
        # don't actually run on that port; the app container's
        # fake-podman.app.py listener bound an OS-assigned local port
        # and recorded it. Look that up and proxy.
        local_port = container_port(self.state_dir, upstream_name)
        if local_port is None:
            self.not_found(b"unknown upstream " + upstream_name.encode())
            return

        body_length = int(self.headers.get("Content-Length", "0") or 0)
        body = self.rfile.read(body_length) if body_length > 0 else b""

        forward = http.client.HTTPConnection("127.0.0.1", local_port, timeout=5)
        forward_headers = {}
        for name, value in self.headers.items():
            if name.lower() in ("host", "content-length", "connection"):
                continue
            forward_headers[name] = value
        try:
            forward.request(self.command, self.path, body=body, headers=forward_headers)
            resp = forward.getresponse()
            resp_body = resp.read()
        except Exception as exc:
            self.send_response(502)
            self.send_header("Content-Type", "text/plain")
            msg = f"upstream {upstream_name} unreachable: {exc}".encode()
            self.send_header("Content-Length", str(len(msg)))
            self.end_headers()
            if self.command != "HEAD":
                self.wfile.write(msg)
            return
        finally:
            forward.close()

        self.send_response(resp.status)
        skip = {"connection", "transfer-encoding"}
        for name, value in resp.getheaders():
            if name.lower() in skip:
                continue
            self.send_header(name, value)
        self.send_header("Content-Length", str(len(resp_body)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(resp_body)

    def redir(self, to):
        self.send_response(301)
        self.send_header("Location", to)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def static(self, root):
        rel = self.path.split("?", 1)[0].split("#", 1)[0]
        if rel in ("", "/"):
            rel = "/index.html"
        rel = os.path.normpath(rel.lstrip("/"))
        if rel.startswith("..") or os.path.isabs(rel):
            self.not_found()
            return
        target = os.path.abspath(os.path.join(root, rel))
        root_abs = os.path.abspath(root)
        if target != root_abs and not target.startswith(root_abs + os.sep):
            self.not_found()
            return
        if os.path.isdir(target):
            target = os.path.join(target, "index.html")
        try:
            with open(target, "rb") as f:
                body = f.read()
        except OSError:
            self.not_found()
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def serve(self):
        routes = load_routes()
        host = self.host_header()
        route = routes.get(host)
        if route is None:
            self.not_found()
            return
        if "proxy" in route:
            self.proxy(*route["proxy"])
            return
        if "redir" in route:
            self.redir(route["redir"])
            return
        if "static" in route:
            self.static(route["static"])
            return
        self.not_found()

    def do_GET(self):
        self.serve()

    def do_HEAD(self):
        self.serve()

    def do_POST(self):
        self.serve()


class Server(ThreadingHTTPServer):
    allow_reuse_address = True


def main():
    if len(sys.argv) != 3:
        print("usage: fake-podman.caddy.py <port> <state_dir>", file=sys.stderr)
        sys.exit(2)
    port = int(sys.argv[1])
    Handler.state_dir = sys.argv[2]
    Server(("0.0.0.0", port), Handler).serve_forever()


if __name__ == "__main__":
    main()

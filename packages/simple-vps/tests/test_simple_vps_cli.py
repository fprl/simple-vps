import argparse
import contextlib
import importlib.machinery
import importlib.util
import io
import json
import subprocess
import tempfile
import unittest
from pathlib import Path


CLI_PATH = Path(__file__).resolve().parents[1] / "roles/infra/files/simple-vps"


def load_cli(tmp_path):
    loader = importlib.machinery.SourceFileLoader("simple_vps_cli", str(CLI_PATH))
    spec = importlib.util.spec_from_loader(loader.name, loader)
    module = importlib.util.module_from_spec(spec)
    loader.exec_module(module)

    module.STATE_PATH = tmp_path / "state.json"
    module.CADDYFILE_PATH = tmp_path / "Caddyfile"
    module.CADDY_ROOT = tmp_path
    module.MANAGED_CADDY_DIR = tmp_path / "simple-vps"
    module.USER_CADDY_DIR = tmp_path / "conf.d"
    module.ROUTES_CADDYFILE_PATH = module.MANAGED_CADDY_DIR / "routes.caddy"
    module.BACKUP_DIR = tmp_path / "backups"
    module.CADDY_BIN = "true"
    module.SYSTEMCTL_BIN = "true"
    module.require_root = lambda: None
    return module


def call_quiet(func, *args, **kwargs):
    with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
        return func(*args, **kwargs)


def capture_quiet(func, *args, **kwargs):
    stdout = io.StringIO()
    with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(io.StringIO()):
        func(*args, **kwargs)
    return stdout.getvalue()


class SimpleVpsCliTest(unittest.TestCase):
    def test_publish_writes_state_and_caddyfile(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))

            call_quiet(cli.cmd_publish, argparse.Namespace(host="Example.com", port="3000", force=False))
            call_quiet(cli.cmd_publish, argparse.Namespace(host="api.example.com", port="3001", force=False))

            state = json.loads(cli.STATE_PATH.read_text(encoding="utf-8"))
            self.assertEqual(
                state["routes"],
                [
                    {"host": "api.example.com", "port": 3001, "type": "proxy"},
                    {"host": "example.com", "port": 3000, "type": "proxy"},
                ],
            )

            caddyfile = cli.CADDYFILE_PATH.read_text(encoding="utf-8")
            self.assertIn("import simple-vps/*.caddy", caddyfile)
            self.assertIn("import conf.d/*.caddy", caddyfile)

            routes_caddyfile = cli.ROUTES_CADDYFILE_PATH.read_text(encoding="utf-8")
            self.assertIn("http://:8080 {", routes_caddyfile)
            self.assertIn("bind 127.0.0.1", routes_caddyfile)
            self.assertIn("@route_0 host api.example.com", routes_caddyfile)
            self.assertIn("reverse_proxy 127.0.0.1:3001", routes_caddyfile)
            self.assertIn("@route_1 host example.com", routes_caddyfile)
            self.assertIn("reverse_proxy 127.0.0.1:3000", routes_caddyfile)

    def test_publish_conflict_requires_force(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))

            call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3000", force=False))
            with self.assertRaises(SystemExit):
                call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3001", force=False))

            state = json.loads(cli.STATE_PATH.read_text(encoding="utf-8"))
            self.assertEqual(state["routes"], [{"host": "example.com", "port": 3000, "type": "proxy"}])

            call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3001", force=True))
            state = json.loads(cli.STATE_PATH.read_text(encoding="utf-8"))
            self.assertEqual(state["routes"], [{"host": "example.com", "port": 3001, "type": "proxy"}])

    def test_unpublish_removes_route(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))

            call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3000", force=False))
            call_quiet(cli.cmd_unpublish, argparse.Namespace(host="example.com"))

            state = json.loads(cli.STATE_PATH.read_text(encoding="utf-8"))
            self.assertEqual(state["routes"], [])

    def test_legacy_port_routes_load_as_proxy_routes(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))
            cli.STATE_PATH.write_text(
                json.dumps({"version": 1, "routes": [{"host": "example.com", "port": 3000}]}),
                encoding="utf-8",
            )

            state = cli.load_state()

            self.assertEqual(state["version"], 2)
            self.assertEqual(state["routes"], [{"host": "example.com", "port": 3000, "type": "proxy"}])

    def test_route_static_and_redirect_write_state_and_caddyfile(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))

            call_quiet(
                cli.cmd_route_static,
                argparse.Namespace(
                    host="Data.example.com",
                    root="/var/apps/data-feed/current/public",
                    app="data-feed",
                    header=["Cache-Control: public, max-age=60"],
                    force=False,
                ),
            )
            call_quiet(
                cli.cmd_route_redirect,
                argparse.Namespace(
                    host="old.example.com",
                    to="https://new.example.com{uri}",
                    app=None,
                    force=False,
                ),
            )

            state = json.loads(cli.STATE_PATH.read_text(encoding="utf-8"))
            self.assertEqual(
                state["routes"],
                [
                    {
                        "app": "data-feed",
                        "headers": {"Cache-Control": "public, max-age=60"},
                        "host": "data.example.com",
                        "root": "/var/apps/data-feed/current/public",
                        "type": "static",
                    },
                    {
                        "host": "old.example.com",
                        "to": "https://new.example.com{uri}",
                        "type": "redirect",
                    },
                ],
            )

            routes_caddyfile = cli.ROUTES_CADDYFILE_PATH.read_text(encoding="utf-8")
            self.assertIn("root * \"/var/apps/data-feed/current/public\"", routes_caddyfile)
            self.assertIn("header Cache-Control \"public, max-age=60\"", routes_caddyfile)
            self.assertIn("file_server", routes_caddyfile)
            self.assertIn("redir \"https://new.example.com{uri}\" permanent", routes_caddyfile)

    def test_routes_json_lists_typed_routes(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))

            call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3000", force=False))
            output = capture_quiet(cli.cmd_routes, argparse.Namespace(json=True))

            payload = json.loads(output)
            self.assertEqual(payload["routes"], [{"host": "example.com", "port": 3000, "type": "proxy"}])

    def test_invalid_host_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))

            with self.assertRaises(SystemExit):
                call_quiet(cli.cmd_publish, argparse.Namespace(host="-bad.example.com", port="3000", force=False))

    def test_failed_reload_restores_previous_caddyfile(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))
            old_caddyfile = cli.managed_content("old caddyfile\n")
            cli.STATE_PATH.write_text(
                json.dumps({"version": 2, "routes": [{"host": "old.example.com", "port": 3000, "type": "proxy"}]}),
                encoding="utf-8",
            )
            cli.SYSTEMCTL_BIN = "false"
            cli.CADDYFILE_PATH.write_text(old_caddyfile, encoding="utf-8")

            with self.assertRaises(SystemExit):
                call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3000", force=False))

            self.assertEqual(cli.CADDYFILE_PATH.read_text(encoding="utf-8"), old_caddyfile)
            state = json.loads(cli.STATE_PATH.read_text(encoding="utf-8"))
            self.assertEqual(state["routes"], [{"host": "old.example.com", "port": 3000, "type": "proxy"}])
            self.assertEqual(len(list(cli.BACKUP_DIR.iterdir())), 1)

    def test_manual_edit_in_generated_caddyfile_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))

            call_quiet(cli.cmd_generate_caddy, argparse.Namespace(force=False))
            content = cli.ROUTES_CADDYFILE_PATH.read_text(encoding="utf-8")
            cli.ROUTES_CADDYFILE_PATH.write_text(content + "# manual edit\n", encoding="utf-8")

            with self.assertRaises(SystemExit):
                call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3000", force=False))

    def test_validate_uses_caddyfile_adapter(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))
            commands = []

            def fake_run(command, text=False, capture_output=False, check=False):
                commands.append(command)
                return subprocess.CompletedProcess(command, 0, stdout="", stderr="")

            cli.subprocess.run = fake_run
            call_quiet(cli.cmd_generate_caddy, argparse.Namespace())

            validate_commands = [command for command in commands if command[:2] == ["true", "validate"]]
            self.assertTrue(validate_commands)
            for command in validate_commands:
                self.assertEqual(command[-2:], ["--adapter", "caddyfile"])

    def test_generate_caddy_is_idempotent(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))

            first = capture_quiet(cli.cmd_generate_caddy, argparse.Namespace())
            second = capture_quiet(cli.cmd_generate_caddy, argparse.Namespace())

            self.assertIn("Generated", first)
            self.assertIn("already up to date", second)

    def test_status_includes_services_and_tools(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))
            cli.service_status = lambda service: f"{service}-active"
            cli.tool_status = lambda tool: f"{tool}-installed"

            output = capture_quiet(cli.cmd_status, argparse.Namespace())

            self.assertIn("routes: 0", output)
            self.assertIn("services:", output)
            self.assertNotIn("  docker:", output)
            self.assertIn("tools:", output)
            self.assertIn("  litestream: litestream-installed", output)
            self.assertIn("  bun: bun-installed", output)


if __name__ == "__main__":
    unittest.main()

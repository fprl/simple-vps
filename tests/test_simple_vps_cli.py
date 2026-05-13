import argparse
import contextlib
import importlib.machinery
import importlib.util
import io
import json
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
    module.BACKUP_DIR = tmp_path / "backups"
    module.CADDY_BIN = "true"
    module.SYSTEMCTL_BIN = "true"
    module.require_root = lambda: None
    return module


def call_quiet(func, *args, **kwargs):
    with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
        return func(*args, **kwargs)


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
                    {"host": "api.example.com", "port": 3001},
                    {"host": "example.com", "port": 3000},
                ],
            )

            caddyfile = cli.CADDYFILE_PATH.read_text(encoding="utf-8")
            self.assertIn("127.0.0.1:8080 {", caddyfile)
            self.assertIn("@route_0 host api.example.com", caddyfile)
            self.assertIn("reverse_proxy 127.0.0.1:3001", caddyfile)
            self.assertIn("@route_1 host example.com", caddyfile)
            self.assertIn("reverse_proxy 127.0.0.1:3000", caddyfile)

    def test_publish_conflict_requires_force(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))

            call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3000", force=False))
            with self.assertRaises(SystemExit):
                call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3001", force=False))

            state = json.loads(cli.STATE_PATH.read_text(encoding="utf-8"))
            self.assertEqual(state["routes"], [{"host": "example.com", "port": 3000}])

            call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3001", force=True))
            state = json.loads(cli.STATE_PATH.read_text(encoding="utf-8"))
            self.assertEqual(state["routes"], [{"host": "example.com", "port": 3001}])

    def test_unpublish_removes_route(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))

            call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3000", force=False))
            call_quiet(cli.cmd_unpublish, argparse.Namespace(host="example.com"))

            state = json.loads(cli.STATE_PATH.read_text(encoding="utf-8"))
            self.assertEqual(state["routes"], [])

    def test_invalid_host_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))

            with self.assertRaises(SystemExit):
                call_quiet(cli.cmd_publish, argparse.Namespace(host="-bad.example.com", port="3000", force=False))

    def test_failed_reload_restores_previous_caddyfile(self):
        with tempfile.TemporaryDirectory() as tmp:
            cli = load_cli(Path(tmp))
            cli.STATE_PATH.write_text(
                json.dumps({"version": 1, "routes": [{"host": "old.example.com", "port": 3000}]}),
                encoding="utf-8",
            )
            cli.SYSTEMCTL_BIN = "false"
            cli.CADDYFILE_PATH.write_text("old caddyfile\n", encoding="utf-8")

            with self.assertRaises(SystemExit):
                call_quiet(cli.cmd_publish, argparse.Namespace(host="example.com", port="3000", force=False))

            self.assertEqual(cli.CADDYFILE_PATH.read_text(encoding="utf-8"), "old caddyfile\n")
            state = json.loads(cli.STATE_PATH.read_text(encoding="utf-8"))
            self.assertEqual(state["routes"], [{"host": "old.example.com", "port": 3000}])
            self.assertEqual(len(list(cli.BACKUP_DIR.iterdir())), 1)


if __name__ == "__main__":
    unittest.main()

"""Tooling branch tests; deliberately no daemon, PostgreSQL or real uploads."""

import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tarfile
import tempfile
import time
import unittest


ROOT = Path(__file__).resolve().parents[2]
FORMAT = ROOT / "deploy/backup-format.py"
IMAGE = "sha256:" + "a" * 64

# Subprocess boundary double, with state changes and injectable failures.
FAKE_DOCKER = r'''#!/usr/bin/env python3
import io, json, os, pathlib, sys, tarfile
a = sys.argv[1:]
root = pathlib.Path(os.environ["FAKE_STATE"])
failure = os.environ.get("FAKE_FAIL", "")
def event(name):
    with (root / "events").open("a") as f: f.write(name + "\n")
def fail(name):
    event(name)
    if failure == name: sys.exit(1)
if a[0] == "info": sys.exit(0)
if a[0] == "compose":
    project = a[a.index("-p") + 1]
    c = a[a.index("-p") + 2:]
    if c[0] == "config":
        if "--format" in c:
            print(json.dumps({"services":{"db":{"environment":{
                "POSTGRES_USER":"sweetnet", "POSTGRES_DB":"sweetnet", "POSTGRES_PASSWORD":"test_only_hex_123"
            }}}}))
        elif "--images" in c: print("sweetnet:test")
    elif c[0] == "ps": print("app-id" if c[-1] == "app" else "db-id")
    elif c[0] == "stop":
        (root / "stopped").touch()
        fail("stop")
    elif c[0] == "start":
        fail("start")
        (root / "stopped").unlink(missing_ok=True)
    elif c[0] == "exec":
        joined = " ".join(c)
        if "pg_dump" in joined:
            fail("dump")
            sys.stdout.buffer.write(b"PGDMP-test-only")
        elif "--list" in c:
            fail("dump-list")
            sys.stdin.buffer.read()
        else:
            fail("restore-db")
            sys.stdin.buffer.read()
    elif c[0] == "run": fail("migrate")
    elif c[0] == "create": fail("create-app")
    elif c[0] == "up": fail("up-" + c[-1])
    else: sys.exit(3)
elif a[0] == "inspect":
    fmt = a[a.index("--format") + 1]
    if "Running" in fmt: print("false" if (root / "stopped").exists() else "true")
    elif fmt == "{{.Image}}": print("sha256:" + "a" * 64)
    else:
        p = os.environ["FAKE_PROJECT"]
        if a[-1] == "app-id": print(p + "_uploads /data/uploads")
        else: print(p + "_postgres_data /var/lib/postgresql/data")
elif a[0] == "create":
    fail("lock")
    print("lock-id")
elif a[0] == "rm": fail("unlock")
elif a[0] == "image": print("sha256:" + "a" * 64)
elif a[0] == "ps":
    fail("inventory-containers")
    if os.environ.get("FAKE_EXISTING") == "container": print("existing-id")
elif a[0] == "volume":
    fail("inventory-volumes")
    if os.environ.get("FAKE_EXISTING") == "volume": print(os.environ["FAKE_PROJECT"] + "_uploads")
elif a[0] == "network":
    fail("inventory-networks")
    if os.environ.get("FAKE_EXISTING") == "network": print("existing-id")
elif a[0] == "run":
    assert a[a.index("--log-driver")+1] == "none"
    assert a[a.index("--network")+1] == "none"
    if "tar -czf" in a[-1]:
        fail("archive")
        if failure == "archive-wait":
            import time
            time.sleep(15)
        stream = io.BytesIO()
        with tarfile.open(fileobj=stream, mode="w:gz") as tar:
            member = tarfile.TarInfo("./test-image.jpg")
            member.size = 4
            tar.addfile(member, io.BytesIO(b"test"))
        sys.stdout.buffer.write(stream.getvalue())
    else:
        fail("restore-files")
        sys.stdin.buffer.read()
else: sys.exit(3)
'''


class MaintenanceTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="sweetnet-tooling-")
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)
        executable = self.directory / "docker"
        executable.write_text(FAKE_DOCKER)
        executable.chmod(0o700)
        self.env_file = self.directory / "test.env"
        self.env_file.write_text("POSTGRES_PASSWORD=test_only_hex_123\n")
        self.bundle = self.directory / "bundle"
        self.project = "source"

    def invoke(self, operation, *, failure="", existing=""):
        env = dict(os.environ, PATH=str(self.directory) + os.pathsep + os.environ["PATH"],
                   FAKE_STATE=str(self.directory), FAKE_PROJECT=self.project,
                   FAKE_FAIL=failure, FAKE_EXISTING=existing)
        command = ["bash", str(ROOT / f"scripts/{operation}.sh"), str(self.env_file), self.project, str(self.bundle)]
        if operation == "restore":
            command.append("--new-environment")
        return subprocess.run(command, env=env, capture_output=True, text=True)

    def events(self):
        file = self.directory / "events"
        return file.read_text().splitlines() if file.exists() else []

    def make_bundle(self, member_name="./photo.jpg", member_type=tarfile.REGTYPE):
        self.bundle.mkdir()
        (self.bundle / "database.dump").write_bytes(b"PGDMP-test-only")
        with tarfile.open(self.bundle / "uploads.tar.gz", "w:gz") as archive:
            member = tarfile.TarInfo(member_name)
            member.type = member_type
            if member_type == tarfile.REGTYPE:
                member.size = 4
                archive.addfile(member, io.BytesIO(b"test"))
            else:
                member.linkname = "/outside"
                archive.addfile(member)
        metadata = {"format": 1, "postgres_major": 17, "source_project": "source", "app_image_id": IMAGE,
                    "sha256": {name: hashlib.sha256((self.bundle / name).read_bytes()).hexdigest()
                               for name in ("database.dump", "uploads.tar.gz")}}
        (self.bundle / "manifest.json").write_text(json.dumps(metadata))
        self.project = "new-target"

    def test_backup_success_order_and_private_permissions(self):
        result = self.invoke("backup")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.events(), ["lock", "stop", "dump", "archive", "dump-list", "start", "unlock"])
        self.assertFalse((self.bundle / "INCOMPLETE").exists())
        self.assertEqual(self.bundle.stat().st_mode & 0o777, 0o700)
        self.assertEqual((self.bundle / "database.dump").stat().st_mode & 0o777, 0o600)
        self.assertNotIn("PGDMP", result.stdout + result.stderr)

    def test_backup_failures_restart_app_and_leave_incomplete_bundle(self):
        for failure in ("stop", "dump", "archive", "dump-list", "start"):
            with self.subTest(failure=failure):
                result = self.invoke("backup", failure=failure)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("start", self.events())
                self.assertEqual(self.events()[-1], "unlock")
                self.assertTrue((self.bundle / "INCOMPLETE").exists())
                # Only test-generated files, never user data or Docker volumes.
                shutil.rmtree(self.bundle)
                (self.directory / "events").unlink()
                (self.directory / "stopped").unlink(missing_ok=True)

    def test_restore_success_orders_database_files_migration_app(self):
        self.make_bundle()
        result = self.invoke("restore")
        self.assertEqual(result.returncode, 0, result.stderr)
        events = self.events()
        self.assertEqual(events[-7:], ["up-db", "restore-db", "create-app", "restore-files", "migrate", "up-app", "unlock"])
        self.assertNotIn("start", events)

    def test_restore_existing_resources_are_rejected(self):
        self.make_bundle()
        for existing in ("container", "volume", "network"):
            with self.subTest(existing=existing):
                result = self.invoke("restore", existing=existing)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("up-db", self.events())

    def test_restore_failure_never_starts_partially_restored_app(self):
        self.make_bundle()
        for failure in ("inventory-volumes", "up-db", "restore-db", "create-app", "restore-files", "migrate"):
            with self.subTest(failure=failure):
                result = self.invoke("restore", failure=failure)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("up-app", self.events())
                self.assertNotIn("start", self.events())
                self.assertEqual(self.events()[-1], "unlock")
                (self.directory / "events").unlink()

    def test_restore_to_source_rejected_before_lock(self):
        self.make_bundle()
        self.project = "source"
        self.assertNotEqual(self.invoke("restore").returncode, 0)
        self.assertEqual(self.events(), [])

    def test_restore_corruption_rejected_before_lock(self):
        self.make_bundle()
        (self.bundle / "database.dump").write_bytes(b"changed")
        self.assertNotEqual(self.invoke("restore").returncode, 0)
        self.assertEqual(self.events(), [])

    def test_restore_incomplete_rejected(self):
        self.make_bundle()
        (self.bundle / "INCOMPLETE").touch()
        self.assertNotEqual(self.invoke("restore").returncode, 0)
        self.assertEqual(self.events(), [])

    def test_restore_wrong_image_rejected(self):
        self.make_bundle()
        path = self.bundle / "manifest.json"
        data = json.loads(path.read_text())
        data["app_image_id"] = "sha256:other"
        path.write_text(json.dumps(data))
        self.assertNotEqual(self.invoke("restore").returncode, 0)
        self.assertEqual(self.events(), [])

    def test_unsafe_archive_rejected_before_lock(self):
        for name, kind in (("../outside", tarfile.REGTYPE), ("/outside", tarfile.REGTYPE),
                           ("./link", tarfile.SYMTYPE), ("./hard", tarfile.LNKTYPE)):
            with self.subTest(name=name):
                self.make_bundle(name, kind)
                self.assertNotEqual(self.invoke("restore").returncode, 0)
                self.assertEqual(self.events(), [])
                shutil.rmtree(self.bundle)

    def test_backup_existing_destination_rejected(self):
        self.bundle.mkdir()
        self.assertNotEqual(self.invoke("backup").returncode, 0)
        self.assertEqual(self.events(), [])

    def test_missing_restore_guard(self):
        result = subprocess.run(["bash", str(ROOT / "scripts/restore.sh"), "env", "project", "backup"],
                                capture_output=True)
        self.assertEqual(result.returncode, 2)

    def test_lock_collision_does_not_stop_app_or_remove_other_lock(self):
        result = self.invoke("backup", failure="lock")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.events(), ["lock"])

    def test_termination_restarts_app(self):
        env = dict(os.environ, PATH=str(self.directory) + os.pathsep + os.environ["PATH"],
                   FAKE_STATE=str(self.directory), FAKE_PROJECT=self.project,
                   FAKE_FAIL="archive-wait")
        proc = subprocess.Popen(["bash", str(ROOT / "scripts/backup.sh"), str(self.env_file), self.project, str(self.bundle)],
                                env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        try:
            deadline = time.monotonic() + 10
            while "archive" not in self.events() and time.monotonic() < deadline:
                time.sleep(0.05)
            self.assertIn("archive", self.events())
            os.killpg(proc.pid, signal.SIGTERM)
            proc.communicate(timeout=10)
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("start", self.events())
            self.assertEqual(self.events()[-1], "unlock")
        finally:
            if proc.poll() is None:
                os.killpg(proc.pid, signal.SIGKILL)
                proc.communicate()


@unittest.skipUnless(shutil.which("docker"), "Docker CLI is not installed")
class ComposeConfigurationTests(unittest.TestCase):
    def config(self, production=False, password=True):
        with tempfile.TemporaryDirectory(prefix="sweetnet-compose-") as temporary:
            env_file = Path(temporary) / "test.env"
            env_file.write_text(
                ("POSTGRES_PASSWORD=test_only_hex_123\n" if password else "")
                + "APP_ENV=development\nAPP_ORIGIN=http://localhost:8080\n"
                + "SWEETNET_DOMAIN=sweetnet.example.com\nACME_EMAIL=operator@example.com\n")
            env = {k: v for k, v in os.environ.items()
                   if not k.startswith(("COMPOSE_", "POSTGRES_", "APP_", "SWEETNET_"))}
            command = ["docker", "compose", "--env-file", str(env_file), "-f", str(ROOT / "compose.yaml")]
            if production:
                command += ["-f", str(ROOT / "deploy/compose.production.yaml")]
            return subprocess.run(command + ["-p", "sweetnet-test-config", "config", "--format", "json"],
                                  env=env, capture_output=True, text=True)

    def test_development_contract(self):
        result = self.config()
        self.assertEqual(result.returncode, 0, result.stderr)
        config = json.loads(result.stdout)
        services = config["services"]
        self.assertNotIn("ports", services["db"])
        self.assertEqual(services["db"]["image"], "postgres:17.11-alpine3.23")
        self.assertEqual(services["migrate"]["depends_on"]["db"]["condition"], "service_healthy")
        self.assertEqual(services["app"]["depends_on"]["migrate"]["condition"], "service_completed_successfully")
        self.assertEqual(services["app"]["user"], "10001:10001")
        self.assertTrue(services["app"]["read_only"])
        self.assertEqual(services["app"]["ports"][0]["host_ip"], "127.0.0.1")
        self.assertEqual(services["app"]["ports"][0]["published"], "8080")
        self.assertEqual(services["app"]["environment"]["UPLOAD_DIR"], "/data/uploads")
        self.assertEqual(services["migrate"]["command"], ["/app/admin", "migrate"])
        self.assertEqual(config["volumes"]["uploads"]["name"], "sweetnet-test-config_uploads")

    def test_production_override_contract(self):
        result = self.config(production=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        services = json.loads(result.stdout)["services"]
        for service in ("app", "migrate"):
            self.assertEqual(services[service]["environment"]["APP_ENV"], "production")
            self.assertEqual(services[service]["environment"]["APP_ORIGIN"], "https://sweetnet.example.com")
        self.assertEqual(services["caddy"]["image"], "caddy:2.11.4-alpine")
        self.assertTrue(any(mount["source"] == str(ROOT / "deploy/Caddyfile")
                            for mount in services["caddy"]["volumes"] if mount["type"] == "bind"))
        self.assertNotIn("ports", services["db"])

    def test_missing_password_refuses_startup(self):
        self.assertNotEqual(self.config(password=False).returncode, 0)


if __name__ == "__main__":
    unittest.main()

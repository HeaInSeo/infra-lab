import hashlib
import importlib.util
import json
import os
import shlex
import subprocess
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "runtime_guard", os.path.join(os.path.dirname(__file__), "claude_runtime_integrity.py"))
runtime_guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runtime_guard)
GOOD = b'#!/usr/bin/env bash\n# approved disposable fixture\nexit 0\n'


class RuntimeIntegrityTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.home = self.temp.name
        self.guard = runtime_guard.Guard(self.home)
        os.makedirs(self.guard.share)
        os.makedirs(os.path.join(self.home, ".local/bin"))
        self.live = os.path.join(self.home, ".local/bin/claude-operator-tick.sh")
        digest = hashlib.sha256(GOOD).hexdigest()
        self.backup = os.path.join(self.guard.share, "operator-" + digest + ".sh")
        self.write(self.backup, GOOD, 0o444)
        self.write(self.live, GOOD, 0o555)
        manifest = {"schema_version": 1, "roles": {"operator": {
            "live": self.live, "backup": self.backup, "sha256": digest,
            "size": len(GOOD), "mode": 0o555}}}
        self.write(os.path.join(self.guard.share, "manifest.json"),
                   json.dumps(manifest).encode(), 0o444)

    @staticmethod
    def write(path, content, mode):
        if os.path.exists(path):
            os.chmod(path, 0o600)
        with open(path, "wb") as stream:
            stream.write(content)
        os.chmod(path, mode)

    def tearDown(self):
        for root, dirs, names in os.walk(self.home):
            for name in names:
                os.chmod(os.path.join(root, name), 0o600)
        self.temp.cleanup()

    def contents(self):
        with open(self.live, "rb") as stream:
            return stream.read()

    def test_zero_bytes_recovered_without_worker_launch(self):
        self.write(self.live, b"", 0o755)
        with mock.patch.object(self.guard, "record") as record:
            self.guard.check("operator")
        self.assertEqual(self.contents(), GOOD)
        self.assertEqual(os.stat(self.live).st_mode & 0o777, 0o555)
        self.assertEqual(record.call_args[0][2], "RUNTIME_WRAPPER_RECOVERED")

    def test_nonempty_valid_syntax_checksum_drift_also_recovered(self):
        self.write(self.live, GOOD.replace(b"exit 0", b"exit 9"), 0o555)
        with mock.patch.object(self.guard, "record"):
            self.guard.check("operator")
        self.assertEqual(self.contents(), GOOD)

    def test_bad_backup_blocks_without_touching_live(self):
        self.write(self.live, b"", 0o755)
        self.write(self.backup, b"", 0o444)
        with self.assertRaises(ValueError):
            self.guard.check("operator")
        self.assertEqual(self.contents(), b"")

    def test_empty_or_invalid_candidate_cannot_replace_good_file(self):
        for data in (b"", b"#!/usr/bin/env bash\n" + b"#" * 50 + b"\nif then\n"):
            with self.assertRaises(ValueError):
                runtime_guard.atomic_replace(self.live, data)
            self.assertEqual(self.contents(), GOOD)

    def test_replace_failure_preserves_previous_complete_file(self):
        with mock.patch.object(os, "replace", side_effect=OSError("replace fault")):
            with self.assertRaises(OSError):
                runtime_guard.atomic_replace(self.live, GOOD.replace(b"exit 0", b"exit 3"))
        self.assertEqual(self.contents(), GOOD)
        self.assertFalse(any(n.startswith(".runtime-stage-") for n in os.listdir(os.path.dirname(self.live))))

    def test_preexec_failure_report_is_visible(self):
        with mock.patch.dict(os.environ, {"SERVICE_RESULT": "exit-code", "EXIT_CODE": "exited", "EXIT_STATUS": "203"}):
            with mock.patch.object(self.guard, "record") as record:
                self.guard.report("operator")
        self.assertEqual(record.call_args[0][2], "RUNTIME_SERVICE_FAILED")
        self.assertIn("EXIT_STATUS=203", record.call_args[0][3])

    def test_same_ssh_quote_loss_truncates_writable_fixture(self):
        self.write(self.live, GOOD, 0o755)
        tokens = shlex.split("ssh host grep -nE 'LOG|log|claude.out|tee' " + shlex.quote(self.live))
        remote = " ".join(tokens[2:])
        result = subprocess.run(["/usr/bin/bash", "-c", remote], stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=5)
        self.assertEqual(result.returncode, 0)  # Last tee masks the earlier command failures.
        self.assertEqual(self.contents(), b"")

    @unittest.skipIf(os.getuid() == 0, "Read-only protection is verified as the actual non-root runtime owner")
    def test_readonly_fixture_blocks_the_same_bad_command(self):
        tokens = shlex.split("ssh host grep -nE 'LOG|log|claude.out|tee' " + shlex.quote(self.live))
        result = subprocess.run(["/usr/bin/bash", "-c", " ".join(tokens[2:])],
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=5)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.contents(), GOOD)


if __name__ == "__main__":
    unittest.main(verbosity=2)

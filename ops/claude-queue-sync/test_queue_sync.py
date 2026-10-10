import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest import mock
import apply_queue_sync as q

BASE = b"#!/usr/bin/env bash\nset -uo pipefail\n" + q.BEGIN.encode() + b"old contract\ntrigger_kind=SCHEDULED "+ "로 기록하라.".encode() + q.END.encode() + b"\nprintf '%s' \"$PAYLOAD\"\n"
PROTOCOL = q.CONTRACT + "\nKeep original authority and read back both records.\n"

class PatchTests(unittest.TestCase):
    def test_guards_and_idempotence(self):
        result = q.patch_payload(BASE, PROTOCOL)
        self.assertEqual(result.split(q.BEGIN.encode())[0], BASE.split(q.BEGIN.encode())[0])
        self.assertEqual(result.split(q.END.encode())[1], BASE.split(q.END.encode())[1])
        self.assertEqual(q.patch_payload(result, PROTOCOL), result)
        self.assertNotIn("trigger_kind=SCHEDULED 로 기록하라.".encode(), result)

    def test_invalid_delimiter_input_is_rejected(self):
        for protocol in [PROTOCOL+"\nEOF\nexec false", PROTOCOL+"\x00", "wrong version"]:
            with self.assertRaises(ValueError):
                q.patch_payload(BASE, protocol)
        with self.assertRaises(ValueError):
            q.patch_payload(BASE.replace(q.BEGIN.encode(), b""), PROTOCOL)

class DeploymentTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.home = Path(self.tmp.name)
        share = self.home/".local/share/claude-runtime-integrity"
        share.mkdir(parents=True)
        (self.home/".local/state/claude-runtime-integrity").mkdir(parents=True)
        (self.home/".local/bin").mkdir(parents=True)
        integrity = Path(__file__).parents[1]/"claude-runtime-integrity/claude_runtime_integrity.py"
        shutil.copyfile(str(integrity), str(self.home/".local/bin/claude-runtime-integrity.py"))
        self.manifest = {"schema_version":1,"roles":{}}
        self.expected = {}
        for role in ["operator", "guardrail"]:
            (self.home/(".local/state/claude-"+role)).mkdir(parents=True)
            live = self.home/(".local/bin/claude-"+role+"-tick.sh")
            live.write_bytes(BASE)
            os.chmod(str(live),0o555)
            sha = q.digest(BASE)
            backup = share/(role+"-"+sha+".sh")
            backup.write_bytes(BASE)
            os.chmod(str(backup),0o444)
            self.manifest["roles"][role] = {"live":str(live),"backup":str(backup),
                "mode":0o555,"size":len(BASE),"sha256":sha}
            self.expected[role] = sha
        self.manifest_path = share/"manifest.json"
        q.atomic_json(self.manifest_path,self.manifest)

    def tearDown(self):
        self.tmp.cleanup()

    def test_dry_run_has_no_mutation_and_apply_keeps_old_backups(self):
        original = self.manifest_path.read_bytes()
        report = q.deploy(self.home,PROTOCOL,False,self.expected,lambda role:None)
        self.assertFalse(report["applied"])
        self.assertEqual(self.manifest_path.read_bytes(),original)
        report = q.deploy(self.home,PROTOCOL,True,self.expected,lambda role:None)
        self.assertTrue(report["applied"])
        new = json.loads(self.manifest_path.read_text())
        for role, entry in new["roles"].items():
            data = Path(entry["live"]).read_bytes()
            self.assertEqual(q.digest(data),entry["sha256"])
            self.assertEqual(Path(entry["backup"]).read_bytes(),data)
            self.assertTrue(Path(self.manifest["roles"][role]["backup"]).exists())
        self.assertEqual(q.deploy(self.home,PROTOCOL,True,self.expected,lambda role:None)["roles"],{})

    def test_busy_role_prevents_any_write(self):
        original = self.manifest_path.read_bytes()
        def busy(role):
            raise RuntimeError("busy")
        with self.assertRaises(RuntimeError):
            q.deploy(self.home,PROTOCOL,True,self.expected,busy)
        self.assertEqual(self.manifest_path.read_bytes(),original)

    def test_baseline_movement_is_rejected(self):
        expected = dict(self.expected,operator="0"*64)
        original = self.manifest_path.read_bytes()
        with self.assertRaises(RuntimeError):
            q.deploy(self.home,PROTOCOL,True,expected,lambda role:None)
        self.assertEqual(self.manifest_path.read_bytes(),original)

    def test_failed_live_swap_rolls_back_manifest_and_wrappers(self):
        guard = q.load_guard(self.home)
        replace = guard.atomic_replace
        failed = [False]
        def fail_once(path,data,mode=0o555):
            if str(path).endswith("claude-operator-tick.sh") and not failed[0]:
                failed[0] = True
                raise OSError("injected swap failure")
            return replace(path,data,mode)
        with mock.patch.object(q,"load_guard",return_value=guard):
            with mock.patch.object(guard,"atomic_replace",side_effect=fail_once):
                with self.assertRaises(OSError):
                    q.deploy(self.home,PROTOCOL,True,self.expected,lambda role:None)
        self.assertEqual(json.loads(self.manifest_path.read_text()),self.manifest)
        for entry in self.manifest["roles"].values():
            self.assertEqual(Path(entry["live"]).read_bytes(),BASE)

if __name__ == "__main__":
    unittest.main()

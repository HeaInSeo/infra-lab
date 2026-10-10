#!/usr/bin/env python3
"""Approved PAYLOAD-only update of existing Claude roles; never starts workers."""
import argparse
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile

CONTRACT = "QSYNC-20261010-v1"
BEGIN = "read -r -d '' PAYLOAD <<'EOF' || true\n"
END = "\nEOF"
EXPECTED = {
    "operator": "c950e50ca9c575bf07da22f07e11a463c06aa2eff4a72e130f452960d3994631",
    "guardrail": "33bb7693ef2d0ad741f338bb4ce848556f98c69e7e3366965c8d13af8cb5b7dc",
}

def digest(data):
    return hashlib.sha256(data).hexdigest()

def patch_payload(data, protocol):
    text = data.decode("utf-8")
    if text.count(BEGIN) != 1:
        raise ValueError("Expected one literal PAYLOAD heredoc")
    before, rest = text.split(BEGIN, 1)
    payload, after = rest.split(END, 1)
    if CONTRACT in payload:
        return data
    if "\nEOF" in protocol or "\x00" in protocol or CONTRACT not in protocol:
        raise ValueError("Invalid protocol input")
    old_trigger = "trigger_kind=SCHEDULED 로 기록하라."
    payload = payload.replace(old_trigger,
        "trigger_kind는 실제 시작 근거가 있을 때 SCHEDULED 또는 MANUAL_EXISTING_SERVICE로 기록하라. 시작 근거가 없으면 UNKNOWN_EXISTING_SERVICE로 기록하고 추정하지 말라.")
    result = (before + BEGIN + protocol.strip() + "\n\n" + payload + END + after).encode("utf-8")
    patched = result.decode("utf-8")
    new_before, new_rest = patched.split(BEGIN, 1)
    new_payload, new_after = new_rest.split(END, 1)
    assert new_before == before and new_after == after, "Shell guards must stay byte-identical"
    return result

def atomic_json(path, value, mode=0o444):
    data = (json.dumps(value, sort_keys=True, indent=2) + "\n").encode("utf-8")
    fd, name = tempfile.mkstemp(prefix=".qsync-", dir=str(path.parent))
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        os.chmod(name, mode)
        os.replace(name, str(path))
        fd2 = os.open(str(path.parent), os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(fd2)
        finally:
            os.close(fd2)
    finally:
        if os.path.exists(name):
            os.unlink(name)

def service_inactive(role):
    p = subprocess.run(["systemctl", "--user", "show", "claude-"+role+"-tick.service",
                        "-p", "ActiveState", "-p", "MainPID"],
                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
    if p.returncode or "ActiveState=inactive" not in p.stdout or "MainPID=0" not in p.stdout:
        raise RuntimeError("Existing "+role+" service is not inactive: "+p.stdout+p.stderr)

def load_guard(home):
    spec = importlib.util.spec_from_file_location("integrity_guard",
        str(home/".local/bin/claude-runtime-integrity.py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module

def deploy(home, protocol, apply=False, expected=None, health=service_inactive):
    home = Path(home)
    expected = EXPECTED if expected is None else expected
    guard = load_guard(home)
    share = home/".local/share/claude-runtime-integrity"
    state = home/".local/state/claude-runtime-integrity"
    manifest_path = share/"manifest.json"
    with open(str(state/"repair.lock"), "a") as repair:
        fcntl.flock(repair, fcntl.LOCK_EX | fcntl.LOCK_NB)
        role_locks = []
        try:
            for role in sorted(expected):
                health(role)
                handle = open(str(home/(".local/state/claude-"+role)/"role.lock"), "a")
                role_locks.append(handle)
                fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
            manifest_bytes, manifest_mode = guard.read_regular(manifest_path)
            if manifest_mode != 0o444:
                raise RuntimeError("Manifest protection mode changed")
            old = json.loads(manifest_bytes.decode("utf-8"))
            new = json.loads(manifest_bytes.decode("utf-8"))
            candidates = {}
            originals = {}
            for role in sorted(expected):
                entry = old["roles"][role]
                live = home/(".local/bin/claude-"+role+"-tick.sh")
                raw, mode = guard.read_regular(live)
                if mode != 0o555 or str(live) != entry["live"]:
                    raise RuntimeError("Protected live path/mode changed: "+role)
                guard.validate_bytes(raw, entry["sha256"])
                if len(raw) != entry["size"]:
                    raise RuntimeError("Size mismatch: "+role)
                candidate = patch_payload(raw, protocol)
                if candidate == raw:
                    continue
                if digest(raw) != expected[role]:
                    raise RuntimeError("Baseline moved; reread and review: "+role)
                guard.validate_bytes(candidate)
                backup = share/(role+"-"+digest(candidate)+".sh")
                candidates[role] = (live, candidate, backup)
                originals[role] = raw
                new["roles"][role] = dict(entry, backup=str(backup), sha256=digest(candidate),
                                          size=len(candidate))
            report = {"contract": CONTRACT, "applied": False, "roles": {
                role: {"sha256": digest(data), "size": len(data)}
                for role, (live, data, backup) in candidates.items()}}
            if not apply or not candidates:
                return report
            for role, (live, data, backup) in candidates.items():
                if backup.exists():
                    existing, mode = guard.read_regular(backup)
                    if existing != data or mode != 0o444:
                        raise RuntimeError("Unexpected versioned backup: "+role)
                else:
                    guard.atomic_replace(backup, data, 0o444)
            new["queue_sync_contract"] = CONTRACT
            try:
                # A crash between the manifest and live swap is repaired by the existing
                # ExecStartPre hook from the fully validated new backup.
                atomic_json(manifest_path, new)
                for role, (live, data, backup) in candidates.items():
                    guard.atomic_replace(live, data, 0o555)
                for role, (live, data, backup) in candidates.items():
                    reread, mode = guard.read_regular(live)
                    if reread != data or mode != 0o555:
                        raise RuntimeError("Live readback failed: "+role)
            except BaseException:
                atomic_json(manifest_path, old)
                for role, raw in originals.items():
                    guard.atomic_replace(candidates[role][0], raw, 0o555)
                raise
            report["applied"] = True
            return report
        finally:
            for handle in role_locks:
                handle.close()

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--home", default=str(Path.home()))
    parser.add_argument("--protocol", default=str(Path(__file__).with_name("QSYNC.md")))
    parser.add_argument("--apply", action="store_true")
    args = parser.parse_args()
    report = deploy(args.home, Path(args.protocol).read_text(encoding="utf-8"), args.apply)
    print(json.dumps(report, sort_keys=True))

if __name__ == "__main__":
    main()

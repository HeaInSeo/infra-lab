#!/usr/bin/env python3
"""Guard the two existing Claude tick wrappers; never launch a worker ourselves."""
import argparse
import datetime
import fcntl
import hashlib
import json
import os
import stat
import subprocess
import tempfile

ROLES = ("operator", "guardrail")


def validate_bytes(data, expected=None):
    if len(data) < 32 or not data.startswith(b"#!/usr/bin/env bash\n") or b"\0" in data:
        raise ValueError("empty, incomplete or invalid shell wrapper")
    if expected and hashlib.sha256(data).hexdigest() != expected:
        raise ValueError("wrapper checksum mismatch")
    check = subprocess.run(["/usr/bin/bash", "-n"], input=data,
                           stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
    if check.returncode:
        raise ValueError("wrapper shell syntax invalid")


def read_regular(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, "rb") as stream:
        info = os.fstat(stream.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():
            raise ValueError("runtime file must be an owned regular file")
        return stream.read(), stat.S_IMODE(info.st_mode)


def atomic_replace(path, data, mode=0o555):
    """Complete and sync a candidate before replacing the existing inode."""
    validate_bytes(data)
    directory = os.path.dirname(path)
    fd, candidate = tempfile.mkstemp(prefix=".runtime-stage-", dir=directory)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        os.chmod(candidate, mode)
        os.replace(candidate, path)
        parent = os.open(directory, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(parent)
        finally:
            os.close(parent)
    finally:
        if os.path.exists(candidate):
            os.unlink(candidate)


class Guard:
    def __init__(self, home):
        self.home = os.path.abspath(home)
        self.share = os.path.join(self.home, ".local/share/claude-runtime-integrity")
        self.state = os.path.join(self.home, ".local/state/claude-runtime-integrity")

    def record(self, role, level, code, detail):
        os.makedirs(self.state, exist_ok=True)
        event = {"utc": datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ"),
                 "role": role, "level": level, "code": code, "detail": detail}
        with open(os.path.join(self.state, "integrity.log"), "a") as stream:
            stream.write(json.dumps(event, sort_keys=True) + "\n")
            stream.flush()
        print(json.dumps(event, sort_keys=True), flush=True)
        if level != "INFO":
            alert = os.path.join(self.home, ".local/bin/claude-role-alert.sh")
            if os.access(alert, os.X_OK):
                subprocess.run([alert, role, level, code, detail], timeout=10,
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def check(self, role):
        os.makedirs(self.state, exist_ok=True)
        with open(os.path.join(self.state, "repair.lock"), "a") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            raw, mode = read_regular(os.path.join(self.share, "manifest.json"))
            if mode & 0o222:
                raise ValueError("manifest must be read-only")
            manifest = json.loads(raw.decode("utf-8"))
            if manifest.get("schema_version") != 1:
                raise ValueError("unsupported manifest")
            entry = manifest["roles"][role]
            live = os.path.join(self.home, ".local/bin/claude-" + role + "-tick.sh")
            digest = entry["sha256"]
            if len(digest) != 64 or any(c not in "0123456789abcdef" for c in digest):
                raise ValueError("invalid approved digest")
            backup = os.path.join(self.share, role + "-" + digest + ".sh")
            if entry["live"] != live or entry["backup"] != backup or entry["mode"] != 0o555:
                raise ValueError("manifest path or mode outside the fixed roles")
            good, backup_mode = read_regular(backup)
            if backup_mode & 0o222 or len(good) != entry["size"]:
                raise ValueError("approved backup changed")
            validate_bytes(good, digest)
            try:
                current, current_mode = read_regular(live)
                validate_bytes(current, digest)
                if current_mode != 0o555:
                    raise ValueError("live wrapper protection changed")
            except (OSError, ValueError) as problem:
                atomic_replace(live, good)
                restored, restored_mode = read_regular(live)
                validate_bytes(restored, digest)
                if restored_mode != 0o555:
                    raise ValueError("restored wrapper protection invalid")
                self.record(role, "WARN", "RUNTIME_WRAPPER_RECOVERED",
                            str(problem) + "; restored exact approved sha256=" + digest)
            else:
                self.record(role, "INFO", "RUNTIME_INTEGRITY_OK", "sha256=" + digest)

    def report(self, role):
        result = os.environ.get("SERVICE_RESULT", "unknown")
        if result != "success":
            detail = "SERVICE_RESULT={}; EXIT_CODE={}; EXIT_STATUS={}".format(
                result, os.environ.get("EXIT_CODE", "unknown"),
                os.environ.get("EXIT_STATUS", "unknown"))
            self.record(role, "ERROR", "RUNTIME_SERVICE_FAILED", detail)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("check", "report"))
    parser.add_argument("role", choices=ROLES)
    args = parser.parse_args()
    guard = Guard(os.path.expanduser("~"))
    try:
        getattr(guard, args.action)(args.role)
    except (OSError, ValueError, KeyError, subprocess.SubprocessError) as problem:
        guard.record(args.role, "ERROR", "RUNTIME_INTEGRITY_BLOCKED", str(problem))
        return 78
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

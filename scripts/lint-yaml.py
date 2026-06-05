#!/usr/bin/env python3
import pathlib
import sys

try:
    import yaml
except ImportError:
    print("WARNING: pyyaml not installed, skipping yaml parse", file=sys.stderr)
    sys.exit(0)

_SKIP = {".git", ".terraform"}
errors = []

for p in pathlib.Path(".").rglob("*"):
    if p.suffix not in (".yaml", ".yml"):
        continue
    if any(part in _SKIP for part in p.parts):
        continue
    try:
        list(yaml.safe_load_all(p.read_text()))
    except Exception as e:
        errors.append(f"{p}: {e}")

if errors:
    for e in errors:
        print(e, file=sys.stderr)
    sys.exit(1)

checked = sum(
    1 for p in pathlib.Path(".").rglob("*")
    if p.suffix in (".yaml", ".yml")
    and not any(part in _SKIP for part in p.parts)
)
print(f"yaml parse ok ({checked} files)")

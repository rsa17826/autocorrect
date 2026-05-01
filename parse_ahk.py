#!/usr/bin/env python3
"""
Parse an AutoHotKey v1/v2 hotstring file into a JSON corrections dictionary.

Usage:
  python3 parse_ahk.py autocorrect.ahk corrections.json
"""

import re
import json
import sys

# AHK hotstring pattern:
#   :options:trigger::replacement
# options can include: *, ?, c, c1, r, b0, etc. (we mostly ignore them)
# A line like  ::word::  (empty replacement) is an exception/no-op marker — skip it.
HOTSTRING_RE = re.compile(r"^:([^:]*):(.*?)::(.*?)(?:\s*;.*)?$")


def parse_ahk(path: str) -> dict[str, str]:
  corrections: dict[str, str] = {}
  skipped = 0

  with open(path, encoding="utf-8", errors="replace") as f:
    for lineno, raw in enumerate(f, 1):
      line = raw.strip()

      # Skip comments and blank lines
      if not line or line.startswith(";"):
        continue

      m = HOTSTRING_RE.match(line)
      if not m:
        continue

      options = m.group(1) # e.g. "?", "c", "*", ""
      trigger = m.group(2) # the misspelling
      replace = m.group(3) # the correction

      # Skip no-op / exception markers (empty replacement)
      if not replace.strip():
        skipped += 1
        continue

      # Skip if trigger is also empty (malformed)
      if not trigger.strip():
        skipped += 1
        continue

      # Unescape AHK escape sequences we care about
      replace = replace.replace("`n", "\n").replace("`t", "\t")

      # AHK uses {Enter}, {Tab}, etc. — simplify them
      replace = re.sub(r"\{Enter\}", "\n", replace, flags=re.IGNORECASE)
      replace = re.sub(r"\{Tab\}", "\t", replace, flags=re.IGNORECASE)
      replace = re.sub(r"\{[^}]+\}", "", replace) # drop other {keys}

      corrections[trigger] = replace

  return corrections, skipped


def main():
  if len(sys.argv) < 3:
    print(f"Usage: {sys.argv[0]} input.ahk output.json")
    sys.exit(1)

  ahk_path = sys.argv[1]
  json_path = sys.argv[2]

  corrections, skipped = parse_ahk(ahk_path)

  with open(json_path, "w", encoding="utf-8") as f:
    json.dump(corrections, f, ensure_ascii=False, indent=2, sort_keys=True)

  print(f"Parsed {len(corrections)} corrections ({skipped} no-op entries skipped)")
  print(f"Written to {json_path}")

  # Show a few examples
  print("\nSample entries:")
  for k, v in list(corrections.items())[:10]:
    print(f"  {k!r:30s} -> {v!r}")


if __name__ == "__main__":
  main()

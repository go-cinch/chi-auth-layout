#!/usr/bin/env python3
"""Refresh generated action seeds while preserving stable codes by seed row ID."""
import re
import sys
from pathlib import Path

previous, generated = map(Path, sys.argv[1:])
row = re.compile(r"(?m)^(\s*\((\d+),\s*'[^']+',\s*')([^']+)(')")
codes = {match[2]: match[3] for match in row.finditer(previous.read_text())}
content = row.sub(lambda match: match[1] + codes.get(match[2], match[3]) + match[4], generated.read_text())
previous.write_text(content)

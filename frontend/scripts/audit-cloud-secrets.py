#!/usr/bin/env python3
"""Fail when configured server secrets appear in exported browser assets or logs.

Values are read from the environment and are never printed. This complements,
not replaces, browser network inspection and secret-manager access controls.
"""
import argparse
import os
from pathlib import Path
import sys

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--root', action='append', required=True, help='Public asset directory or captured log file')
parser.add_argument('--env', action='append', default=[], help='Environment name holding a server secret/canary')
args = parser.parse_args()
names = args.env or ['CRYPTORAILS_API_KEY', 'CRYPTORAILS_WEBHOOK_SECRET', 'OPENROUTER_API_KEY', 'STRIPE_SECRET_KEY', 'STRIPE_WEBHOOK_SECRET', 'GOOGLE_CLIENT_SECRET', 'APPLE_CLIENT_SECRET']
secrets = {name: os.environ[name].encode() for name in names if os.environ.get(name)}
if not secrets:
    parser.error('At least one configured secret/canary is required; an empty scan is not evidence')
failures = []
files = 0
for value in args.root:
    root = Path(value)
    if not root.exists():
        parser.error('Missing scan target: ' + str(root))
    targets = [root] if root.is_file() else sorted(p for p in root.rglob('*') if p.is_file() and not p.is_symlink())
    for target in targets:
        data = target.read_bytes()
        files += 1
        for name, secret in secrets.items():
            if secret in data:
                failures.append((name, target))
if not files:
    parser.error('No files found; an empty scan is not evidence')
for name, target in failures:
    print(f'FAIL: {name} appears in {target}', file=sys.stderr)
print(f'Scanned {files} files for {len(secrets)} configured server secret values; {len(failures)} exposures.')
sys.exit(1 if failures else 0)

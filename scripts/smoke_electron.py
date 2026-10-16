#!/usr/bin/env python3
"""Verify the packaged Chromium window, private Go engine and quit cleanup."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time

parser = argparse.ArgumentParser()
parser.add_argument('--executable', required=True)
args = parser.parse_args()
with tempfile.TemporaryDirectory(prefix='videoflow-electron-smoke-') as temp:
    env = dict(os.environ, HOME=temp, XDG_CONFIG_HOME=temp,
               DATABASE_PATH=str(Path(temp) / 'db.sqlite'), STORAGE_ROOT=str(Path(temp) / 'storage'))
    for key in ['OPENROUTER_API_KEY', 'ELECTRON_RUN_AS_NODE']:
        env.pop(key, None)
    result = subprocess.run([str(Path(args.executable).resolve()), '--smoke-test'], env=env,
                            capture_output=True, text=True, timeout=60)
    if result.returncode:
        raise RuntimeError(result.stdout + result.stderr)
    ready = next((json.loads(line) for line in result.stdout.splitlines() if line.startswith('{"desktop_ready"')), None)
    assert ready and ready['desktop_ready'], result.stdout + result.stderr
    for attempt in range(30):
        try:
            os.kill(ready['engine_pid'], 0)
        except ProcessLookupError:
            break
        time.sleep(.1)
    else:
        raise AssertionError('Electron quit left the Go engine running')
    print('Packaged Electron window rendered 6eek and quit without leaving its Go engine running.')

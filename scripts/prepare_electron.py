#!/usr/bin/env python3
"""Reuse a verified native download as Electron's external runtime payload."""
import argparse
import json
import struct
from pathlib import Path, PurePosixPath
import shutil
import stat
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parent.parent
REQUIRED = ['videoflow', 'ffmpeg', 'ffprobe', 'frontend/index.html', 'fonts/NotoSans-Regular.ttf',
            'fonts/NotoSansCJKsc-Regular.otf', 'licenses/NOTICE.txt', 'media-sources.zip']

def prepare(archive, destination, target=None):
    destination = Path(destination)
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=destination.parent) as temp:
        temp = Path(temp)
        with zipfile.ZipFile(archive) as bundle:
            for item in bundle.infolist():
                name = PurePosixPath(item.filename)
                mode = item.external_attr >> 16
                if (not name.parts or name.parts[0] != '6eek' or '..' in name.parts or
                        '\\' in item.filename or stat.S_ISLNK(mode) or
                        any(part.startswith('.env') or '.sqlite' in part or part == 'storage' for part in name.parts)):
                    raise ValueError(f'Unsafe or private archive entry: {item.filename}')
                entry_path = temp.joinpath(*name.parts)
                if item.is_dir():
                    entry_path.mkdir(parents=True, exist_ok=True)
                else:
                    entry_path.parent.mkdir(parents=True, exist_ok=True)
                    entry_path.write_bytes(bundle.read(item))
                    entry_path.chmod(mode & 0o777)
        folder = temp / '6eek'
        for name in REQUIRED:
            if not (folder / name).is_file():
                raise ValueError(f'Missing runtime resource: {name}')
        for name in ['videoflow', 'ffmpeg', 'ffprobe']:
            if not (folder / name).stat().st_mode & 0o111:
                raise ValueError(f'Runtime resource is not executable: {name}')
        if target:
            system, arch = target.split('-')
            for program in ['videoflow', 'ffmpeg', 'ffprobe']:
                header = (folder / program).read_bytes()[:32]
                if system == 'linux':
                    valid = len(header) >= 20 and header[:6] == b'\x7fELF\x02\x01' and struct.unpack('<H', header[18:20])[0] == {'amd64': 62, 'arm64': 183}[arch]
                else:
                    valid = len(header) >= 8 and header[:4] == b'\xcf\xfa\xed\xfe' and struct.unpack('<I', header[4:8])[0] == {'amd64': 0x1000007, 'arm64': 0x100000c}[arch]
                if not valid:
                    raise ValueError(f'{program} does not match {target}')
            (folder / 'target.json').write_text(json.dumps({'platform': {'darwin': 'darwin', 'linux': 'linux'}[system], 'arch': {'amd64': 'x64', 'arm64': 'arm64'}[arch]}))
        if destination.exists():
            shutil.rmtree(destination)
        shutil.move(str(folder), destination)

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--archive', required=True)
    parser.add_argument('--target', required=True, choices=['linux-amd64', 'linux-arm64', 'darwin-amd64', 'darwin-arm64'])
    args = parser.parse_args()
    prepare(args.archive, ROOT / 'electron/payload/6eek', args.target)
    print('Electron runtime prepared; includes native engine, interface, media tools, fonts and licenses.')

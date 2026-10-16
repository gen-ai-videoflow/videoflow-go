#!/usr/bin/env python3
"""Build beginner downloads after `cd frontend && npm ci && npm run build`."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = [('linux', 'amd64'), ('linux', 'arm64'), ('darwin', 'amd64'), ('darwin', 'arm64')]


LICENSES = ['NOTICE.txt', 'ffmpeg-COPYING.GPLv3', 'ffmpeg-LICENSE.md', 'freetype-FTL.TXT', 'freetype-GPLv2.TXT', 'freetype-LICENSE.TXT', 'fribidi-COPYING', 'harfbuzz-COPYING', 'libass-COPYING', 'x264-COPYING', 'NotoSans-OFL.txt', 'NotoSansCJK-OFL.txt']


def validate_media(media, target):
    required = ['ffmpeg', 'ffprobe', 'fonts/NotoSans-Regular.ttf', 'fonts/NotoSansCJKsc-Regular.otf', 'media-sources.zip', 'BUILD-INFO.txt'] + ['licenses/' + name for name in LICENSES]
    if any(not (media / file).is_file() for file in required + ['manifest.json']):
        raise ValueError(f'media bundle is missing for {target}; run scripts/build_media.py on the target platform first')
    lock_path = ROOT / 'scripts/media-sources.json'
    builder = ROOT / 'scripts/build_media.py'
    lock_hash = hashlib.sha256(lock_path.read_bytes()).hexdigest()
    builder_hash = hashlib.sha256(builder.read_bytes()).hexdigest()
    manifest = json.loads((media / 'manifest.json').read_text())
    if manifest.get('target') != target or manifest.get('source_lock_sha256') != lock_hash or manifest.get('builder_sha256') != builder_hash:
        raise ValueError(f'media bundle is stale or built for the wrong platform: {target}')
    for file in required:
        if manifest.get('artifacts', {}).get(file) != hashlib.sha256((media / file).read_bytes()).hexdigest():
            raise ValueError(f'media bundle artifact has changed: {file}')
    sources = json.loads(lock_path.read_text())
    try:
        with zipfile.ZipFile(media / 'media-sources.zip') as archive:
            names = [item['filename'] for item in sources.values()] + ['media-sources.json', 'build_media.py', 'BUILD.txt']
            if sorted(archive.namelist()) != sorted(names):
                raise ValueError('source archive entries do not match the source lock')
            for item in sources.values():
                if hashlib.sha256(archive.read(item['filename'])).hexdigest() != item['sha256']:
                    raise ValueError(f'wrong source archive: {item["filename"]}')
            if hashlib.sha256(archive.read('build_media.py')).hexdigest() != builder_hash or hashlib.sha256(archive.read('media-sources.json')).hexdigest() != lock_hash:
                raise ValueError('corresponding build script or source lock does not match')
            if not archive.read('BUILD.txt').strip():
                raise ValueError('source build instructions are empty')
    except (zipfile.BadZipFile, KeyError, ValueError, RuntimeError) as error:
        raise ValueError(f'invalid corresponding media sources: {error}') from error


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--target', choices=[f'{s}-{a}' for s, a in TARGETS])
    parser.add_argument('--media-root', default=str(ROOT / 'bin/media'))
    parser.add_argument('--output', default=str(ROOT / 'dist'))
    args = parser.parse_args()
    if not (ROOT / 'frontend/build/index.html').exists():
        parser.error('Build the frontend first: cd frontend && npm ci && npm run build')
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    checksums = []
    for system, arch in TARGETS:
        target = f'{system}-{arch}'
        if args.target and target != args.target:
            continue
        media = Path(args.media_root).resolve() / target
        try:
            validate_media(media, target)
        except (OSError, ValueError, KeyError) as error:
            parser.error(str(error))
        with tempfile.TemporaryDirectory(prefix='videoflow-package-') as temp:
            folder = Path(temp) / '6eek'
            folder.mkdir()
            binary = 'videoflow.exe' if system == 'windows' else 'videoflow'
            env = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED='0')
            subprocess.run(['go', 'build', '-trimpath', '-o', str(folder / binary), './cmd/server'], cwd=ROOT, env=env, check=True)
            shutil.copytree(ROOT / 'frontend/build', folder / 'frontend')
            for program in ['ffmpeg', 'ffprobe']:
                shutil.copy2(media / program, folder / program)
                (folder / program).chmod(0o755)
            for directory in ['fonts', 'licenses']:
                shutil.copytree(media / directory, folder / directory)
            for file in ['media-sources.zip', 'BUILD-INFO.txt']:
                shutil.copy2(media / file, folder / file)
            shutil.copyfile(ROOT / 'docs/QUICKSTART.txt', folder / 'START HERE.txt')
            if system == 'windows':
                (folder / 'Start 6eek.bat').write_text('@echo off\ncd /d "%~dp0"\nvideoflow.exe --desktop\nif errorlevel 1 pause\n', encoding='utf-8')
            else:
                launcher = folder / ('Start 6eek.command' if system == 'darwin' else 'Start 6eek.sh')
                launcher.write_text('#!/bin/sh\ncd "$(dirname "$0")" || exit 1\nexec ./videoflow --desktop\n')
                launcher.chmod(0o755)
            archive = output / f'6eek-{target}.zip'
            with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as bundle:
                for path in sorted(folder.rglob('*')):
                    if path.is_file():
                        bundle.write(path, path.relative_to(folder.parent))
            checksums.append(f'{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}')
            print(archive)
    (output / 'SHA256SUMS.txt').write_text('\n'.join(checksums) + '\n')


if __name__ == '__main__':
    main()

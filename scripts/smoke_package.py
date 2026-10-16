#!/usr/bin/env python3
"""Launch an extracted native download with no system tools on PATH."""
import argparse
import json
import os
from pathlib import Path
import platform
import socket
import subprocess
import tempfile
import time
import urllib.request
import zipfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--archive', required=True)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix='6eek clean computer ') as temp:
        root = Path(temp)
        with zipfile.ZipFile(args.archive) as archive:
            assert not any('.sqlite' in item.filename or '/storage/' in item.filename or '/.env' in item.filename for item in archive.infolist()), 'Private runtime data in package'
            archive.extractall(root)
            for item in archive.infolist():
                (root / item.filename).chmod((item.external_attr >> 16) & 0o777)
        folder = root / '6eek'
        for file in ['ffmpeg', 'ffprobe', 'fonts/NotoSans-Regular.ttf', 'fonts/NotoSansCJKsc-Regular.otf', 'media-sources.zip', 'licenses/NOTICE.txt']:
            assert (folder / file).is_file(), f'Missing {file}'
        ffmpeg = folder / 'ffmpeg'
        if platform.system() == 'Linux':
            result = subprocess.run(['ldd', str(ffmpeg)], capture_output=True, text=True)
            assert 'not a dynamic executable' in result.stdout + result.stderr or 'statically linked' in result.stdout + result.stderr, result.stdout + result.stderr
        else:
            result = subprocess.check_output(['otool', '-L', str(ffmpeg)], text=True)
            for line in result.splitlines()[1:]:
                assert line.strip().startswith(('/usr/lib/', '/System/Library/')), f'Unbundled library: {line}'
        version = subprocess.check_output([str(ffmpeg), '-version'], text=True)
        assert '--enable-libass' in version and '--enable-libx264' in version
        assert '--enable-nonfree' not in version
        socket_probe = socket.socket()
        socket_probe.bind(('127.0.0.1', 0))
        port = socket_probe.getsockname()[1]
        socket_probe.close()
        home = root / 'user'
        home.mkdir()
        no_tools = root / 'no-system-tools'
        no_tools.mkdir()
        env = dict(os.environ, PATH=str(no_tools), HOME=str(home), XDG_CONFIG_HOME=str(home / '.config'), PORT=str(port))
        for name in ['DATABASE_PATH', 'STORAGE_ROOT', 'OPENROUTER_API_KEY', 'LD_LIBRARY_PATH', 'DYLD_LIBRARY_PATH']:
            env.pop(name, None)
        with (root / 'app.log').open('w') as log:
            app = subprocess.Popen([str(folder/'videoflow'), '--desktop', '--no-browser'], cwd=root, env=env, stdout=log, stderr=subprocess.STDOUT)
            try:
                base = f'http://127.0.0.1:{port}'
                for attempt in range(60):
                    try:
                        with urllib.request.urlopen(base+'/api/health', timeout=1) as response:
                            health = json.load(response)
                        break
                    except OSError:
                        if app.poll() is not None:
                            raise RuntimeError((root / 'app.log').read_text())
                        time.sleep(.1)
                else:
                    raise RuntimeError('Packaged app did not start')
                assert health['media_ready'] is True, health
                assert health['missing_keys'] == ['OPENROUTER_API_KEY'], health
                with urllib.request.urlopen(base+'/settings') as response:
                    assert b'<html' in response.read()
                print('Extracted app serves UI/API and finds bundled FFmpeg with an empty system PATH.')
            finally:
                app.terminate()
                app.wait(timeout=15)
        # Prove the bundled libraries/font can process audio and visibly render captions.
        fixture = root / 'fixture.mp4'
        subprocess.run([str(ffmpeg), '-y', '-f', 'lavfi', '-i', 'color=c=blue:s=320x240:d=2', '-f', 'lavfi', '-i', 'sine=frequency=440:duration=2', '-c:v', 'libx264', '-c:a', 'aac', '-shortest', str(fixture)], check=True, capture_output=True)
        subtitles = root / 'captions.srt'
        subtitles.write_text('1\n00:00:00,000 --> 00:00:01,800\nBundled captions 字幕\n')
        # No system font provider is built in; only the supplied font is available.
        vf = f"subtitles='{subtitles}':fontsdir='{folder/'fonts'}':force_style='FontName=Noto Sans CJK SC'"
        captioned = root / 'captioned.mp4'
        output = subprocess.run([str(ffmpeg), '-y', '-i', str(fixture), '-vf', vf, '-c:v', 'libx264', '-c:a', 'copy', str(captioned)], check=True, capture_output=True, text=True)
        assert 'failed to find any fallback' not in output.stderr, output.stderr
        def frame(path):
            return subprocess.check_output([str(ffmpeg), '-v', 'error', '-ss', '1', '-i', str(path), '-frames:v', '1', '-f', 'md5', '-'])
        assert frame(fixture) != frame(captioned), 'Captions were not rendered'
        audio = root / 'audio.wav'
        subprocess.run([str(ffmpeg), '-v', 'error', '-i', str(fixture), '-vn', '-ar', '16000', '-ac', '1', '-c:a', 'pcm_s16le', str(audio)], check=True)
        assert audio.stat().st_size > 44
        probe = json.loads(subprocess.check_output([str(folder/'ffprobe'), '-v', 'error', '-show_format', '-of', 'json', str(captioned)]))
        assert float(probe['format']['duration']) > 1
        print('Bundled FFmpeg/FFprobe encode video, extract audio and render captions with the bundled font.')


if __name__ == '__main__':
    main()

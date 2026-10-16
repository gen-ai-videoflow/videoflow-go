#!/usr/bin/env python3
"""Build native, self-contained FFmpeg and package its corresponding sources.

Developer tools: C/C++ compiler, make, cmake, pkg-config, meson and ninja.
No build tools are needed by people using the resulting download.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]
LOCK = ROOT / 'scripts/media-sources.json'


def native_target():
    system = {'Linux': 'linux', 'Darwin': 'darwin'}.get(platform.system())
    arch = {'x86_64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64'}.get(platform.machine())
    if not system or not arch:
        raise RuntimeError('Build media on a supported native macOS or Linux machine')
    return f'{system}-{arch}'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', default=str(ROOT / 'bin/media'))
    parser.add_argument('--cache', default=str(ROOT / 'bin/media-sources'))
    parser.add_argument('--jobs', type=int, default=min(os.cpu_count() or 2, 8))
    args = parser.parse_args()
    target = native_target()
    cache = Path(args.cache).resolve()
    cache.mkdir(parents=True, exist_ok=True)
    bundle = Path(args.output).resolve() / target
    bundle.mkdir(parents=True, exist_ok=True)
    work = bundle / 'work'
    prefix = bundle / 'prefix'
    if prefix.exists():
        shutil.rmtree(prefix)
    prefix.mkdir()
    sources = json.loads(LOCK.read_text())
    for name, item in sources.items():
        dest = cache / item['filename']
        if not dest.exists():
            print(f'Downloading {name}', flush=True)
            request = urllib.request.Request(item['url'], headers={'User-Agent': '6eek-media-build'})
            with urllib.request.urlopen(request, timeout=120) as response:
                data = response.read()
            if hashlib.sha256(data).hexdigest() != item['sha256']:
                raise RuntimeError(f'Checksum mismatch for {name}')
            dest.write_bytes(data)
        if hashlib.sha256(dest.read_bytes()).hexdigest() != item['sha256']:
            raise RuntimeError(f'Checksum mismatch for cached {name}')
    # Each invocation builds cleanly; stale objects must not cross architectures.
    if work.exists():
        shutil.rmtree(work)
    work.mkdir()
    trees = {}
    for name, item in sources.items():
        if name.startswith('font'):
            continue
        folder = work / name
        folder.mkdir()
        with tarfile.open(cache / item['filename']) as archive:
            archive.extractall(folder, filter='data')
        trees[name] = next(path for path in folder.iterdir() if path.is_dir())
    env = dict(os.environ)
    env['PKG_CONFIG_LIBDIR'] = str(prefix / 'lib/pkgconfig')
    env['PKG_CONFIG_PATH'] = str(prefix / 'lib/pkgconfig')
    env['CFLAGS'] = '-O2 -fPIC'
    env['CXXFLAGS'] = '-O2 -fPIC'
    if platform.system() == 'Darwin':
        env['MACOSX_DEPLOYMENT_TARGET'] = '12.0'
    jobs = str(args.jobs)

    def run(command, cwd):
        print('Building:', ' '.join(map(str, command)), flush=True)
        subprocess.run(list(map(str, command)), cwd=cwd, env=env, check=True)

    ft = trees['freetype']
    run(['cmake', '-S', ft, '-B', ft/'build', '-DCMAKE_BUILD_TYPE=Release', f'-DCMAKE_INSTALL_PREFIX={prefix}', '-DBUILD_SHARED_LIBS=OFF', '-DFT_DISABLE_ZLIB=ON', '-DFT_DISABLE_BZIP2=ON', '-DFT_DISABLE_PNG=ON', '-DFT_DISABLE_HARFBUZZ=ON', '-DFT_DISABLE_BROTLI=ON'], ft)
    run(['cmake', '--build', ft/'build', '-j', jobs], ft)
    run(['cmake', '--install', ft/'build'], ft)
    fr = trees['fribidi']
    run(['meson', 'setup', 'build', f'--prefix={prefix}', '--libdir=lib', '--default-library=static', '--buildtype=release', '-Ddocs=false', '-Dtests=false', '-Dbin=false'], fr)
    run(['meson', 'compile', '-C', 'build', '-j', jobs], fr)
    run(['meson', 'install', '-C', 'build'], fr)
    hb = trees['harfbuzz']
    run(['meson', 'setup', 'build', f'--prefix={prefix}', '--libdir=lib', '--default-library=static', '--buildtype=release', '-Dglib=disabled', '-Dgobject=disabled', '-Dcairo=disabled', '-Dicu=disabled', '-Dgraphite2=disabled', '-Dfreetype=enabled', '-Dtests=disabled', '-Ddocs=disabled', '-Dutilities=disabled', '--auto-features=disabled'], hb)
    run(['meson', 'compile', '-C', 'build', '-j', jobs], hb)
    run(['meson', 'install', '-C', 'build'], hb)
    ass = trees['libass']
    run(['./configure', f'--prefix={prefix}', '--disable-shared', '--enable-static', '--disable-asm', '--disable-fontconfig', '--disable-coretext', '--disable-directwrite', '--disable-require-system-font-provider'], ass)
    run(['make', '-j', jobs], ass)
    run(['make', 'install'], ass)
    x264 = trees['x264']
    run(['./configure', f'--prefix={prefix}', '--enable-static', '--enable-pic', '--disable-cli', '--disable-opencl', '--disable-asm'], x264)
    run(['make', '-j', jobs], x264)
    run(['make', 'install'], x264)
    ffmpeg = trees['ffmpeg']
    flags = ['--disable-shared', '--enable-static', '--disable-autodetect', '--disable-doc', '--disable-ffplay', '--disable-network', '--disable-x86asm', '--enable-gpl', '--enable-version3', '--enable-libass', '--enable-libx264', '--disable-encoders', '--enable-encoder=libx264,aac,pcm_s16le,mjpeg,png,rawvideo', '--pkg-config-flags=--static']
    if platform.system() == 'Linux':
        flags.append('--extra-ldflags=-static')
    run(['./configure', f'--prefix={prefix}', *flags], ffmpeg)
    run(['make', '-j', jobs], ffmpeg)
    run(['make', 'install'], ffmpeg)
    for program in ['ffmpeg', 'ffprobe']:
        shutil.copy2(prefix / 'bin' / program, bundle / program)
        (bundle / program).chmod(0o755)
    fonts = bundle / 'fonts'
    fonts.mkdir(exist_ok=True)
    for name in ['font', 'font_cjk']:
        shutil.copy2(cache / sources[name]['filename'], fonts / sources[name]['filename'])
    licenses = bundle / 'licenses'
    licenses.mkdir(exist_ok=True)
    for name, files in {
        'ffmpeg': ['COPYING.GPLv3', 'LICENSE.md'],
        'freetype': ['docs/FTL.TXT', 'docs/GPLv2.TXT', 'LICENSE.TXT'],
        'fribidi': ['COPYING'], 'harfbuzz': ['COPYING'],
        'libass': ['COPYING'], 'x264': ['COPYING'],
    }.items():
        for file in files:
            source = trees[name] / file
            if not source.exists():
                raise RuntimeError(f'Required license is missing: {name}/{file}')
            shutil.copy2(source, licenses / (name + '-' + source.name))
    for name in ['font_license', 'font_cjk_license']:
        shutil.copy2(cache / sources[name]['filename'], licenses / sources[name]['filename'])
    (licenses / 'NOTICE.txt').write_text('This download uses FFmpeg built with GPLv3 components, libass, FreeType, FriBidi, HarfBuzz, x264 and the OFL Noto Sans font. License texts are in this folder. Exact source archives, download checksums and the build script are provided in ../media-sources.zip. FFmpeg runs as a separate process; it is not linked into 6eek.\n')
    with zipfile.ZipFile(bundle / 'media-sources.zip', 'w', zipfile.ZIP_STORED) as archive:
        for item in sources.values():
            archive.write(cache / item['filename'], item['filename'])
        archive.write(LOCK, 'media-sources.json')
        archive.write(Path(__file__), 'build_media.py')
        archive.writestr('BUILD.txt', 'Install a C/C++ compiler, make, cmake, pkg-config, meson and ninja on the target platform. Place build_media.py and media-sources.json under scripts/ in a project folder. Put these archives in a cache folder. Run python3 scripts/build_media.py --cache /path/to/cache --output /path/to/output. The builder verifies every source checksum.\n')
    version = subprocess.check_output([str(bundle/'ffmpeg'), '-version'], text=True)
    (bundle / 'BUILD-INFO.txt').write_text(version)
    artifacts = ['ffmpeg', 'ffprobe', 'BUILD-INFO.txt', 'media-sources.zip']
    artifacts += [str(path.relative_to(bundle)) for folder in ['fonts', 'licenses'] for path in (bundle / folder).iterdir() if path.is_file()]
    manifest = {
        'target': target,
        'source_lock_sha256': hashlib.sha256(LOCK.read_bytes()).hexdigest(),
        'builder_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'artifacts': {name: hashlib.sha256((bundle / name).read_bytes()).hexdigest() for name in artifacts},
    }
    (bundle / 'manifest.json').write_text(json.dumps(manifest, indent=2)+'\n')
    print(f'Native media bundle ready: {bundle}', flush=True)


if __name__ == '__main__':
    main()

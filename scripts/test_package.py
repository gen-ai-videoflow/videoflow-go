"""Packaging must refuse downloads that omit media dependencies."""
import subprocess
import tempfile
import shutil
import platform
import json
import hashlib
import sys
import unittest
import importlib.util
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class PackageSafetyTests(unittest.TestCase):
    def test_media_bundle_is_required(self):
        spec = importlib.util.spec_from_file_location('six_eek_packager', ROOT / 'scripts/package.py')
        packager = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(packager)
        with self.assertRaisesRegex(ValueError, 'media bundle is missing'):
            packager.validate_media(Path('/nonexistent/6eek-media'), 'linux-amd64')


class CompleteBundleTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        system = {'Linux': 'linux', 'Darwin': 'darwin'}[platform.system()]
        arch = {'x86_64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64'}[platform.machine()]
        cls.target = f'{system}-{arch}'
        cls.original = ROOT / 'bin/media' / cls.target
        if not (cls.original / 'manifest.json').exists():
            raise unittest.SkipTest('Native bundle is not built')

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bundle = self.root / self.target
        shutil.copytree(self.original, self.bundle, ignore=shutil.ignore_patterns('work', 'prefix'))

    def reject(self):
        result = subprocess.run([sys.executable, str(ROOT/'scripts/package.py'), '--target', self.target, '--media-root', str(self.root), '--output', str(self.root/'output')], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        return result.stderr

    def test_missing_dependency_license_is_rejected(self):
        (self.bundle/'licenses/libass-COPYING').unlink()
        self.assertIn('media bundle is missing', self.reject())

    def test_corrupt_corresponding_sources_are_rejected(self):
        path = self.bundle/'media-sources.zip'
        path.write_bytes(b'not a source archive')
        manifest_path = self.bundle/'manifest.json'
        manifest = json.loads(manifest_path.read_text())
        # Even a resealed archive must actually contain the matching sources.
        manifest['artifacts']['media-sources.zip'] = hashlib.sha256(path.read_bytes()).hexdigest()
        manifest_path.write_text(json.dumps(manifest))
        self.assertIn('invalid corresponding media sources', self.reject())

    def test_changed_builder_requires_a_rebuild(self):
        path = self.bundle/'manifest.json'
        manifest = json.loads(path.read_text())
        manifest['builder_sha256'] = '0'*64
        path.write_text(json.dumps(manifest))
        self.assertIn('stale or built for the wrong platform', self.reject())


if __name__ == '__main__':
    unittest.main()

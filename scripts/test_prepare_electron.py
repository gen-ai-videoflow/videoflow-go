import importlib.util
from pathlib import Path
import tempfile
import struct
import unittest
import zipfile

spec = importlib.util.spec_from_file_location('prepare', Path(__file__).with_name('prepare_electron.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class PayloadTests(unittest.TestCase):
    def bundle(self, root, extra=None, missing=None):
        archive = root / 'native.zip'
        with zipfile.ZipFile(archive, 'w') as target:
            for name in module.REQUIRED:
                if name == missing:
                    continue
                item = zipfile.ZipInfo('6eek/' + name)
                item.external_attr = 0o100755 << 16
                content = bytearray(32)
                content[:6] = b'\x7fELF\x02\x01'
                content[18:20] = struct.pack('<H', 62)
                target.writestr(item, content)
            if extra:
                target.writestr(extra, b'private-or-unsafe')
        return archive

    def test_preserves_executable_and_resources(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            module.prepare(self.bundle(root), root / 'payload')
            self.assertTrue((root / 'payload/videoflow').stat().st_mode & 0o111)
            self.assertTrue((root / 'payload/media-sources.zip').is_file())

    def test_rejects_traversal_and_private_data(self):
        for extra in ['6eek/../../escape', '/6eek/escape', '6eek/db.sqlite', '6eek/.env', '6eek/storage/a']:
            with self.subTest(extra=extra), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                with self.assertRaises(ValueError):
                    module.prepare(self.bundle(root, extra), root / 'payload')
                self.assertFalse((root / 'payload').exists())

    def test_rejects_wrong_native_architecture(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            with self.assertRaisesRegex(ValueError, 'does not match'):
                module.prepare(self.bundle(root), root / 'payload', 'linux-arm64')
            module.prepare(self.bundle(root), root / 'payload', 'linux-amd64')
            self.assertIn('x64', (root / 'payload/target.json').read_text())

    def test_missing_resource_keeps_previous_payload(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            destination = root / 'payload'
            destination.mkdir()
            (destination / 'previous').write_text('keep')
            with self.assertRaises(ValueError):
                module.prepare(self.bundle(root, missing='ffmpeg'), destination)
            self.assertEqual((destination / 'previous').read_text(), 'keep')

if __name__ == '__main__':
    unittest.main()

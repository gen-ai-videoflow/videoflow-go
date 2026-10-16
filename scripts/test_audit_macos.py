import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('audit', Path(__file__).with_name('audit_macos.py'))
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)

class ReleasePolicyTests(unittest.TestCase):
    def report(self):
        result = {name: {'returncode': 0, 'detail': ''} for name in audit.REQUIRED_CHECKS}
        result['signature_identity']['detail'] = 'Authority=Developer ID Application: 6eek (TEAM123)\nTeamIdentifier=TEAM123'
        return result

    def test_rejects_broken_signature_even_when_process_launches(self):
        report = self.report()
        report['bundle_signature']['returncode'] = 1
        self.assertIn('bundle_signature', audit.release_errors(report))

    def test_rejects_adhoc_signature(self):
        report = self.report()
        report['signature_identity']['detail'] = 'Signature=adhoc\nTeamIdentifier=not set'
        self.assertIn('Developer ID Application identity', audit.release_errors(report))

    def test_rejects_missing_ticket_or_gatekeeper_rejection(self):
        for name in ['notarization_ticket', 'gatekeeper', 'engine_signature_ffmpeg']:
            report = self.report()
            report[name]['returncode'] = 1
            self.assertIn(name, audit.release_errors(report))

    def test_requires_every_check(self):
        self.assertTrue(audit.release_errors({}))

    def test_accepts_verified_developer_id_release(self):
        self.assertEqual(audit.release_errors(self.report()), [])

if __name__ == '__main__':
    unittest.main()

#!/usr/bin/env python3
"""Test fail-closed shell preflight in temporary trees, never building a product.

The `go` shell fixture below ONLY supplies tool-version/error responses to test
build.sh control flow. It is not a Go dependency, library, compiler or test of
production compilation. No service, host configuration or existing Go is modified.
"""
from __future__ import annotations
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]

class BuildGateTests(unittest.TestCase):
    def run_case(self, version: str = 'go1.23.2', extra: dict[str,str] | None = None, query_fails: bool = False) -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory(prefix='scyllascope-build-preflight-') as tmp:
            root = Path(tmp)
            shutil.copy2(ROOT / 'build.sh', root / 'build.sh')
            binary = root / 'scyllascope'
            original = b'pre-existing-binary-preserved-on-failure\n'
            binary.write_bytes(original)
            marker = root / 'BUILD-VERIFIED.sha256'
            marker.write_text(hashlib.sha256(original).hexdigest() + '  scyllascope\n')
            commands = root / 'commands'; commands.mkdir()
            go = commands / 'go'
            go.write_text('#!/bin/sh\n' +
                ('exit 42\n' if query_fails else
                 'if [ "$1 $2" = "env GOVERSION" ]; then printf "%s\\n" "'+version+'"; exit 0; fi\nexit 43\n'))
            go.chmod(0o755)
            env = dict(os.environ, PATH=str(commands)+os.pathsep+os.environ['PATH'])
            env.pop('SCYLLASCOPE_RACE_TESTS', None)
            if extra: env.update(extra)
            result = subprocess.run(['bash', str(root / 'build.sh')], env=env,
                                    capture_output=True, text=True, timeout=10)
            self.assertNotEqual(result.returncode, 0, result.stdout)
            self.assertFalse(marker.exists(), 'stale verified marker survived a failed attempt')
            self.assertEqual(binary.read_bytes(), original, 'previous binary was damaged')
            self.assertFalse(list(root.glob('.scyllascope-build.*')), 'temporary artifact leaked')
            return result

    def test_old_go_invalidates_marker(self):
        result = self.run_case()
        self.assertIn('require Go >=1.25.0', result.stderr)

    def test_failed_version_query_invalidates_marker(self):
        self.run_case(query_fails=True)

    def test_malformed_version_invalidates_marker(self):
        self.run_case(version='not-a-go-version')

    def test_race_opt_out_cannot_generate_verified_build(self):
        result = self.run_case(version='go1.27.1', extra={'SCYLLASCOPE_RACE_TESTS':'0'})
        self.assertIn('Race checks cannot be skipped', result.stderr)

    def test_unknown_race_setting_fails_closed(self):
        result = self.run_case(version='go1.27.1', extra={'SCYLLASCOPE_RACE_TESTS':'typo'})
        self.assertIn('Race checks cannot be skipped', result.stderr)

if __name__ == '__main__':
    unittest.main(verbosity=2)

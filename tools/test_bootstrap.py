#!/usr/bin/env python3
"""Isolated bootstrap transport/dispatch regressions; no real network or sudo.

Only the installed_binary constant in a private script copy is replaced. PATH
fakes serve official URL fixtures, reject all other destinations, and permit
privileged dispatch only to executables beneath this test's private directory.
"""
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
TAG = 'v0.6.3'
VERSION = TAG[1:]
BASE = 'https://github.com/disnana/ScyllaScope/releases'
RAW = 'https://raw.githubusercontent.com/disnana/ScyllaScope/main/install.sh'
COMPLETE = '__SCYLLASCOPE_INSTALLER_COMPLETE_V1__'

MOCK = r'''import json, os, pathlib, subprocess, sys
root=pathlib.Path(os.environ['BOOTSTRAP_TEST_ROOT']).resolve()
name=pathlib.Path(sys.argv[0]).name
args=sys.argv[1:]
def record(file, value):
 with (root/file).open('a') as out: out.write(json.dumps(value)+'\n')
if name=='id':
 if args==['-u']: print('1000'); sys.exit(0)
 sys.exit(93)
if name=='uname':
 if args==['-m']: print(os.environ.get('BOOTSTRAP_CPU','x86_64')); sys.exit(0)
 if args in ([],['-s']): print(os.environ.get('BOOTSTRAP_OS','Linux')); sys.exit(0)
 sys.exit(93)
if name=='sudo':
 record('sudo.jsonl',args)
 while args and args[0]=='--': args=args[1:]
 if not args: sys.exit(94)
 command=pathlib.Path(args[0]).resolve()
 if not command.is_relative_to(root): sys.exit(95)
 sys.exit(subprocess.run(args,env=os.environ).returncode)
if name=='curl':
 urls=[x for x in args if x.startswith(('https://','http://'))]
 if len(urls)!=1: record('curl.jsonl',{'invalid_args':args}); sys.exit(96)
 url=urls[0];record('curl.jsonl',url)
 config=json.loads((root/'transport.json').read_text())
 destination=None
 for i,arg in enumerate(args):
  if arg in ('--output','-o'): destination=args[i+1]
 if url==config['raw_url']:
  data=pathlib.Path(config['source']).read_bytes()
  if config.get('raw_mode')=='truncated_fail':
   data=data[:data.rfind(b'\nss_main ')]
  code=22 if config.get('raw_mode') in ('truncated_fail','complete_fail') else 0
 elif url in config['responses']:
  data=pathlib.Path(config['responses'][url]).read_bytes()
  code=22 if url==config.get('fail_url') else 0
 else:
  record('unexpected-url.jsonl',url);sys.exit(97)
 if destination is None: sys.stdout.buffer.write(data);sys.stdout.buffer.flush()
 else:
  target=pathlib.Path(destination).resolve()
  if not target.is_relative_to(root): sys.exit(95)
  target.write_bytes(data)
 sys.exit(code)
sys.exit(98)
'''


class BootstrapTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='scyllascope-bootstrap-test-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root/'bin'
        self.bin.mkdir()
        self.work = self.root/'work'
        self.work.mkdir()
        self.installed = self.root/'installed'/'scyllascope'
        self.installed.parent.mkdir()
        self.script = self.root/'install.sh'
        original = (ROOT/'install.sh').read_text()
        pattern = r'''installed_binary=(['"])/usr/local/bin/scyllascope\1'''
        self.assertEqual(len(re.findall(pattern, original)), 1,
                         'bootstrap must expose exactly one fixed installed_binary constant')
        self.source = re.sub(pattern, 'installed_binary='+shlex.quote(str(self.installed)), original)
        self.script.write_text(self.source)
        for tool in ['curl', 'sudo', 'id', 'uname']:
            path = self.bin/tool
            path.write_text('#!'+sys.executable+'\n'+MOCK)
            path.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.bin)+os.pathsep+os.environ['PATH'],
                        BOOTSTRAP_TEST_ROOT=str(self.root), TMPDIR=str(self.work))
        self.config = {'raw_url': RAW, 'source': str(self.script), 'responses': {}}
        self.publish()

    def payload(self, version=VERSION):
        return ('#!'+sys.executable+'\n'+r'''import json, os, pathlib, sys
root=pathlib.Path(os.environ['BOOTSTRAP_TEST_ROOT'])
args=sys.argv[1:]
with (root/'payload.jsonl').open('a') as out: out.write(json.dumps(args)+'\n')
if args==['version']:
 print('ScyllaScope '+VERSION_VALUE)
 sys.exit(int(os.environ.get('BOOTSTRAP_VERSION_EXIT','0')))
if args in (['service','install'],['service','uninstall']):
 sys.exit(int(os.environ.get('BOOTSTRAP_BACKEND_EXIT','0')))
sys.exit(99)
'''.replace('VERSION_VALUE', repr(version))).encode()

    def archive(self, *, binary_version=VERSION, metadata_version=VERSION,
                omit=None, duplicate=None, unsafe=None, extra=None):
        entries = [('scyllascope', self.payload(binary_version)),
                   ('VERSION', (metadata_version+'\n').encode()),
                   ('LICENSE', b'Synthetic MIT license fixture\n'),
                   ('THIRD_PARTY.md', b'Synthetic third-party notices\n'),
                   ('LICENSES/RC-MIT.txt', b'Synthetic RC MIT notice\n')]
        if extra is not None:
            entries.append(extra)
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode='w:gz') as archive:
            for name, data in entries:
                if name == omit:
                    continue
                item = tarfile.TarInfo(name)
                item.mode = 0o755 if name == 'scyllascope' else 0o644
                if unsafe and name == unsafe[0]:
                    item.type = unsafe[1]
                    item.linkname = unsafe[2]
                    archive.addfile(item)
                else:
                    item.size = len(data)
                    archive.addfile(item, io.BytesIO(data))
                if name == duplicate:
                    second = tarfile.TarInfo(name)
                    second.mode, second.size = item.mode, len(data)
                    archive.addfile(second, io.BytesIO(data))
        return output.getvalue()

    def publish(self, *, archive=None, manifest=None, latest_version=VERSION):
        if archive is None:
            archive = self.archive()
        digest = hashlib.sha256(archive).hexdigest()
        default = ''.join(f'{digest}  {self.asset(arch)}\n' for arch in ['amd64', 'arm64'])
        files = {'archive': archive, 'manifest': (manifest if manifest is not None else default).encode(),
                 'latest': (latest_version+'\n').encode()}
        for name, content in files.items():
            (self.root/name).write_bytes(content)
        self.config['responses'] = {
            BASE+'/latest/download/VERSION': str(self.root/'latest'),
            BASE+'/download/'+TAG+'/SHA256SUMS': str(self.root/'manifest'),
            **{BASE+'/download/'+TAG+'/'+self.asset(arch): str(self.root/'archive')
               for arch in ['amd64', 'arm64']}}
        self.save_config()

    @staticmethod
    def asset(arch='amd64'):
        return f'scyllascope-{TAG}-linux-{arch}.tar.gz'

    def save_config(self):
        (self.root/'transport.json').write_text(json.dumps(self.config))

    def calls(self, kind):
        path = self.root/(kind+'.jsonl')
        return [json.loads(x) for x in path.read_text().splitlines()] if path.exists() else []

    def invoke(self, *args, streamed=None, pipeline=False, **env):
        configuration = dict(self.env, **env)
        if pipeline:
            command = ['bash', '-o', 'pipefail', '-c',
                       'curl -fsSL '+shlex.quote(RAW)+' | bash -s -- '+
                       ' '.join(shlex.quote(x) for x in args)]
        elif streamed is not None:
            command = ['bash', '-s', '--', *args]
        else:
            command = ['bash', str(self.script), *args]
        return subprocess.run(command, input=streamed, text=True, capture_output=True,
                              cwd=self.work, env=configuration, timeout=20)

    def invoke_readme(self, index, action):
        # Execute the documentation itself, preserving its quoted pipeline.
        # Do not reconstruct a supposedly equivalent command in the test.
        lines = re.findall(r"(?m)^bash -o pipefail -c [^\n]+$",
                           (ROOT/'README.md').read_text())
        self.assertGreaterEqual(len(lines), 3, 'README must show all three commands')
        command = shlex.split(lines[index])
        self.assertEqual(command[:4], ['bash', '-o', 'pipefail', '-c'])
        self.assertEqual(len(command), 5)
        # Full successful retrieval must precede starting the consumer. The
        # command substitution preserves curl status; && prevents dispatch
        # even when a failed producer emitted an otherwise complete script.
        expected = ('script=$(curl -fsSL '+RAW+') && '+
                    r'printf "%s\n" "$script" | bash -s -- '+action)
        if action in ('install', 'update'):
            expected += ' --version '+TAG
        self.assertEqual(command[4], expected)
        return subprocess.run(command, text=True, capture_output=True, cwd=self.work,
                              env=self.env, timeout=20)

    def services(self):
        return [x for x in self.calls('payload') if x[:1] == ['service']]

    def assert_rejected_without_privilege(self, result, *, no_payload=False):
        self.assertNotEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertEqual(self.calls('sudo'), [])
        self.assertEqual(self.services(), [])
        if no_payload:
            self.assertEqual(self.calls('payload'), [])
        self.assertEqual(self.calls('unexpected-url'), [])

    def install_existing(self):
        self.installed.write_bytes(self.payload())
        self.installed.chmod(0o755)

    def test_readme_install_command_dispatches_verified_install(self):
        result = self.invoke_readme(0, 'install')
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertEqual(self.services(), [['service', 'install']])
        self.assertEqual(self.calls('curl')[0], RAW)
        self.assertIn(BASE+'/download/'+TAG+'/'+self.asset(), self.calls('curl'))
        self.assertNotIn(BASE+'/latest/download/VERSION', self.calls('curl'))
        self.assertEqual(self.calls('unexpected-url'), [])

    def test_readme_update_command_dispatches_existing_install(self):
        self.install_existing()
        result = self.invoke_readme(1, 'update')
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertEqual(self.services(), [['service', 'install']])
        self.assertEqual(self.calls('curl')[0], RAW)
        self.assertEqual(self.calls('unexpected-url'), [])

    def test_readme_uninstall_command_uses_installed_cli_without_release_fetch(self):
        self.install_existing()
        result = self.invoke_readme(2, 'uninstall')
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertEqual(self.services(), [['service', 'uninstall']])
        # Fetching the documented bootstrap script is expected; no release
        # pointer, checksum or archive is fetched for uninstall.
        self.assertEqual(self.calls('curl'), [RAW])

    def test_help_does_not_fetch_or_request_privilege(self):
        for args in [('--help',), ('install', '--help')]:
            with self.subTest(args=args):
                result = self.invoke(*args)
                self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
                self.assertIn('Usage:', result.stdout)
                self.assertEqual(self.calls('curl'), [])
                self.assertEqual(self.calls('sudo'), [])
                self.assertEqual(self.calls('payload'), [])

    def test_installed_symlink_is_rejected_before_fetch_or_dispatch(self):
        target = self.root/'untrusted-installed-target'
        target.write_bytes(self.payload())
        target.chmod(0o755)
        self.installed.symlink_to(target)
        for action in ['install', 'update', 'uninstall']:
            with self.subTest(action=action):
                self.assert_rejected_without_privilege(self.invoke(action), no_payload=True)
                self.assertEqual(self.calls('curl'), [])

    def test_archive_unexpected_member_is_rejected(self):
        self.publish(archive=self.archive(extra=('UNEXPECTED', b'synthetic extra member')))
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_archive_path_traversal_member_is_rejected(self):
        # The unsafe name escapes the archive root. If a faulty implementation
        # were to extract it under this test cwd/TMPDIR it would still land
        # inside our private test directory, never a real host path.
        self.publish(archive=self.archive(extra=('LICENSES/../../escaped', b'synthetic traversal')))
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)
        self.assertFalse((self.root/'escaped').exists())
        self.assertFalse((self.work/'escaped').exists())

    def assert_readme_download_failure_safe(self, index, action):
        for failure_mode in ['complete_fail', 'truncated_fail']:
            with self.subTest(action=action, failure_mode=failure_mode):
                self.config['raw_mode'] = failure_mode
                self.save_config()
                result = self.invoke_readme(index, action)
                self.assert_rejected_without_privilege(result, no_payload=True)
                self.assertTrue(self.calls('curl'))
                self.assertTrue(all(url == RAW for url in self.calls('curl')))

    def test_readme_install_download_failures_do_not_execute_payload(self):
        self.assert_readme_download_failure_safe(0, 'install')

    def test_readme_update_download_failures_do_not_execute_payload(self):
        self.install_existing()
        self.assert_readme_download_failure_safe(1, 'update')

    def test_readme_uninstall_download_failures_do_not_execute_payload(self):
        self.install_existing()
        self.assert_readme_download_failure_safe(2, 'uninstall')

    def test_install_fetches_latest_once_then_pins_official_urls(self):
        result = self.invoke('install')
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertEqual(self.services(), [['service', 'install']])
        self.assertEqual(len(self.calls('sudo')), 1)
        calls = self.calls('curl')
        self.assertEqual(calls.count(BASE+'/latest/download/VERSION'), 1)
        self.assertIn(BASE+'/download/'+TAG+'/SHA256SUMS', calls)
        self.assertIn(BASE+'/download/'+TAG+'/'+self.asset(), calls)
        self.assertEqual(self.calls('unexpected-url'), [])

    def test_explicit_version_is_pinned_and_skips_latest_pointer(self):
        self.publish(latest_version='0.6.99')
        result = self.invoke('install', '--version', TAG)
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertNotIn(BASE+'/latest/download/VERSION', self.calls('curl'))
        self.assertEqual(self.services(), [['service', 'install']])

    def test_update_reuses_backend_install(self):
        self.install_existing()
        result = self.invoke('update', '--version', TAG)
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertEqual(self.services(), [['service', 'install']])

    def test_uninstall_uses_installed_backend_retaining_state_without_download(self):
        self.install_existing()
        result = self.invoke('uninstall')
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertEqual(self.calls('curl'), [])
        self.assertEqual(self.services(), [['service', 'uninstall']])
        self.assertEqual(self.calls('sudo'), [['--', str(self.installed), 'service', 'uninstall']])

    def test_arm64_selects_official_arm64_asset(self):
        result = self.invoke('install', '--version', TAG, BOOTSTRAP_CPU='aarch64')
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertIn(BASE+'/download/'+TAG+'/'+self.asset('arm64'), self.calls('curl'))
        self.assertNotIn(BASE+'/download/'+TAG+'/'+self.asset('amd64'), self.calls('curl'))

    def test_update_requires_existing_install(self):
        self.assert_rejected_without_privilege(self.invoke('update'))

    def test_uninstall_requires_existing_install(self):
        self.assert_rejected_without_privilege(self.invoke('uninstall'))
        self.assertEqual(self.calls('curl'), [])

    def test_unsupported_os(self):
        self.assert_rejected_without_privilege(self.invoke('install', BOOTSTRAP_OS='Darwin'))

    def test_unsupported_cpu(self):
        self.assert_rejected_without_privilege(self.invoke('install', BOOTSTRAP_CPU='riscv64'))

    def test_bad_action_and_extra_backend_arguments_are_rejected(self):
        self.assert_rejected_without_privilege(self.invoke('uninstall', '--purge'))
        self.assert_rejected_without_privilege(self.invoke('repair'))

    def test_invalid_version_is_rejected(self):
        self.assert_rejected_without_privilege(self.invoke('install', '--version', '../../invalid'))

    def test_missing_version_argument_is_rejected(self):
        self.assert_rejected_without_privilege(self.invoke('install', '--version'))

    def test_bad_latest_version_is_rejected(self):
        self.publish(latest_version='../../invalid')
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_each_fetch_failure_prevents_privileged_dispatch(self):
        for url in [BASE+'/latest/download/VERSION', BASE+'/download/'+TAG+'/SHA256SUMS',
                    BASE+'/download/'+TAG+'/'+self.asset()]:
            with self.subTest(url=url):
                self.config['fail_url'] = url
                self.save_config()
                self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_checksum_mismatch_never_executes_downloaded_payload(self):
        self.publish()
        with (self.root/'archive').open('ab') as out:
            out.write(b'corrupt after manifest creation')
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_manifest_missing_selected_asset(self):
        digest = hashlib.sha256(self.archive()).hexdigest()
        self.publish(manifest=digest+'  '+self.asset('arm64')+'\n')
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_manifest_duplicate_selected_asset(self):
        digest = hashlib.sha256(self.archive()).hexdigest()
        line = digest+'  '+self.asset()+'\n'
        self.publish(manifest=line+line)
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_manifest_malformed_checksum(self):
        self.publish(manifest='not-a-sha256  '+self.asset()+'\n')
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_malformed_archive_with_matching_checksum(self):
        self.publish(archive=b'not a gzip archive')
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_truncated_archive_with_matching_checksum(self):
        archive = self.archive()
        self.publish(archive=archive[:len(archive)//2])
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_archive_missing_required_member(self):
        self.publish(archive=self.archive(omit='THIRD_PARTY.md'))
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_archive_duplicate_member(self):
        self.publish(archive=self.archive(duplicate='scyllascope'))
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_archive_unsafe_symlink(self):
        target = self.root/'link-target'
        target.write_bytes(self.payload())
        target.chmod(0o755)
        self.publish(archive=self.archive(unsafe=('scyllascope', tarfile.SYMTYPE, str(target))))
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_archive_unsafe_hardlink(self):
        self.publish(archive=self.archive(unsafe=('scyllascope', tarfile.LNKTYPE, 'VERSION')))
        self.assert_rejected_without_privilege(self.invoke('install'), no_payload=True)

    def test_archive_version_mismatch(self):
        self.publish(archive=self.archive(metadata_version='0.6.3-rc.9'))
        self.assert_rejected_without_privilege(self.invoke('install'))

    def test_binary_version_mismatch(self):
        self.publish(archive=self.archive(binary_version='0.6.3-rc.9'))
        self.assert_rejected_without_privilege(self.invoke('install'))

    def test_binary_version_command_failure(self):
        result = self.invoke('install', BOOTSTRAP_VERSION_EXIT='49')
        self.assert_rejected_without_privilege(result)

    def test_backend_failure_is_propagated(self):
        result = self.invoke('install', BOOTSTRAP_BACKEND_EXIT='61')
        self.assertEqual(result.returncode, 61, result.stdout+result.stderr)
        self.assertEqual(self.services(), [['service', 'install']])

    def test_update_backend_failure_is_propagated(self):
        self.install_existing()
        result = self.invoke('update', BOOTSTRAP_BACKEND_EXIT='61')
        self.assertEqual(result.returncode, 61, result.stdout+result.stderr)
        self.assertEqual(self.services(), [['service', 'install']])

    def test_uninstall_backend_failure_is_propagated_without_fetch(self):
        self.install_existing()
        result = self.invoke('uninstall', BOOTSTRAP_BACKEND_EXIT='61')
        self.assertEqual(result.returncode, 61, result.stdout+result.stderr)
        self.assertEqual(self.calls('curl'), [])
        self.assertEqual(self.services(), [['service', 'uninstall']])

    def test_truncated_stream_never_reaches_main(self):
        marker = '\nss_main '
        self.assertIn(marker, self.source)
        prefix = self.source[:self.source.rfind(marker)]
        result = self.invoke('install', streamed=prefix)
        self.assertEqual(self.services(), [], result.stdout+result.stderr)
        self.assertEqual(self.calls('sudo'), [])
        self.assertEqual(self.calls('curl'), [])
        # Incomplete trailing invocation must not bypass the completion gate.
        partial = self.source[:self.source.rfind(COMPLETE)+len(COMPLETE)//2]
        self.invoke('install', streamed=partial)
        self.assertEqual(self.calls('sudo'), [])
        self.assertEqual(self.services(), [])

    def test_truncated_curl_pipeline_fails_closed(self):
        self.config['raw_mode'] = 'truncated_fail'
        self.save_config()
        result = self.invoke('install', pipeline=True)
        self.assertNotEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertEqual(self.calls('sudo'), [])
        self.assertEqual(self.services(), [])

    def test_pipefail_preserves_upstream_failure_after_complete_script(self):
        self.config['raw_mode'] = 'complete_fail'
        self.save_config()
        result = self.invoke('install', pipeline=True)
        # pipefail preserves curl's failure even if the complete consumer
        # script already ran. It cannot roll back an already-dispatched action.
        self.assertNotEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertEqual(self.calls('unexpected-url'), [])


if __name__ == '__main__':
    unittest.main(verbosity=2)

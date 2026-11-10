"""Offline installer tests; network commands are replaced with local fixtures."""
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest

INSTALLER = Path(__file__).resolve().parents[2] / 'install.sh'
VERSION = '0.1.2-dev.1'


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='fql-installer-test-')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.home = self.root / 'home'
        self.home.mkdir()
        self.assets = self.root / 'assets'
        self.assets.mkdir()
        self.fake_bin = self.root / 'commands'
        self.fake_bin.mkdir()
        self.destination = self.home / '.local/bin'
        self.environment = dict(os.environ, HOME=str(self.home), ZDOTDIR=str(self.home),
                                SHELL='/bin/zsh', PATH=str(self.fake_bin) + os.pathsep + os.environ['PATH'],
                                FQL_VERSION=VERSION, FQL_TEST_ASSETS=str(self.assets))
        for key in ['FQL_INSTALL_DIR', 'FQL_MODIFY_PATH']:
            self.environment.pop(key, None)
        self.asset = f'fql_{VERSION}_darwin_arm64.tar.gz'
        self.archive(self.asset)
        self.manifest()
        self.executable('uname', '#!/bin/sh\ncase "$1" in -s) printf "%s\\n" "${FQL_TEST_OS:-Darwin}";; -m) printf "%s\\n" "${FQL_TEST_ARCH:-arm64}";; esac\n')
        self.executable('curl', '#!' + sys.executable + '\n' + '''import json,os,pathlib,shutil,sys
args=sys.argv[1:]
out=pathlib.Path(args[args.index('--output')+1]);url=args[-1]
if 'api.github.com' in url:
 if url.endswith('/latest') and os.environ.get('FQL_TEST_NO_STABLE'): sys.exit(22)
 out.write_text(json.dumps({'tag_name':'v0.1.2-dev.1'}));sys.exit(0)
source=pathlib.Path(os.environ['FQL_TEST_ASSETS'])/url.rsplit('/',1)[-1]
if not source.exists(): sys.exit(22)
shutil.copyfile(source,out)
''')

    def executable(self, name, content):
        path = self.fake_bin / name
        path.write_text(content)
        path.chmod(0o755)

    def archive(self, name, member='fql', symlink=False, binary=None):
        with tarfile.open(self.assets / name, 'w:gz') as archive:
            entry = tarfile.TarInfo(member)
            entry.mode = 0o755
            if symlink:
                entry.type = tarfile.SYMTYPE
                entry.linkname = '/bin/sh'
                archive.addfile(entry)
            else:
                data = binary or f'#!/bin/sh\nprintf "fql {VERSION}\\n"\n'.encode()
                entry.size = len(data)
                archive.addfile(entry, io.BytesIO(data))

    def manifest(self):
        lines = [hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + p.name
                 for p in sorted(self.assets.glob('*.tar.gz'))]
        (self.assets / 'SHA256SUMS').write_text('\n'.join(lines) + '\n')

    def run_installer(self, *arguments, success=True):
        result = subprocess.run(['/bin/sh', str(INSTALLER), *arguments], env=self.environment,
                                capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
        return result

    def test_install_update_and_path_are_idempotent(self):
        self.run_installer()
        binary = self.destination / 'fql'
        self.assertEqual(subprocess.check_output([str(binary), 'version'], text=True).strip(), f'fql {VERSION}')
        profile = self.home / '.zshrc'
        first = profile.read_text()
        self.run_installer()
        self.assertEqual(first, profile.read_text())
        self.assertEqual(list(self.destination.glob('.fql.*')), [])

    def test_latest_falls_back_to_prerelease(self):
        self.environment.pop('FQL_VERSION')
        self.environment['FQL_TEST_NO_STABLE'] = '1'
        self.run_installer('--no-modify-path')
        self.assertTrue((self.destination / 'fql').exists())
        self.assertFalse((self.home / '.zshrc').exists())

    def test_checksum_failure_preserves_existing_binary(self):
        self.destination.mkdir(parents=True)
        binary = self.destination / 'fql'
        binary.write_text('existing installation')
        (self.assets / self.asset).write_bytes(b'tampered archive')
        self.run_installer(success=False)
        self.assertEqual(binary.read_text(), 'existing installation')

    def test_platforms_and_cpu_detection(self):
        for system, operating_system in [('Darwin', 'darwin'), ('Linux', 'linux')]:
            for machine, architecture in [('x86_64', 'amd64'), ('aarch64', 'arm64')]:
                with self.subTest(system=system, machine=machine):
                    name = f'fql_{VERSION}_{operating_system}_{architecture}.tar.gz'
                    self.archive(name)
                    self.manifest()
                    self.environment.update(FQL_TEST_OS=system, FQL_TEST_ARCH=machine)
                    self.run_installer('--no-modify-path')
        self.environment['FQL_TEST_ARCH'] = 'unsupported'
        self.run_installer(success=False)

    def test_unsafe_archive_and_invalid_version_are_rejected(self):
        for member, symlink in [('../outside', False), ('fql', True)]:
            self.archive(self.asset, member=member, symlink=symlink)
            self.manifest()
            self.run_installer(success=False)
            self.assertFalse((self.destination / 'fql').exists())
        self.run_installer('--version', '../../unsafe', success=False)
        self.run_installer('--install-dir', 'relative', success=False)

    def test_path_with_shell_metacharacters_is_not_evaluated(self):
        destination = self.home / 'tools " quote $dollar `touch SHOULD_NOT_EXIST`'
        self.run_installer('--install-dir', str(destination))
        profile = self.home / '.zshrc'
        result = subprocess.check_output(['/bin/sh', '-c', '. "$HOME/.zshrc"; printf "%s" "$PATH"'], env=self.environment, text=True)
        self.assertTrue(result.startswith(str(destination) + ':'))
        self.assertFalse(Path('SHOULD_NOT_EXIST').exists())


if __name__ == '__main__':
    unittest.main()

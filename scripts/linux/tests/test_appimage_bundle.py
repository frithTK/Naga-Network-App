import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import threading
import time
import unittest

ROOT = Path(__file__).resolve().parents[3]
LIB = ROOT / 'packaging/linux/runtime-lib.sh'


class AppImageBundleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='naga-bundle-')
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.source = self.base / 'source with spaces'
        self.source.mkdir()
        for name in ['naga-control', 'sing-box', 'olcrtc', 'hev-socks5-tunnel', 'install-system.sh']:
            p = self.source / name
            p.write_text('#!/bin/sh\nexit 0\n')
            p.chmod(0o755)
        (self.source / 'runtime.version').write_text('1.2.5\n')
        (self.source / 'olcrtc.version').write_text('7f849e08\n')
        shutil.copy2(LIB, self.source / LIB.name)
        manifest = ''.join(f'{hashlib.sha256(p.read_bytes()).hexdigest()}  ./{p.name}\n'
                           for p in sorted(self.source.iterdir()))
        (self.source / 'runtime.sha256').write_text(manifest)
        self.build_id = hashlib.sha256(manifest.encode()).hexdigest()
        (self.source / 'runtime.build-id').write_text(self.build_id + '\n')
        self.cache = self.base / 'cache'
        self.bin = self.base / 'bin'
        self.data = self.base / 'data'
        self.mocks = self.base / 'mocks'
        self.mocks.mkdir()
        self.log = self.base / 'pkexec.log'
        self.env = dict(os.environ, PATH=str(self.mocks) + os.pathsep + os.environ['PATH'],
                        NAGA_TEST_DATA=str(self.data), NAGA_TEST_LOG=str(self.log))
        self.env.pop('PKEXEC_UID', None)
        self.mock('systemctl', 'exit 0')
        self.mock('curl', 'cat >/dev/null\nprintf \'{"service":"naga-control","version":"1.2.5"}\\n\'')
        self.mock('pkexec', '''printf 'called\n' >>"$NAGA_TEST_LOG"
if [ "${NAGA_TEST_DENY:-}" = 1 ]; then exit 126; fi
cp "$(dirname "$2")/runtime.build-id" "$NAGA_TEST_DATA/runtime.build-id"
''')

    def mock(self, name, body):
        p = self.mocks / name
        p.write_text('#!/bin/sh\nset -eu\n' + body + '\n')
        p.chmod(0o755)

    def stage(self):
        return subprocess.run(['bash', '-c', 'source "$1"; naga_stage_runtime "$2" "$3"',
                               'test', str(LIB), str(self.source), str(self.cache)],
                              env=self.env, text=True, capture_output=True, timeout=5)

    def prepare(self):
        return subprocess.run(['bash', '-c', 'source "$1"; shift; naga_prepare_runtime "$@"',
                               'test', str(LIB), str(self.source), str(self.cache),
                               str(self.bin), str(self.data), 'testuser'],
                              env=self.env, text=True, capture_output=True, timeout=5)

    def test_runtime_staging_is_private_and_reused(self):
        first = self.stage()
        self.assertEqual(first.returncode, 0, first.stderr)
        cached = self.cache / self.build_id
        self.assertEqual(Path(first.stdout.strip()), cached)
        self.assertEqual(self.cache.stat().st_mode & 0o777, 0o700)
        self.assertEqual((cached / 'sing-box').read_bytes(), (self.source / 'sing-box').read_bytes())
        before = (cached / 'naga-control').stat().st_ino
        self.assertEqual(self.stage().returncode, 0)
        self.assertEqual((cached / 'naga-control').stat().st_ino, before)

    def test_corrupt_source_or_cache_is_rejected(self):
        self.assertEqual(self.stage().returncode, 0)
        (self.cache / self.build_id / 'sing-box').write_text('changed')
        self.assertNotEqual(self.stage().returncode, 0)
        (self.source / 'naga-control').write_text('changed')
        self.assertNotEqual(self.prepare().returncode, 0)
        self.assertFalse(self.log.exists())

    def test_authorization_failure_restores_previous_control(self):
        self.bin.mkdir()
        previous = self.bin / 'naga-control'
        previous.write_text('old installation')
        self.env['NAGA_TEST_DENY'] = '1'
        result = self.prepare()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(previous.is_symlink())
        self.assertEqual(previous.read_text(), 'old installation')
        self.assertFalse(list(self.bin.glob('.naga-runtime-*')))

    def test_successful_prepare_reuses_active_bundle_without_second_prompt(self):
        first = self.prepare()
        self.assertEqual(first.returncode, 0, first.stderr)
        target = self.bin / 'naga-control'
        self.assertEqual(target.resolve(), self.cache / self.build_id / 'naga-control')
        self.assertEqual((self.data / 'control.token').stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.prepare().returncode, 0)
        self.assertEqual(self.log.read_text().splitlines(), ['called'])

    def test_entrypoint_version_and_update_wait(self):
        appdir = self.base / 'AppDir'
        appdir.mkdir()
        shutil.copytree(self.source, appdir / 'runtime')
        entry = appdir / 'AppRun'
        shutil.copy2(ROOT / 'packaging/linux/AppRun', entry)
        old = subprocess.Popen(['sleep', '0.3'])
        self.addCleanup(lambda: old.poll() is not None or old.kill())
        reaper = threading.Thread(target=old.wait)
        reaper.start()
        start = time.monotonic()
        result = subprocess.run(['bash', str(entry), '--wait-for-pid', str(old.pid), '--version'],
                                capture_output=True, text=True, timeout=5)
        reaper.join()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('Naga Network 1.2.5', result.stdout)
        self.assertGreaterEqual(time.monotonic() - start, 0.2)

    def test_privileged_installer_rejects_direct_launch(self):
        installer = self.source / 'install-system.sh'
        shutil.copy2(ROOT / 'packaging/linux/install-system.sh', installer)
        result = subprocess.run(['bash', str(installer)], env=self.env,
                                capture_output=True, text=True, timeout=5)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('pkexec', result.stderr)

    @unittest.skipUnless(shutil.which('systemd-analyze'), 'requires systemd unit parser')
    def test_rendered_service_is_accepted_by_systemd(self):
        template = ROOT / 'platform/linux/systemd/naga-network-control@.service'
        for name in ['home', 'home with spaces &pipe|']:
            with self.subTest(name=name):
                home = self.base / name
                binary = home / '.local/bin/naga-control'
                binary.parent.mkdir(parents=True)
                binary.write_text('#!/bin/sh\nexit 0\n')
                binary.chmod(0o755)
                (home / '.local/share/naga-network').mkdir(parents=True)
                unit = self.base / 'naga-network-control@.service'
                render = subprocess.run([
                    'bash', '-c', 'source "$1"; shift; naga_render_service "$@"',
                    'test', str(LIB), str(template), str(home), str(os.getgid()), str(unit),
                ], text=True, capture_output=True, timeout=5)
                self.assertEqual(render.returncode, 0, render.stderr)
                check = subprocess.run(['systemd-analyze', 'verify', str(unit)],
                                       text=True, capture_output=True, timeout=10)
                self.assertEqual(check.returncode, 0, check.stderr)


if __name__ == '__main__':
    unittest.main()

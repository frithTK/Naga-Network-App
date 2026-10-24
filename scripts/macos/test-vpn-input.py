#!/usr/bin/env python3
"""Exercise the production VPN frame reader with real pipes on Linux/macOS."""

import base64
import os
import select
import subprocess
import tempfile
import threading
import time
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
LIMIT = 8 * 1024 * 1024


class VPNInputTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.directory = tempfile.TemporaryDirectory(prefix='naga-vpn-input-')
        cls.addClassCleanup(cls.directory.cleanup)
        directory = Path(cls.directory.name)
        cls.binary = directory / 'vpn-input-probe'
        sources = ROOT / 'native/macos/vpn'
        subprocess.run(['swiftc', '-parse-as-library', '-swift-version', '5',
                        '-module-cache-path', str(directory / 'cache'),
                        str(sources / 'VPNProtocol.swift'), str(sources / 'VPNConfigInput.swift'),
                        str(sources / 'tests/VPNConfigInputProbe.swift'), '-o', str(cls.binary)],
                       check=True, timeout=120)

    def start_probe(self):
        process = subprocess.Popen([str(self.binary)], stdin=subprocess.PIPE,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE, bufsize=0)

        def cleanup():
            if process.poll() is None:
                process.kill()
            process.wait(timeout=5)
            for pipe in (process.stdin, process.stdout, process.stderr):
                pipe.close()

        self.addCleanup(cleanup)
        return process

    def read_line(self, process):
        # Large frames also produce output larger than a pipe's capacity.
        data = bytearray()
        deadline = time.monotonic() + 5
        while True:
            remaining = deadline - time.monotonic()
            self.assertGreater(remaining, 0, 'frame reader did not reply while stdin stayed open')
            self.assertTrue(select.select([process.stdout], [], [], remaining)[0],
                            'frame reader waited for more bytes or EOF after the newline')
            chunk = os.read(process.stdout.fileno(), 65536)
            self.assertTrue(chunk, 'probe exited before replying')
            data.extend(chunk)
            if data.endswith(b'\n'):
                return bytes(data[:-1])

    def send(self, process, data):
        # Concurrent writing bounds a regression that stops draining a large input.
        errors = []

        def write():
            try:
                view = memoryview(data)
                while view:
                    written = process.stdin.write(view)
                    view = view[written:]
            except Exception as error:
                errors.append(error)

        writer = threading.Thread(target=write, daemon=True)
        writer.start()
        writer.join(timeout=5)
        self.assertFalse(writer.is_alive(), 'frame reader stopped draining stdin')
        self.assertEqual(errors, [])

    def assert_frame(self, data):
        process = self.start_probe()
        self.send(process, data + b'\n')
        self.assertEqual(base64.b64decode(self.read_line(process), validate=True), data)
        self.assertIsNone(process.poll(), 'stdin must remain open after readiness')
        process.stdin.close()
        self.assertEqual(self.read_line(process), b'stopped')
        self.assertEqual(process.wait(timeout=5), 0)

    def test_short_frame_keeps_lifetime_pipe_open(self):
        self.assert_frame(b'{"test":true}')

    def test_buffer_boundaries_and_exact_size_limit(self):
        for size in (65535, 65536, 65537, 131079, LIMIT):
            with self.subTest(size=size):
                self.assert_frame(b'{' + b' ' * (size - 2) + b'}')

    def test_fragmented_unicode_and_separate_newline(self):
        process = self.start_probe()
        data = '{"name":"Профиль 🇨🇿", "escaped":"a\\nb"}'.encode()
        for byte in data:
            self.send(process, bytes([byte]))
        self.assertFalse(select.select([process.stdout], [], [], 0.1)[0],
                         'partial frame must wait for its newline')
        self.send(process, b'\n')
        self.assertEqual(base64.b64decode(self.read_line(process), validate=True), data)

    def test_oversized_frame_rejected_without_eof(self):
        process = self.start_probe()
        self.send(process, b'x' * (LIMIT + 1))
        self.assertEqual(self.read_line(process), b'error')
        self.assertEqual(process.wait(timeout=5), 0)

    def test_empty_and_truncated_frames_rejected(self):
        for data in (b'', b'{"incomplete":', b'\n'):
            with self.subTest(data=data):
                process = self.start_probe()
                self.send(process, data)
                process.stdin.close()
                self.assertEqual(self.read_line(process), b'error')
                self.assertEqual(process.wait(timeout=5), 0)


if __name__ == '__main__':
    unittest.main(verbosity=2)

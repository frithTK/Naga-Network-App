import importlib.util
import os
import plistlib
import shutil
import struct
import subprocess
import tempfile
import unittest
import zipfile
from pathlib import Path

ROOT=Path(__file__).resolve().parents[3]
def module(name,path):
    spec=importlib.util.spec_from_file_location(name,path)
    result=importlib.util.module_from_spec(spec);spec.loader.exec_module(result);return result
VERIFY=module('verify_ipa',ROOT/'scripts/ios/verify-ipa.py')
MACHO=module('check_macho',ROOT/'ios/packaging/ensure-macho-text.py')

class PackagingTests(unittest.TestCase):
    def test_failed_build_cannot_publish_stale_app(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            shutil.copytree(ROOT/'scripts/ios',root/'scripts/ios')
            (root/'app').mkdir();(root/'app/pubspec.yaml').write_text('version: 1.2.6+11\n')
            (root/'ios/xtool/NagaNetwork.app').mkdir(parents=True)
            (root/'ios/xtool/NagaNetwork.app/stale').write_text('old')
            (root/'ios/Info.plist').write_bytes(plistlib.dumps({'CFBundleShortVersionString':'1.2.5','CFBundleVersion':'10'}))
            stub=root/'xtool-stub';stub.write_text('#!/bin/sh\necho "Swift compile failed"\nexit 9\n');stub.chmod(0o700)
            env=root/'scripts/ios/xtool-env.sh';env.write_text(f'naga_require_xtool() {{ printf "%s\\n" "{stub}"; }}\nnaga_require_clean_linker() {{ :; }}\n')
            result=subprocess.run(['bash',str(root/'scripts/ios/build-ipa.sh')],capture_output=True,text=True,timeout=10)
            self.assertNotEqual(result.returncode,0)
            self.assertFalse((root/'dist/NagaNetwork-1.2.6-ios.ipa').exists())
            self.assertEqual((root/'ios/xtool/NagaNetwork.app/stale').read_text(),'old')
    def test_embedded_version_and_output_provenance(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);work=root/'work';work.mkdir();ipa=work/'result.ipa'
            def write(version):
                with zipfile.ZipFile(ipa,'w') as z:
                    z.writestr('Payload/NagaNetwork.app/Info.plist',plistlib.dumps({'CFBundleShortVersionString':version,'CFBundleVersion':'11'}))
            (work/'build.log').write_text('Wrote to result.ipa\n')
            write('1.2.5')
            with self.assertRaises(ValueError):VERIFY.publish(work,'1.2.6+11',root/'dist')
            write('1.2.6');VERIFY.publish(work,'1.2.6+11',root/'dist')
            self.assertTrue((root/'dist/NagaNetwork-1.2.6-ios.ipa').exists())
            outside=root/'outside.ipa';shutil.copy2(ipa,outside)
            (work/'build.log').write_text(f'Wrote to {outside}\n')
            with self.assertRaises(ValueError):VERIFY.publish(work,'1.2.6+11',root/'dist')
    def test_macho_never_modifies_data_or_malformed_headers(self):
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'fixture'
            # __TEXT contains data at the old patcher's dummy-code offset.
            data=bytearray(0x2000);struct.pack_into('<I',data,0,0xFEEDFACF)
            struct.pack_into('<II',data,16,1,152)
            struct.pack_into('<II',data,32,0x19,152);data[40:56]=b'__TEXT'.ljust(16,b'\0')
            struct.pack_into('<QQQQ',data,56,0x100000000,0x2000,0,0x2000)
            struct.pack_into('<I',data,96,1)
            data[104:120]=b'__const'.ljust(16,b'\0');data[120:136]=b'__TEXT'.ljust(16,b'\0')
            struct.pack_into('<QQI',data,136,0x100001000,16,0x1000)
            data[0x1000:0x1010]=b'KEEP-PRIVATE-DATA';path.write_bytes(data)
            with self.assertRaises(ValueError):MACHO.ensure_text(path)
            self.assertEqual(path.read_bytes(),bytes(data))
            data[104:120]=b'__text'.ljust(16,b'\0');path.write_bytes(data)
            self.assertFalse(MACHO.ensure_text(path));self.assertEqual(path.read_bytes(),bytes(data))
            for raw in [b'',bytes(data[:100]),struct.pack('<I',0xFEEDFACF)+bytes(28)]:
                path.write_bytes(raw)
                with self.assertRaises(ValueError):MACHO.ensure_text(path)
                self.assertEqual(path.read_bytes(),raw)

if __name__=='__main__':unittest.main()

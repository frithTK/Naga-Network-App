import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from bundle_paths import parse_rpaths, resolve_rpath


class BundlePathTests(unittest.TestCase):
    def test_otool_paths_preserve_spaces_and_ignore_other_commands(self):
        commands = '''
Load command 0
          cmd LC_RPATH
      cmdsize 56
         path @executable_path/../Frameworks With Spaces (offset 12)
Load command 1
          cmd LC_LOAD_DYLIB
         name @rpath/Example.framework/Example (offset 24)
Load command 2
          cmd LC_RPATH
         path /usr/local/lib/. (offset 12)
'''
        self.assertEqual(parse_rpaths(commands), [
            '@executable_path/../Frameworks With Spaces', '/usr/local/lib/.'])

    def test_actual_framework_paths_reject_external_missing_and_symlink_escape(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            app = root / 'Naga Network.app'
            executable = app / 'Contents/MacOS/Naga Network'
            framework = app / 'Contents/Frameworks/FlutterMacOS.framework/Versions/A/FlutterMacOS'
            executable.parent.mkdir(parents=True)
            framework.parent.mkdir(parents=True)
            (app / 'Contents/Frameworks/escape').symlink_to(root, target_is_directory=True)
            for value in ('/usr/local/lib/.', '/opt/homebrew/lib', 'Frameworks',
                          '@loader_path/Frameworks', '@executable_path/Frameworks',
                          '@loader_path/../../../../../..', '@executable_path/../Frameworks/escape'):
                self.assertIsNone(resolve_rpath(value, framework, executable, app), value)
            self.assertEqual(resolve_rpath('@loader_path/../../..', framework, executable, app),
                             app / 'Contents/Frameworks')
            self.assertEqual(resolve_rpath('@executable_path/../Frameworks', framework, executable, app),
                             app / 'Contents/Frameworks')
            self.assertEqual(resolve_rpath('/usr/lib/swift', framework, executable, app), Path('/usr/lib/swift'))


if __name__ == '__main__':
    unittest.main()

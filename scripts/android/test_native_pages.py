import importlib.util
import struct
import unittest
from pathlib import Path
spec=importlib.util.spec_from_file_location('verify_native_pages',Path(__file__).with_name('verify-native-pages.py'))
verify=importlib.util.module_from_spec(spec);spec.loader.exec_module(verify)

class NativePagesTests(unittest.TestCase):
    def fixture(self,alignments):
        data=bytearray(64+56*len(alignments));data[:6]=b'\x7fELF\x02\x01'
        struct.pack_into('<Q',data,32,64);struct.pack_into('<HH',data,54,56,len(alignments))
        for index,alignment in enumerate(alignments):
            offset=64+56*index;struct.pack_into('<I',data,offset,1);struct.pack_into('<Q',data,offset+48,alignment)
        return data
    def test_every_load_segment_must_be_aligned(self):
        verify.verify_elf(self.fixture([16384,65536]))
        for data in [self.fixture([16384,4096]),self.fixture([4096,16384]),b'',self.fixture([])]:
            with self.assertRaises(ValueError):verify.verify_elf(data)
    def test_offsets_must_be_congruent(self):
        data=self.fixture([16384]);struct.pack_into('<Q',data,64+8,4096)
        with self.assertRaises(ValueError):verify.verify_elf(data)
    def test_relro_padding_does_not_change_load_alignment(self):
        # A valid 16 KiB PT_LOAD can include 4 KiB RELRO padding. The next
        # writable LOAD is on another 16 KiB page; RELRO end alone is not
        # the Android ELF alignment criterion.
        data=self.fixture([16384,16384,1])
        struct.pack_into('<Q',data,64+16,0x47d50)
        struct.pack_into('<Q',data,64+8,0x3fd50)
        struct.pack_into('<Q',data,64+56+16,0x4c540)
        struct.pack_into('<Q',data,64+56+8,0x40540)
        struct.pack_into('<I',data,64+112,0x6474E552)
        struct.pack_into('<Q',data,64+112+16,0x47d50)
        struct.pack_into('<Q',data,64+112+40,0x12b0)
        verify.verify_elf(data)

if __name__=='__main__':unittest.main()

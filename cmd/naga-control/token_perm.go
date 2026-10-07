package main

import (
	"io/fs"
	"runtime"
)

func tokenFileWorldReadable(info fs.FileInfo) bool {
	if runtime.GOOS == "windows" {
		// NTFS ACL is not expressed in unix permission bits; os.Stat typically
		// reports 0666. Rejecting those bits would make every Windows token unusable.
		return false
	}
	return info.Mode().Perm()&0o077 != 0
}

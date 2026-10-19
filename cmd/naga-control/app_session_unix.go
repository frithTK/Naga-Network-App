//go:build darwin || linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func lockAppSession(root string) (func(), error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("application data directory must be a real directory")
	}
	dirFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(dirFD)
	var dirStat unix.Stat_t
	if err := unix.Fstat(dirFD, &dirStat); err != nil || dirStat.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("application data directory must belong to the current user")
	}
	if err := unix.Fchmod(dirFD, 0o700); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(dirFD, "backend.lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, err
	}
	var lockStat unix.Stat_t
	if err := unix.Fstat(fd, &lockStat); err != nil || lockStat.Uid != uint32(os.Geteuid()) || lockStat.Mode&unix.S_IFMT != unix.S_IFREG {
		unix.Close(fd)
		return nil, fmt.Errorf("invalid application backend lock file")
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("another application backend is already using this data directory: %w", err)
	}
	return func() { _ = unix.Close(fd) }, nil
}

func validateSessionToken(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 4096 {
		return fmt.Errorf("control token must be a private regular file: %s", filepath.Base(path))
	}
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil || stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("control token must belong to the current user")
	}
	return nil
}

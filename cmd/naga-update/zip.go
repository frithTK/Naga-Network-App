package main

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

type zipLimits struct {
	files                 int
	fileBytes, totalBytes uint64
}

var portableZipLimits = zipLimits{files: 10000, fileBytes: 512 << 20, totalBytes: 2 << 30}

type zipEntry struct {
	name string
	file *zip.File
}

func extractZip(zipPath, dest string) error {
	return extractZipLimited(zipPath, dest, portableZipLimits)
}

func extractZipLimited(zipPath, dest string, limits zipLimits) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("открыть zip: %w", err)
	}
	defer reader.Close()
	if len(reader.File) > limits.files {
		return errors.New("слишком много файлов в обновлении")
	}
	entries := make([]zipEntry, 0, len(reader.File))
	seen := map[string]bool{}
	var total uint64
	for _, file := range reader.File {
		name, err := portableZipName(file.Name)
		if err != nil {
			return err
		}
		if seen[strings.ToLower(name)] {
			return fmt.Errorf("повторный путь в zip: %s", file.Name)
		}
		seen[strings.ToLower(name)] = true
		if !file.FileInfo().IsDir() && !file.Mode().IsRegular() {
			return fmt.Errorf("ссылка или специальный файл в zip: %s", file.Name)
		}
		if file.UncompressedSize64 > limits.fileBytes || file.UncompressedSize64 > limits.totalBytes-total {
			return errors.New("распакованное обновление превышает допустимый размер")
		}
		total += file.UncompressedSize64
		entries = append(entries, zipEntry{name: name, file: file})
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer root.Close()
	stage, err := zipPrivateDir(root)
	if err != nil {
		return err
	}
	defer root.RemoveAll(stage)
	staged, err := root.OpenRoot(stage)
	if err != nil {
		return err
	}
	defer staged.Close()
	for _, entry := range entries {
		if err := stageZipEntry(staged, entry, limits); err != nil {
			return err
		}
		if err := cleanZipDestination(root, entry.name, entry.file.FileInfo().IsDir()); err != nil {
			return err
		}
	}
	backup, err := zipPrivateDir(root)
	if err != nil {
		return err
	}
	if err := applyZipEntries(root, stage, backup, entries, root.Rename); err != nil {
		return err
	}
	// Windows may keep the old updater executable open until this process exits.
	_ = root.RemoveAll(backup)
	return nil
}

func portableZipName(raw string) (string, error) {
	name := strings.TrimSuffix(strings.ReplaceAll(raw, "\\", "/"), "/")
	bad := func() (string, error) { return "", fmt.Errorf("небезопасный путь в zip: %s", raw) }
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, ":\x00<>\"|?*") {
		return bad()
	}
	for _, r := range name {
		if r < 32 {
			return bad()
		}
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, ". ") != part {
			return bad()
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		runes := []rune(base)
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
			len(runes) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && strings.ContainsRune("123456789¹²³", runes[3]) {
			return bad()
		}
	}
	if strings.HasPrefix(strings.ToLower(strings.Split(name, "/")[0]), ".naga-update-") {
		return bad()
	}
	return name, nil
}

func zipPrivateDir(root *os.Root) (string, error) {
	for range 8 {
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			return "", err
		}
		name := ".naga-update-" + hex.EncodeToString(token[:])
		if err := root.Mkdir(name, 0o700); err == nil {
			return name, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", errors.New("не удалось создать staging обновления")
}

func stageZipEntry(root *os.Root, entry zipEntry, limits zipLimits) error {
	if entry.file.FileInfo().IsDir() {
		return root.MkdirAll(entry.name, 0o755)
	}
	if err := root.MkdirAll(path.Dir(entry.name), 0o755); err != nil {
		return err
	}
	source, err := entry.file.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	mode := entry.file.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	out, err := root.OpenFile(entry.name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	written, err := io.Copy(out, io.LimitReader(source, int64(limits.fileBytes)+1))
	if err != nil {
		return err
	}
	if uint64(written) > limits.fileBytes || uint64(written) != entry.file.UncompressedSize64 {
		return errors.New("неверный размер файла в zip")
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return out.Close()
}

func cleanZipDestination(root *os.Root, name string, directory bool) error {
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		wantDir := i < len(parts)-1 || directory
		if info.Mode()&os.ModeSymlink != 0 || wantDir && !info.IsDir() || !wantDir && !info.Mode().IsRegular() {
			return fmt.Errorf("небезопасное назначение обновления: %s", name)
		}
	}
	return nil
}

func applyZipEntries(root *os.Root, stage, backup string, entries []zipEntry, rename func(string, string) error) error {
	type change struct {
		name   string
		hadOld bool
	}
	var changed []change
	rollback := func(cause error) error {
		var restoreErr error
		for i := len(changed) - 1; i >= 0; i-- {
			item := changed[i]
			var err error
			if item.hadOld {
				err = rename(path.Join(backup, item.name), item.name)
			} else {
				err = root.Remove(item.name)
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				restoreErr = errors.Join(restoreErr, err)
			}
		}
		if restoreErr != nil {
			return errors.Join(cause, fmt.Errorf("не удалось восстановить обновление; сохранена копия %s: %w", backup, restoreErr))
		}
		_ = root.RemoveAll(backup)
		return cause
	}
	for _, entry := range entries {
		if err := cleanZipDestination(root, entry.name, entry.file.FileInfo().IsDir()); err != nil {
			return rollback(err)
		}
		if entry.file.FileInfo().IsDir() {
			if err := root.MkdirAll(entry.name, 0o755); err != nil {
				return rollback(err)
			}
			continue
		}
		if err := root.MkdirAll(path.Dir(entry.name), 0o755); err != nil {
			return rollback(err)
		}
		_, err := root.Lstat(entry.name)
		hadOld := err == nil
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return rollback(err)
		}
		if hadOld {
			if err := root.MkdirAll(path.Join(backup, path.Dir(entry.name)), 0o700); err != nil {
				return rollback(err)
			}
			if err := rename(entry.name, path.Join(backup, entry.name)); err != nil {
				return rollback(err)
			}
		}
		changed = append(changed, change{entry.name, hadOld})
		if err := rename(path.Join(stage, entry.name), entry.name); err != nil {
			return rollback(err)
		}
	}
	return nil
}

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractZip(zipPath, dest string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("открыть zip: %w", err)
	}
	defer reader.Close()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	for _, file := range reader.File {
		if err := extractZipFile(file, destAbs); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(file *zip.File, destAbs string) error {
	name := filepath.Clean(file.Name)
	if name == "." || name == ".." {
		return nil
	}
	target := filepath.Join(destAbs, name)
	rel, err := filepath.Rel(destAbs, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("небезопасный путь в zip: %s", file.Name)
	}
	if file.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	mode := file.Mode()
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, source); err != nil {
		return err
	}
	return out.Close()
}

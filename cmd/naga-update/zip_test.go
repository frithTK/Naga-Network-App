package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractZipWritesFiles(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "bundle.zip")
	if err := writeTestZip(zipPath, map[string]string{
		"NagaNetwork.exe":  "launcher",
		"naga-control.exe": "control",
	}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")
	if err := extractZip(zipPath, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "NagaNetwork.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "launcher" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractZipRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "evil.zip")
	if err := writeTestZip(zipPath, map[string]string{
		"../escape.txt": "nope",
	}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")
	if err := extractZip(zipPath, dest); err == nil {
		t.Fatal("expected traversal error")
	}
}

func writeTestZip(path string, files map[string]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			return err
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return file.Close()
}

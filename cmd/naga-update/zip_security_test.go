package main

import (
	"archive/zip"
	"errors"
	"os"
	"path"
	"path/filepath"
	"testing"
)

func TestExtractZipDoesNotFollowDestinationSymlink(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "installation")
	outside := filepath.Join(root, "private")
	for _, dir := range []string{destination, outside} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(sentinel, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(destination, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	archive := filepath.Join(root, "update.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("linked/sentinel")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write([]byte("overwritten")); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractZip(archive, destination); err == nil {
		t.Fatal("expected rejection of a symlink parent")
	}
	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatal("updater wrote outside installation through an existing directory symlink")
	}
}

func TestPortableZipNames(t *testing.T) {
	for _, name := range []string{"../escape", "dir/../escape", "/absolute", `C:\outside`, `\\host\share\file`, "file:stream", "nul.txt", "COM¹.txt", "dir/file. ", "a//b", ".naga-update-any/file"} {
		if _, err := portableZipName(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for _, name := range []string{"NagaNetwork.exe", "data/flutter_assets/fonts/Inter.ttf", `data\flutter_assets\fonts\Inter.ttf`} {
		if _, err := portableZipName(name); err != nil {
			t.Errorf("rejected %q: %v", name, err)
		}
	}
}

func TestZipRejectsInvalidEntryBeforeReplacingExistingFile(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "update.zip")
	if err := writeTestZip(archive, map[string]string{"existing.txt": "new", "../escape": "bad"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "installation")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dest, "existing.txt")
	if err := os.WriteFile(file, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractZip(archive, dest); err == nil {
		t.Fatal("accepted traversal")
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != "old" {
		t.Fatalf("old installation changed: %q %v", got, err)
	}
}

func TestZipLimitsAndCaseCollisions(t *testing.T) {
	for _, test := range []struct {
		name   string
		files  map[string]string
		limits zipLimits
	}{
		{"count", map[string]string{"a": "a", "b": "b"}, zipLimits{1, 10, 20}},
		{"file", map[string]string{"a": "large"}, zipLimits{10, 2, 20}},
		{"total", map[string]string{"a": "123", "b": "456"}, zipLimits{10, 10, 5}},
		{"case", map[string]string{"a.txt": "a", "A.TXT": "b"}, zipLimits{10, 10, 20}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "update.zip")
			if err := writeTestZip(archive, test.files); err != nil {
				t.Fatal(err)
			}
			if err := extractZipLimited(archive, filepath.Join(dir, "out"), test.limits); err == nil {
				t.Fatal("expected limit/collision error")
			}
		})
	}
}

func TestZipReplacementFailureRollsBackEarlierFiles(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, name := range []string{"stage", "backup"} {
		if err := root.Mkdir(name, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"first", "second"} {
		if err := root.WriteFile(name, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := root.WriteFile(path.Join("stage", name), []byte("new"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	entries := []zipEntry{{name: "first", file: &zip.File{}}, {name: "second", file: &zip.File{}}}
	failure := errors.New("injected replacement failure")
	err = applyZipEntries(root, "stage", "backup", entries, func(old, new string) error {
		if old == "stage/second" {
			return failure
		}
		return root.Rename(old, new)
	})
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v", err)
	}
	for _, name := range []string{"first", "second"} {
		got, err := root.ReadFile(name)
		if err != nil || string(got) != "old" {
			t.Fatalf("%s was not restored: %q %v", name, got, err)
		}
	}
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateManifestMatchesCheckedInFile(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	got, err := generateManifest(root)
	if err != nil {
		t.Fatalf("generate manifest: %v", err)
	}

	want, err := os.ReadFile(filepath.Join(root, "manifest", "utils.json"))
	if err != nil {
		t.Fatalf("read checked-in manifest: %v", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("manifest/utils.json is out of date; run `make manifest`")
	}
}

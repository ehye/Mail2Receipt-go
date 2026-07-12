//go:build !windows

package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicReplaceWithoutForcePreservesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "staged.pdf")
	destination := filepath.Join(dir, "output.pdf")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := atomicReplace(source, destination, false)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("error = %v, want os.ErrExist", err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "old" {
		t.Fatalf("destination = %q, %v", got, err)
	}
}

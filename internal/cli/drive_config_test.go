package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrepareRcloneConfigCreatesSecureFileWithoutOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rclone.conf")
	if err := prepareRcloneConfig(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("created Rclone config permissions = %04o, want 0600", info.Mode().Perm())
	}
	if err := os.WriteFile(path, []byte("[existing]\ntype = drive\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := prepareRcloneConfig(path); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "[existing]\ntype = drive\n" {
		t.Fatalf("existing Rclone config was changed: %q", contents)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("existing Rclone config permissions = %04o, want 0600", info.Mode().Perm())
	}
}

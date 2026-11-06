package archive

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateVerifyAndFilename(t *testing.T) {
	dir := t.TempDir()
	metadata := Metadata{JobName: "db1", Driver: "postgres", TimestampUTC: time.Date(2026, 11, 6, 2, 0, 0, 123, time.UTC), AppVersion: "v1.0.0", FormatVersion: 1}
	path, err := Create(dir, metadata, "backup.sql", func(w io.Writer) error { _, err := io.WriteString(w, "SELECT 1;\n"); return err })
	if err != nil {
		t.Fatal(err)
	}
	if !IsBackupName(filepath.Base(path)) {
		t.Fatalf("invalid generated filename %q", filepath.Base(path))
	}
	got, err := Verify(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.JobName != metadata.JobName || got.Driver != metadata.Driver || got.DumpDuration < 0 {
		t.Fatalf("unexpected metadata: %#v", got)
	}
}

func TestCreateCleansPartialArchiveOnDumpFailure(t *testing.T) {
	dir := t.TempDir()
	_, err := Create(dir, Metadata{JobName: "db", Driver: "sqlite", TimestampUTC: time.Now(), FormatVersion: 1}, "database.sqlite", func(w io.Writer) error {
		_, _ = w.Write([]byte("partial"))
		return errors.New("dump failed")
	})
	if err == nil {
		t.Fatal("expected dump error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial files remain: %#v", entries)
	}
}

func TestVerifyRejectsBadChecksum(t *testing.T) {
	path, err := Create(t.TempDir(), Metadata{JobName: "db", Driver: "sqlite", TimestampUTC: time.Now(), FormatVersion: 1}, "database.sqlite", func(w io.Writer) error { _, err := w.Write([]byte("sqlite")); return err })
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".sha256", []byte("0000  "+filepath.Base(path)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(path); err == nil {
		t.Fatal("accepted invalid checksum")
	}
}

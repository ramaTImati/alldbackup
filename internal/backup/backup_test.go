package backup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"
	"github.com/ramaTImati/alldbackup/internal/archive"
	"github.com/ramaTImati/alldbackup/internal/config"
)

type fakeDumper struct{ failName string }

func (f fakeDumper) Check(_ context.Context, job config.Job, _ map[string]string) error {
	if job.Name == f.failName {
		return errors.New("database unavailable")
	}
	return nil
}

func TestRunSkipsAJobThatIsAlreadyLocked(t *testing.T) {
	root := t.TempDir()
	lockDir := filepath.Join(root, ".locks")
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		t.Fatal(err)
	}
	job := config.Job{Name: "db", Enabled: true, Driver: "sqlite", Source: "host", File: "/tmp/db.sqlite"}
	lock := flock.New(filepath.Join(lockDir, job.Name+".lock"))
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()
	c := config.Defaults()
	c.SecretsFile = ""
	c.Storage.Local.Path = root
	runner := Runner{Config: c, Dumper: fakeDumper{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	err := runner.runJob(context.Background(), job, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("unexpected lock error: %v", err)
	}
}
func (fakeDumper) Dump(_ context.Context, _ config.Job, _ map[string]string, dst io.Writer) error {
	_, err := io.WriteString(dst, "CREATE TABLE example(id INTEGER);\n")
	return err
}

func TestRunContinuesAfterJobFailure(t *testing.T) {
	root := t.TempDir()
	c := config.Defaults()
	c.SecretsFile = ""
	c.Storage.Local.Path = root
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := Runner{Config: c, Dumper: fakeDumper{failName: "first"}, Logger: logger, Version: "test"}
	jobs := []config.Job{
		{Name: "first", Enabled: true, Driver: "sqlite", Source: "host", File: "/tmp/first.sqlite"},
		{Name: "second", Enabled: true, Driver: "sqlite", Source: "host", File: "/tmp/second.sqlite"},
	}
	if err := runner.Run(context.Background(), jobs); err == nil {
		t.Fatal("expected combined job error")
	}
	entries, err := os.ReadDir(filepath.Join(root, "second"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected ZIP and checksum, got %d files", len(entries))
	}
	var zipPath string
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".zip" {
			zipPath = filepath.Join(root, "second", entry.Name())
		}
	}
	if zipPath == "" {
		t.Fatal("second job archive not found")
	}
	if _, err := archive.Verify(zipPath); err != nil {
		t.Fatal(err)
	}
}

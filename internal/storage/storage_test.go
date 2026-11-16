package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ramaTImati/alldbackup/internal/archive"
	"github.com/ramaTImati/alldbackup/internal/config"
)

type fakeExec struct {
	calls       [][]string
	listing     string
	runFailures int
}

func (f *fakeExec) Run(_ context.Context, name string, args, _ []string, _ io.Writer) error {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	if f.runFailures > 0 && containsArg(args, "copyto") {
		f.runFailures--
		return errors.New("mock rclone failure")
	}
	return nil
}

func containsArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}

func TestUploadFailureKeepsLocalFilesForRetry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.zip")
	if err := os.WriteFile(path, []byte("zip"), 0600); err != nil {
		t.Fatal(err)
	}
	exec := &fakeExec{runFailures: 2}
	drive := &Rclone{Exec: exec, Config: config.DriveConfig{Remote: "gdrive:root", Timeout: "1m", Retries: 2}}
	if err := drive.Upload(context.Background(), config.Job{Name: "db"}, path); err == nil {
		t.Fatal("expected upload failure")
	}
	if len(exec.calls) != 3 {
		t.Fatalf("got %d calls, want folder creation plus two upload attempts", len(exec.calls))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("local retry archive removed: %v", err)
	}
	if _, err := os.Stat(path + ".uploaded"); !os.IsNotExist(err) {
		t.Fatalf("failed upload was marked successful: %v", err)
	}
}
func (f *fakeExec) Output(_ context.Context, name string, args, _ []string) (string, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	if name == "rclone" && len(args) > 0 && args[0] == "lsf" {
		return f.listing, nil
	}
	return "", nil
}

func TestUploadCopiesAndVerifiesArchiveOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.zip")
	if err := os.WriteFile(path, []byte("zip"), 0600); err != nil {
		t.Fatal(err)
	}
	exec := &fakeExec{}
	drive := &Rclone{Exec: exec, Config: config.DriveConfig{Enabled: true, Remote: "gdrive:root", ConfigFile: "/etc/alldbackup/rclone.conf", Timeout: "1m", Retries: 2}}
	if err := drive.Upload(context.Background(), config.Job{Name: "db", GoogleDrivePath: "team/finance"}, path); err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 3 {
		t.Fatalf("got %d rclone calls, want folder creation, archive upload, and archive verification", len(exec.calls))
	}
	marker, err := os.ReadFile(path + ".uploaded")
	if err != nil || len(marker) == 0 {
		t.Fatalf("upload marker missing: %v", err)
	}
	for _, call := range exec.calls {
		if len(call) < 3 || call[1] != "--config" || call[2] != "/etc/alldbackup/rclone.conf" {
			t.Fatalf("rclone call did not use the configured file: %#v", call)
		}
		if strings.Contains(strings.Join(call, " "), "password") {
			t.Fatalf("unexpected credential in arguments: %#v", call)
		}
	}
	if !containsArg(exec.calls[0], "mkdir") {
		t.Fatalf("first rclone call did not create the remote folder: %#v", exec.calls[0])
	}
	if got, want := exec.calls[0][len(exec.calls[0])-1], "gdrive:root/team/finance"; got != want {
		t.Fatalf("created remote folder = %q, want %q", got, want)
	}
	if got, want := exec.calls[1][len(exec.calls[1])-1], "gdrive:root/team/finance/backup.zip"; got != want {
		t.Fatalf("uploaded archive remote path = %q, want %q", got, want)
	}
	if got, want := exec.calls[2][len(exec.calls[2])-1], "gdrive:root/team/finance"; got != want {
		t.Fatalf("archive verification remote directory = %q, want %q", got, want)
	}
	if !strings.Contains(strings.Join(exec.calls[2], " "), "--include backup.zip") {
		t.Fatalf("archive verification did not filter to backup.zip: %#v", exec.calls[2])
	}
}

func TestLocalRetentionPreservesPendingUpload(t *testing.T) {
	root := t.TempDir()
	job := config.Job{Name: "db"}
	dir := JobDirectory(root, job)
	old := time.Now().Add(-40 * 24 * time.Hour).UTC()
	path, err := archive.Create(dir, archive.Metadata{JobName: "db", Driver: "sqlite", TimestampUTC: old, FormatVersion: 1}, "database.sqlite", func(w io.Writer) error { _, err := w.Write([]byte("sqlite")); return err })
	if err != nil {
		t.Fatal(err)
	}
	if err := PruneLocal(root, job, 7, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("pending backup was removed", err)
	}
	if err := os.WriteFile(path+".uploaded", []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PruneLocal(root, job, 7, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired uploaded archive still exists: %v", err)
	}
}

func TestRemoteRetentionDeletesOnlyRecognizedArchivePaths(t *testing.T) {
	old := archive.Filename(time.Now().Add(-40 * 24 * time.Hour))
	exec := &fakeExec{listing: old + "\n" + old + ".sha256\nnotes.txt\n"}
	drive := &Rclone{Exec: exec, Config: config.DriveConfig{Enabled: true, Remote: "gdrive:root", Timeout: "1m", Retries: 1, RetentionDays: 7}}
	if err := drive.Prune(context.Background(), config.Job{Name: "db", GoogleDrivePath: "team/dbs"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 3 {
		t.Fatalf("got %d calls, want list plus two exact deletes", len(exec.calls))
	}
	if !strings.Contains(strings.Join(exec.calls[1], " "), old) || !strings.Contains(strings.Join(exec.calls[2], " "), old+".sha256") {
		t.Fatalf("unexpected delete paths: %#v", exec.calls)
	}
	if got, want := exec.calls[0][len(exec.calls[0])-1], "gdrive:root/team/dbs"; got != want {
		t.Fatalf("remote retention directory = %q, want %q", got, want)
	}
	if got, want := exec.calls[1][len(exec.calls[1])-1], "gdrive:root/team/dbs/"+old; got != want {
		t.Fatalf("remote deletion path = %q, want %q", got, want)
	}
}

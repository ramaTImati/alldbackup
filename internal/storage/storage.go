package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ramaTImati/alldbackup/internal/archive"
	"github.com/ramaTImati/alldbackup/internal/config"
	"github.com/ramaTImati/alldbackup/internal/database"
)

type Uploader interface {
	Upload(context.Context, config.Job, string) error
	Prune(context.Context, config.Job, time.Time) error
}

type Rclone struct {
	Exec   database.Executor
	Config config.DriveConfig
}

func NewRclone(c config.DriveConfig) *Rclone { return &Rclone{Exec: database.OSExecutor{}, Config: c} }

func (r *Rclone) Test(ctx context.Context) error {
	if strings.TrimSpace(r.Config.Remote) == "" {
		return errors.New("Google Drive remote is not configured")
	}
	callCtx, cancel := context.WithTimeout(ctx, mustDuration(r.Config.Timeout))
	defer cancel()
	if _, err := r.Exec.Output(callCtx, "rclone", r.args("lsd", r.Config.Remote), nil); err != nil {
		return fmt.Errorf("Google Drive remote test failed: %w", err)
	}
	return nil
}

func JobDirectory(root string, job config.Job) string { return filepath.Join(root, job.Name) }

func (r *Rclone) Upload(ctx context.Context, job config.Job, localPath string) error {
	remote := r.jobRemoteDir(job) + "/" + filepath.Base(localPath)
	if err := r.copy(ctx, localPath, remote); err != nil {
		return err
	}
	if err := r.check(ctx, localPath, remote); err != nil {
		return fmt.Errorf("verify uploaded archive: %w", err)
	}
	sidecar := localPath + ".sha256"
	remoteSidecar := remote + ".sha256"
	if err := r.copy(ctx, sidecar, remoteSidecar); err != nil {
		return err
	}
	if err := r.check(ctx, sidecar, remoteSidecar); err != nil {
		return fmt.Errorf("verify uploaded checksum: %w", err)
	}
	if err := os.WriteFile(localPath+".uploaded", []byte(time.Now().UTC().Format(time.RFC3339Nano)+"\n"), 0600); err != nil {
		return fmt.Errorf("mark backup as uploaded: %w", err)
	}
	return nil
}

func (r *Rclone) copy(ctx context.Context, localPath, remote string) error {
	var last error
	for attempt := 1; attempt <= r.Config.Retries; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, mustDuration(r.Config.Timeout))
		last = r.Exec.Run(callCtx, "rclone", r.args("copyto", "--retries=1", "--timeout="+r.Config.Timeout, localPath, remote), nil, io.Discard)
		cancel()
		if last == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("rclone upload failed after %d attempts: %w", r.Config.Retries, last)
}

func (r *Rclone) check(ctx context.Context, localPath, remote string) error {
	name := filepath.Base(localPath)
	localDir := filepath.Dir(localPath)
	remoteDir := path.Dir(remote)
	callCtx, cancel := context.WithTimeout(ctx, mustDuration(r.Config.Timeout))
	err := r.Exec.Run(callCtx, "rclone", r.args("check", "--one-way", "--include", name, localDir, remoteDir), nil, io.Discard)
	cancel()
	if err == nil {
		return nil
	}
	callCtx, cancel = context.WithTimeout(ctx, mustDuration(r.Config.Timeout))
	defer cancel()
	if cryptErr := r.Exec.Run(callCtx, "rclone", r.args("cryptcheck", "--one-way", "--include", name, localDir, remoteDir), nil, io.Discard); cryptErr == nil {
		return nil
	} else {
		return errors.Join(fmt.Errorf("rclone check: %w", err), fmt.Errorf("rclone cryptcheck: %w", cryptErr))
	}
}

func (r *Rclone) Prune(ctx context.Context, job config.Job, now time.Time) error {
	days := r.Config.RetentionDays
	if days == 0 {
		return nil
	}
	remoteDir := r.jobRemoteDir(job)
	callCtx, cancel := context.WithTimeout(ctx, mustDuration(r.Config.Timeout))
	listing, err := r.Exec.Output(callCtx, "rclone", r.args("lsf", "--files-only", "--max-depth=1", remoteDir), nil)
	cancel()
	if err != nil {
		return fmt.Errorf("list Google Drive backups for retention: %w", err)
	}
	remoteFiles := make(map[string]bool)
	for _, name := range strings.Split(listing, "\n") {
		remoteFiles[strings.TrimSpace(name)] = true
	}
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	for name := range remoteFiles {
		backupTime, ok := archive.BackupTime(name)
		if !ok || !backupTime.Before(cutoff) {
			continue
		}
		for _, suffix := range []string{"", ".sha256"} {
			if !remoteFiles[name+suffix] {
				continue
			}
			callCtx, cancel := context.WithTimeout(ctx, mustDuration(r.Config.Timeout))
			err := r.Exec.Run(callCtx, "rclone", r.args("deletefile", remoteDir+"/"+name+suffix), nil, io.Discard)
			cancel()
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("delete expired remote backup %q: %w", name+suffix, err)
			}
		}
	}
	return nil
}

func (r *Rclone) args(args ...string) []string {
	if r.Config.ConfigFile == "" {
		return args
	}
	return append([]string{"--config", r.Config.ConfigFile}, args...)
}

func (r *Rclone) jobRemoteDir(job config.Job) string {
	jobPath := job.GoogleDrivePath
	if strings.TrimSpace(jobPath) == "" {
		jobPath = job.Name
	}
	return strings.TrimRight(r.Config.Remote, "/") + "/" + jobPath
}

func PruneLocal(root string, job config.Job, days int, requireUploaded bool, now time.Time) error {
	if days == 0 {
		return nil
	}
	dir := JobDirectory(root, job)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	for _, entry := range entries {
		name := entry.Name()
		backupTime, ok := archive.BackupTime(name)
		if !ok || !backupTime.Before(cutoff) {
			continue
		}
		path := filepath.Join(dir, name)
		if !regularFile(path) {
			continue
		}
		if requireUploaded {
			if !regularFile(path + ".uploaded") {
				if _, err := os.Lstat(path + ".uploaded"); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				continue
			}
		}
		for _, suffix := range []string{"", ".sha256", ".uploaded"} {
			if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove expired local backup %q: %w", name+suffix, err)
			}
		}
	}
	return nil
}

func Pending(root string, job config.Job) ([]string, error) {
	dir := JobDirectory(root, job)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var pending []string
	for _, entry := range entries {
		name := entry.Name()
		if !archive.IsBackupName(name) || entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, name)
		if !regularFile(path) || !regularFile(path+".sha256") {
			continue
		}
		if regularFile(path + ".uploaded") {
			continue
		}
		if _, err := archive.Verify(path); err != nil {
			return nil, fmt.Errorf("pending backup %q failed verification: %w", name, err)
		}
		pending = append(pending, path)
	}
	return pending, nil
}

func regularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func mustDuration(value string) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 30 * time.Minute
	}
	return d
}

var _ Uploader = (*Rclone)(nil)

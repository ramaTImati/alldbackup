package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/ramaTImati/alldbackup/internal/archive"
	"github.com/ramaTImati/alldbackup/internal/config"
	"github.com/ramaTImati/alldbackup/internal/database"
	"github.com/ramaTImati/alldbackup/internal/storage"
)

type Runner struct {
	Config   config.Config
	Dumper   database.Dumper
	Uploader storage.Uploader
	Logger   *slog.Logger
	Version  string
}

func (r *Runner) Run(ctx context.Context, jobs []config.Job) error {
	if r.Logger == nil {
		r.Logger = slog.Default()
	}
	secrets, err := config.ReadSecrets(r.Config.SecretsFile, r.Logger.Warn)
	if err != nil {
		return err
	}
	var failures []error
	for _, job := range jobs {
		if err := r.runJob(ctx, job, secrets); err != nil {
			failures = append(failures, fmt.Errorf("job %s: %s", job.Name, redact(err.Error(), job, secrets)))
		}
	}
	return errors.Join(failures...)
}

func (r *Runner) runJob(ctx context.Context, job config.Job, secrets map[string]string) error {
	if r.Logger == nil {
		r.Logger = slog.Default()
	}
	root := r.Config.Storage.Local.Path
	dir := storage.JobDirectory(root, job)
	if err := os.MkdirAll(root, 0755); err != nil {
		return fmt.Errorf("create backup storage directory: %w", err)
	}
	if err := os.Chmod(root, 0755); err != nil {
		return fmt.Errorf("set backup storage directory permissions: %w", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		return fmt.Errorf("set backup directory permissions: %w", err)
	}
	lockDir := filepath.Join(root, ".locks")
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		return fmt.Errorf("create lock directory: %w", err)
	}
	lock := flock.New(filepath.Join(lockDir, job.Name+".lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return fmt.Errorf("acquire backup lock: %w", err)
	}
	if !locked {
		return errors.New("backup job is already running")
	}
	defer lock.Unlock()

	started := time.Now()
	timestampLocation, err := r.Config.TimestampLocation()
	if err != nil {
		return err
	}
	r.Logger.Info("starting backup job", "job", job.Name, "driver", job.Driver, "source", job.Source)
	if free, err := storage.FreeBytes(dir); err != nil {
		r.Logger.Warn("could not check free disk space", "job", job.Name, "error", err)
	} else if free == 0 {
		return errors.New("backup storage has no free disk space")
	} else if free < 100*1024*1024 {
		r.Logger.Warn("backup storage has less than 100 MiB free", "job", job.Name, "free_bytes", free)
	}
	if err := r.Dumper.Check(ctx, job, secrets); err != nil {
		return fmt.Errorf("database check failed: %w", err)
	}
	dumpName := "backup.sql"
	if job.Driver == "sqlite" {
		dumpName = "database.sqlite"
	}
	metadata := archive.Metadata{JobName: job.Name, Driver: job.Driver, TimestampUTC: started.In(timestampLocation), AppVersion: r.Version, FormatVersion: 1}
	path, err := archive.Create(dir, metadata, dumpName, func(dst io.Writer) error {
		return r.Dumper.Dump(ctx, job, secrets, dst)
	})
	if err != nil {
		return fmt.Errorf("create backup archive: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	r.Logger.Info("backup archive created", "job", job.Name, "path", path, "size_bytes", info.Size(), "duration", time.Since(started).Round(time.Millisecond))

	var failures []error
	if r.Config.Storage.GoogleDrive.Enabled {
		pending, err := storage.Pending(root, job)
		if err != nil {
			failures = append(failures, err)
		} else {
			for _, pendingPath := range pending {
				if err := r.Uploader.Upload(ctx, job, pendingPath); err != nil {
					failures = append(failures, fmt.Errorf("Google Drive upload failed for %s: %w", filepath.Base(pendingPath), err))
					r.Logger.Error("Google Drive upload failed; local backup retained", "job", job.Name, "file", filepath.Base(pendingPath), "error", redact(err.Error(), job, secrets))
					continue
				}
				r.Logger.Info("Google Drive upload completed", "job", job.Name, "file", filepath.Base(pendingPath))
				if !r.Config.Storage.Local.Enabled {
					for _, suffix := range []string{"", ".sha256", ".uploaded"} {
						if err := os.Remove(pendingPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
							failures = append(failures, fmt.Errorf("remove uploaded spool file %q: %w", filepath.Base(pendingPath+suffix), err))
						}
					}
				}
			}
		}
		if err := r.Uploader.Prune(ctx, job, time.Now().UTC()); err != nil {
			failures = append(failures, fmt.Errorf("Google Drive retention failed: %w", err))
		} else {
			r.Logger.Info("Google Drive retention completed", "job", job.Name)
		}
	}
	if r.Config.Storage.Local.Enabled {
		if err := storage.PruneLocal(root, job, r.Config.Storage.Local.RetentionDays, r.Config.Storage.GoogleDrive.Enabled, time.Now().UTC()); err != nil {
			failures = append(failures, fmt.Errorf("local retention failed: %w", err))
		} else {
			r.Logger.Info("local retention completed", "job", job.Name)
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	r.Logger.Info("backup completed", "job", job.Name, "duration", time.Since(started).Round(time.Millisecond))
	return nil
}

func redact(message string, job config.Job, secrets map[string]string) string {
	value, ok := os.LookupEnv(job.PasswordEnv)
	if !ok {
		value = secrets[job.PasswordEnv]
	}
	if value != "" {
		message = strings.ReplaceAll(message, value, "[redacted]")
	}
	for _, value := range secrets {
		if value != "" {
			message = strings.ReplaceAll(message, value, "[redacted]")
		}
	}
	return message
}

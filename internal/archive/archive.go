package archive

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const TimestampLayout = "20060102T150405.000000000Z0700"

var filenamePattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}(Z|[+-][0-9]{4})\.zip$`)

type Metadata struct {
	JobName       string        `json:"job_name"`
	Driver        string        `json:"driver"`
	TimestampUTC  time.Time     `json:"timestamp_utc"`
	AppVersion    string        `json:"application_version"`
	FormatVersion int           `json:"backup_format_version"`
	DumpDuration  time.Duration `json:"dump_duration"`
}

func Filename(t time.Time) string { return t.Format(TimestampLayout) + ".zip" }

func IsBackupName(name string) bool {
	_, ok := BackupTime(name)
	return ok
}

func BackupTime(name string) (time.Time, bool) {
	if !filenamePattern.MatchString(name) {
		return time.Time{}, false
	}
	t, err := time.Parse(TimestampLayout+".zip", name)
	if err != nil {
		return time.Time{}, false
	}
	_, offset := t.Zone()
	if offset < -14*60*60 || offset > 14*60*60 {
		return time.Time{}, false
	}
	return t, true
}

// Create writes a complete archive and checksum beside each other, then publishes both atomically.
func Create(dir string, metadata Metadata, dumpName string, dump func(io.Writer) error) (string, error) {
	if dumpName != "backup.sql" && dumpName != "database.sqlite" {
		return "", fmt.Errorf("invalid dump entry name %q", dumpName)
	}
	filenameTime := metadata.TimestampUTC
	metadata.TimestampUTC = metadata.TimestampUTC.UTC()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	name := Filename(filenameTime)
	finalPath := filepath.Join(dir, name)
	if _, err := os.Lstat(finalPath); err == nil {
		return "", fmt.Errorf("backup already exists: %s", finalPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if _, err := os.Lstat(finalPath + ".sha256"); err == nil {
		return "", fmt.Errorf("backup checksum already exists: %s.sha256", finalPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	partialPath := finalPath + ".partial"
	partialSidecar := finalPath + ".sha256.partial"
	finalSidecar := finalPath + ".sha256"
	f, err := os.OpenFile(partialPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("create partial archive: %w", err)
	}
	completed := false
	defer func() {
		f.Close()
		if !completed {
			os.Remove(partialPath)
			os.Remove(partialSidecar)
			os.Remove(finalPath)
			os.Remove(finalSidecar)
		}
	}()
	zw := zip.NewWriter(f)
	entry, err := zw.Create(dumpName)
	dumpStarted := time.Now()
	if err == nil {
		err = dump(entry)
	}
	metadata.DumpDuration = time.Since(dumpStarted)
	if err != nil {
		zw.Close()
		return "", fmt.Errorf("write database dump: %w", err)
	}
	metaBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		zw.Close()
		return "", fmt.Errorf("encode backup metadata: %w", err)
	}
	metaEntry, err := zw.Create("metadata.json")
	if err != nil {
		zw.Close()
		return "", err
	}
	if _, err = metaEntry.Write(append(metaBytes, '\n')); err != nil {
		zw.Close()
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("finalize ZIP: %w", err)
	}
	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("sync ZIP: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close ZIP: %w", err)
	}
	checksum, err := fileChecksum(partialPath)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(partialSidecar, []byte(checksum+"  "+name+"\n"), 0600); err != nil {
		return "", fmt.Errorf("write checksum: %w", err)
	}
	for _, path := range []string{partialPath, partialSidecar} {
		if err := os.Chmod(path, 0664); err != nil {
			return "", fmt.Errorf("make backup readable by all users: %w", err)
		}
	}
	if err := os.Rename(partialPath, finalPath); err != nil {
		return "", fmt.Errorf("publish ZIP: %w", err)
	}
	if err := os.Rename(partialSidecar, finalSidecar); err != nil {
		return "", fmt.Errorf("publish checksum: %w", err)
	}
	completed = true
	return finalPath, nil
}

func Verify(path string) (Metadata, error) {
	checksum, err := fileChecksum(path)
	if err != nil {
		return Metadata{}, err
	}
	base := filepath.Base(path)
	data, err := os.ReadFile(path + ".sha256")
	if err != nil {
		return Metadata{}, fmt.Errorf("read checksum sidecar: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 || fields[0] != checksum || fields[1] != base {
		return Metadata{}, errors.New("backup checksum does not match sidecar")
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("open ZIP: %w", err)
	}
	defer r.Close()
	var meta Metadata
	entries := map[string]bool{}
	for _, file := range r.File {
		if file.Name != "backup.sql" && file.Name != "database.sqlite" && file.Name != "metadata.json" {
			return Metadata{}, fmt.Errorf("unexpected ZIP entry %q", file.Name)
		}
		if entries[file.Name] {
			return Metadata{}, fmt.Errorf("duplicate ZIP entry %q", file.Name)
		}
		entries[file.Name] = true
		reader, err := file.Open()
		if err != nil {
			return Metadata{}, err
		}
		if file.Name == "metadata.json" {
			err = json.NewDecoder(reader).Decode(&meta)
		} else {
			_, err = io.Copy(io.Discard, reader)
		}
		closeErr := reader.Close()
		if err != nil {
			return Metadata{}, fmt.Errorf("read ZIP entry %q: %w", file.Name, err)
		}
		if closeErr != nil {
			return Metadata{}, closeErr
		}
	}
	if !entries["metadata.json"] || !(entries["backup.sql"] != entries["database.sqlite"]) {
		return Metadata{}, errors.New("ZIP must contain metadata.json and exactly one database dump")
	}
	if meta.JobName == "" || meta.Driver == "" || meta.FormatVersion != 1 || meta.TimestampUTC.IsZero() {
		return Metadata{}, errors.New("backup metadata is incomplete or unsupported")
	}
	if meta.DumpDuration < 0 {
		return Metadata{}, errors.New("backup metadata has a negative dump duration")
	}
	if meta.Driver != "mysql" && meta.Driver != "mariadb" && meta.Driver != "postgres" && meta.Driver != "sqlite" {
		return Metadata{}, fmt.Errorf("unsupported database driver %q in metadata", meta.Driver)
	}
	if entries["database.sqlite"] != (meta.Driver == "sqlite") {
		return Metadata{}, errors.New("backup dump entry does not match database driver")
	}
	if timestamp, ok := BackupTime(base); !ok || !timestamp.Equal(meta.TimestampUTC.UTC()) {
		return Metadata{}, errors.New("backup filename timestamp does not match metadata")
	}
	return meta, nil
}

func fileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

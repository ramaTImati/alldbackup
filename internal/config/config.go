package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const DefaultPath = "/etc/alldbackup/config.yaml"

var jobNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var containerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var remotePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*:.+$`)

type Config struct {
	Version     int           `yaml:"version"`
	SecretsFile string        `yaml:"secrets_file"`
	Storage     StorageConfig `yaml:"storage"`
	Jobs        []Job         `yaml:"jobs"`
}

type StorageConfig struct {
	Local       LocalConfig `yaml:"local"`
	GoogleDrive DriveConfig `yaml:"google_drive"`
}

type LocalConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Path          string `yaml:"path"`
	RetentionDays int    `yaml:"retention_days"`
}

type DriveConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Remote        string `yaml:"remote"`
	RetentionDays int    `yaml:"retention_days"`
	Timeout       string `yaml:"timeout"`
	Retries       int    `yaml:"retries"`
}

type Job struct {
	Name        string `yaml:"name"`
	Enabled     bool   `yaml:"enabled"`
	Driver      string `yaml:"driver"`
	Source      string `yaml:"source"`
	Host        string `yaml:"host,omitempty"`
	Port        int    `yaml:"port,omitempty"`
	Container   string `yaml:"container,omitempty"`
	Database    string `yaml:"database,omitempty"`
	Username    string `yaml:"username,omitempty"`
	PasswordEnv string `yaml:"password_env,omitempty"`
	File        string `yaml:"file,omitempty"`
}

func Defaults() Config {
	return Config{
		Version:     1,
		SecretsFile: "/etc/alldbackup/secrets.env",
		Storage: StorageConfig{
			Local:       LocalConfig{Enabled: true, Path: "/var/backups/alldbackup", RetentionDays: 7},
			GoogleDrive: DriveConfig{Enabled: false, RetentionDays: 30, Timeout: "30m", Retries: 3},
		},
	}
}

func DefaultsFor(path string) Config {
	c := Defaults()
	if filepath.Clean(path) != filepath.Clean(DefaultPath) {
		absolute, err := filepath.Abs(path)
		if err == nil {
			path = absolute
		}
		base := filepath.Dir(path)
		c.SecretsFile = filepath.Join(base, "secrets.env")
		c.Storage.Local.Path = filepath.Join(base, "backups")
	}
	return c
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	c := DefaultsFor(path)
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	if c.SecretsFile != "" && !filepath.IsAbs(c.SecretsFile) {
		c.SecretsFile = filepath.Join(filepath.Dir(path), c.SecretsFile)
	}
	if c.Storage.Local.Path != "" && !filepath.IsAbs(c.Storage.Local.Path) {
		c.Storage.Local.Path = filepath.Join(filepath.Dir(path), c.Storage.Local.Path)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported configuration version %d (expected 1)", c.Version)
	}
	if !c.Storage.Local.Enabled && !c.Storage.GoogleDrive.Enabled {
		return errors.New("at least one storage backend must be enabled")
	}
	if c.Storage.Local.RetentionDays < 0 || c.Storage.Local.RetentionDays > 36500 {
		return errors.New("storage.local.retention_days must be between 0 and 36500")
	}
	if c.Storage.GoogleDrive.RetentionDays < 0 || c.Storage.GoogleDrive.RetentionDays > 36500 {
		return errors.New("storage.google_drive.retention_days must be between 0 and 36500")
	}
	if c.Storage.GoogleDrive.Retries < 1 || c.Storage.GoogleDrive.Retries > 10 {
		return errors.New("storage.google_drive.retries must be between 1 and 10")
	}
	timeout, err := timeParseDuration(c.Storage.GoogleDrive.Timeout)
	if err != nil {
		return fmt.Errorf("storage.google_drive.timeout: %w", err)
	}
	if timeout <= 0 {
		return errors.New("storage.google_drive.timeout must be positive")
	}
	if c.Storage.GoogleDrive.Enabled {
		if !remotePattern.MatchString(c.Storage.GoogleDrive.Remote) {
			return errors.New("storage.google_drive.remote must use the form remote:path")
		}
	}
	if strings.TrimSpace(c.Storage.Local.Path) == "" {
		return errors.New("storage.local.path is required as the local backup destination or upload retry spool")
	}
	seen := make(map[string]bool, len(c.Jobs))
	for i, j := range c.Jobs {
		prefix := fmt.Sprintf("jobs[%d]", i)
		if !jobNamePattern.MatchString(j.Name) {
			return fmt.Errorf("%s.name must match [a-z0-9][a-z0-9_-]{0,62}", prefix)
		}
		if seen[j.Name] {
			return fmt.Errorf("duplicate job name %q", j.Name)
		}
		seen[j.Name] = true
		switch j.Driver {
		case "mysql", "mariadb", "postgres":
			if strings.TrimSpace(j.Database) == "" || strings.TrimSpace(j.Username) == "" || strings.TrimSpace(j.PasswordEnv) == "" {
				return fmt.Errorf("%s requires database, username, and password_env", prefix)
			}
			if !envNamePattern.MatchString(j.PasswordEnv) {
				return fmt.Errorf("%s.password_env must be a valid environment variable name", prefix)
			}
			if strings.HasPrefix(j.Database, "-") {
				return fmt.Errorf("%s.database cannot start with a hyphen", prefix)
			}
		case "sqlite":
			if strings.TrimSpace(j.File) == "" {
				return fmt.Errorf("%s.file is required for sqlite", prefix)
			}
			if strings.HasPrefix(j.File, "-") {
				return fmt.Errorf("%s.file cannot start with a hyphen", prefix)
			}
		default:
			return fmt.Errorf("%s.driver must be mysql, mariadb, postgres, or sqlite", prefix)
		}
		switch j.Source {
		case "host":
			if j.Driver != "sqlite" {
				if strings.TrimSpace(j.Host) == "" || j.Port < 1 || j.Port > 65535 {
					return fmt.Errorf("%s requires host and a valid port for host database sources", prefix)
				}
			}
		case "docker":
			if !containerNamePattern.MatchString(strings.TrimSpace(j.Container)) {
				return fmt.Errorf("%s.container must be a Docker name or ID", prefix)
			}
		default:
			return fmt.Errorf("%s.source must be host or docker", prefix)
		}
	}
	return nil
}

func (c Config) EnabledJobs() []Job {
	jobs := make([]Job, 0, len(c.Jobs))
	for _, j := range c.Jobs {
		if j.Enabled {
			jobs = append(jobs, j)
		}
	}
	return jobs
}

func (c Config) FindJob(name string) (Job, bool) {
	for _, j := range c.Jobs {
		if j.Name == name {
			return j, true
		}
	}
	return Job{}, false
}

func Write(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	return f.Close()
}

func AddJob(path string, job Job) error {
	c, err := Load(path)
	if err != nil {
		return err
	}
	job.Enabled = true
	c.Jobs = append(c.Jobs, job)
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".partial"
	defer os.Remove(tmp)
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func ReadSecrets(path string, warn func(string, ...any)) (map[string]string, error) {
	if path == "" {
		return map[string]string{}, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat secrets file: %w", err)
	}
	if info.Mode().Perm()&0077 != 0 && warn != nil {
		warn("secrets file permissions are broader than 0600", "path", path, "permissions", info.Mode().Perm().String())
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open secrets file: %w", err)
	}
	defer f.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		if err := parseSecretLine(values, scanner.Text(), line); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read secrets file: %w", err)
	}
	return values, nil
}

func parseSecretLine(values map[string]string, raw string, line int) error {
	lineText := strings.TrimSpace(raw)
	if lineText == "" || strings.HasPrefix(lineText, "#") {
		return nil
	}
	key, value, ok := strings.Cut(raw, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" || strings.ContainsAny(key, " \t\r") {
		return fmt.Errorf("invalid secrets file entry at line %d", line)
	}
	values[key] = strings.TrimSuffix(value, "\r")
	return nil
}

func timeParseDuration(value string) (time.Duration, error) {
	return time.ParseDuration(value)
}

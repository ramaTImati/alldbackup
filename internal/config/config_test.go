package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultsForAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	c := DefaultsFor(path)
	c.Jobs = []Job{{Name: "db1", Enabled: true, Driver: "sqlite", Source: "host", File: "/tmp/db.sqlite"}}
	if err := Write(path, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.EnabledJobs()) != 1 || loaded.EnabledJobs()[0].Name != "db1" {
		t.Fatalf("unexpected jobs: %#v", loaded.Jobs)
	}
	if loaded.Storage.Local.Path != c.Storage.Local.Path {
		t.Fatalf("storage path = %q, want %q", loaded.Storage.Local.Path, c.Storage.Local.Path)
	}
	if loaded.Storage.GoogleDrive.ConfigFile != filepath.Join(filepath.Dir(path), "rclone.conf") {
		t.Fatalf("Rclone config path = %q, want config directory path", loaded.Storage.GoogleDrive.ConfigFile)
	}
	if loaded.Jobs[0].Schedule != DefaultJobSchedule {
		t.Fatalf("missing schedule = %q, want default %q", loaded.Jobs[0].Schedule, DefaultJobSchedule)
	}
	if err := Write(path, c); !os.IsExist(err) {
		t.Fatalf("second write error = %v, want already exists", err)
	}
}

func TestValidateRejectsTraversalDuplicateAndMissingFields(t *testing.T) {
	c := Defaults()
	c.Jobs = []Job{{Name: "../escape", Enabled: true, Driver: "sqlite", Source: "host", File: "/tmp/db"}}
	if err := c.Validate(); err == nil {
		t.Fatal("accepted traversal job name")
	}
	c.Jobs = []Job{{Name: "db", Enabled: true, Driver: "sqlite", Source: "host", File: "/tmp/db"}, {Name: "db", Enabled: true, Driver: "sqlite", Source: "host", File: "/tmp/db"}}
	if err := c.Validate(); err == nil {
		t.Fatal("accepted duplicate job names")
	}
	c.Jobs = []Job{{Name: "db", Enabled: true, Schedule: DefaultJobSchedule, Driver: "postgres", Source: "host", Database: "app", Username: "backup", PasswordEnv: "DB_PASS", Host: "localhost", Port: 5432}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Storage.Local.RetentionDays = -1
	if err := c.Validate(); err == nil {
		t.Fatal("accepted negative retention")
	}
}

func TestScheduleValidation(t *testing.T) {
	c := Defaults()
	job := Job{Name: "db", Enabled: true, Driver: "sqlite", Source: "host", File: "/tmp/db.sqlite", Schedule: "*/15 1-5 * * 1-5"}
	c.Jobs = []Job{job}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid schedule rejected: %v", err)
	}

	for _, schedule := range []string{"hourly", "60 2 * * *", "0 2 * *"} {
		c.Jobs[0].Schedule = schedule
		if err := c.Validate(); err == nil {
			t.Errorf("accepted invalid schedule %q", schedule)
		}
	}

	c.Jobs[0].Schedule = ""
	if err := c.Validate(); err == nil {
		t.Fatal("accepted empty schedule before defaults are applied")
	}
}

func TestLoadDefaultsMissingJobScheduleToMidnight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("version: 1\njobs:\n  - name: sqlite\n    enabled: true\n    driver: sqlite\n    source: host\n    file: /tmp/db.sqlite\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Jobs[0].Schedule; got != DefaultJobSchedule {
		t.Fatalf("missing schedule = %q, want %q", got, DefaultJobSchedule)
	}
	wantRcloneConfig := filepath.Join(filepath.Dir(path), "rclone.conf")
	if got := c.Storage.GoogleDrive.ConfigFile; got != wantRcloneConfig {
		t.Fatalf("default Rclone config path = %q, want %q", got, wantRcloneConfig)
	}
}

func TestLoadResolvesRelativeRcloneConfigPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("version: 1\nstorage:\n  google_drive:\n    config_file: auth/rclone.conf\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(path), "auth", "rclone.conf")
	if c.Storage.GoogleDrive.ConfigFile != want {
		t.Fatalf("Rclone config path = %q, want %q", c.Storage.GoogleDrive.ConfigFile, want)
	}
}

func TestTimestampTimezoneAcceptsFixedOffsetsAndRejectsInvalidValues(t *testing.T) {
	c := Defaults()
	c.TimestampTimezone = "+07:00"
	location, err := c.TimestampLocation()
	if err != nil {
		t.Fatal(err)
	}
	if got := time.Date(2026, 1, 1, 0, 0, 0, 0, location).Format("Z0700"); got != "+0700" {
		t.Fatalf("formatted offset = %q, want +0700", got)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{"", "GMT+07:00", "+7:00", "+14:01", "-15:00"} {
		c.TimestampTimezone = value
		if err := c.Validate(); err == nil {
			t.Errorf("accepted invalid timestamp_timezone %q", value)
		}
	}
}

func TestLoadRejectsUnknownYAMLField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nstorage:\n  unknown: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("accepted unknown YAML field")
	}
}

func TestExampleConfigParses(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "config.example.yaml")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Jobs) != 4 {
		t.Fatalf("example has %d jobs, want 4", len(c.Jobs))
	}
	if c.TimestampTimezone != "+07:00" {
		t.Fatalf("example timestamp_timezone = %q, want +07:00", c.TimestampTimezone)
	}
}

func TestReadSecretsParsesValuesAndWarnsOnPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.env")
	if err := os.WriteFile(path, []byte("# comment\nDB_PASS=a=b#c\nEMPTY=\n"), 0644); err != nil {
		t.Fatal(err)
	}
	warned := false
	values, err := ReadSecrets(path, func(string, ...any) { warned = true })
	if err != nil {
		t.Fatal(err)
	}
	if values["DB_PASS"] != "a=b#c" || values["EMPTY"] != "" {
		t.Fatalf("unexpected values: %#v", values)
	}
	if !warned {
		t.Fatal("expected warning for permissive file mode")
	}
	if err := os.WriteFile(path, []byte("INVALID ENTRY\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecrets(path, nil); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("unexpected malformed-file error: %v", err)
	}
}

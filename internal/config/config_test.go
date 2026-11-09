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
	if loaded.Jobs[0].Schedule != DefaultSchedule() {
		t.Fatalf("missing schedule = %#v, want default %#v", loaded.Jobs[0].Schedule, DefaultSchedule())
	}
	if loaded.Jobs[0].GoogleDrivePath != "db1" {
		t.Fatalf("missing Google Drive path = %q, want job name db1", loaded.Jobs[0].GoogleDrivePath)
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
	c.Jobs = []Job{{Name: "db", Enabled: true, Schedule: DefaultSchedule(), GoogleDrivePath: "team/db", Driver: "postgres", Source: "host", Database: "app", Username: "backup", PasswordEnv: "DB_PASS", Host: "localhost", Port: 5432}}
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
	job := Job{Name: "db", Enabled: true, Driver: "sqlite", Source: "host", File: "/tmp/db.sqlite", Schedule: Schedule{Interval: "7d", HourAt: 20, MinuteAt: 30}, GoogleDrivePath: "team/db"}
	c.Jobs = []Job{job}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid schedule rejected: %v", err)
	}
	for _, interval := range []string{"1h", "2h", "1d", "3d", "7d"} {
		c.Jobs[0].Schedule.Interval = interval
		if err := c.Validate(); err != nil {
			t.Errorf("valid interval %q rejected: %v", interval, err)
		}
	}

	for _, schedule := range []Schedule{
		{Interval: "30m", HourAt: 2, MinuteAt: 0},
		{Interval: "0h", HourAt: 2, MinuteAt: 0},
		{Interval: "*/15 * * * *", HourAt: 2, MinuteAt: 0},
		{Interval: "1h", HourAt: 24, MinuteAt: 0},
		{Interval: "1h", HourAt: 2, MinuteAt: 10},
		{Interval: "1.5h", HourAt: 2, MinuteAt: 15},
		{Interval: "01h", HourAt: 2, MinuteAt: 15},
		{Interval: "+1h", HourAt: 2, MinuteAt: 15},
	} {
		c.Jobs[0].Schedule = schedule
		if err := c.Validate(); err == nil {
			t.Errorf("accepted invalid schedule %#v", schedule)
		}
	}

	c.Jobs[0].Schedule = Schedule{}
	if err := c.Validate(); err == nil {
		t.Fatal("accepted empty schedule before defaults are applied")
	}
}

func TestLoadDefaultsMissingJobScheduleToMidnight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("version: 2\njobs:\n  - name: sqlite\n    enabled: true\n    driver: sqlite\n    source: host\n    file: /tmp/db.sqlite\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Jobs[0].Schedule; got != DefaultSchedule() {
		t.Fatalf("missing schedule = %#v, want %#v", got, DefaultSchedule())
	}
	if got := c.Jobs[0].GoogleDrivePath; got != "sqlite" {
		t.Fatalf("missing Google Drive path = %q, want job name sqlite", got)
	}
	wantRcloneConfig := filepath.Join(filepath.Dir(path), "rclone.conf")
	if got := c.Storage.GoogleDrive.ConfigFile; got != wantRcloneConfig {
		t.Fatalf("default Rclone config path = %q, want %q", got, wantRcloneConfig)
	}
}

func TestLoadScheduleMapping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("version: 2\njobs:\n  - name: sqlite\n    enabled: true\n    schedule:\n      interval: 3d\n      hour_at: 20\n      minute_at: 30\n    driver: sqlite\n    source: host\n    file: /tmp/db.sqlite\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Schedule{Interval: "3d", HourAt: 20, MinuteAt: 30}
	if got := c.Jobs[0].Schedule; got != want {
		t.Fatalf("loaded schedule = %#v, want %#v", got, want)
	}
}

func TestLoadRejectsLegacyCronSchedule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("version: 1\njobs:\n  - name: sqlite\n    enabled: true\n    schedule: \"0 2 * * *\"\n    driver: sqlite\n    source: host\n    file: /tmp/db.sqlite\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "cron expressions are no longer supported") {
		t.Fatalf("Load() error = %v, want legacy schedule migration guidance", err)
	}
}

func TestAddJobDefaultsGoogleDrivePathToJobName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := Write(path, DefaultsFor(path)); err != nil {
		t.Fatal(err)
	}
	job := Job{Name: "archive-db", Driver: "sqlite", Source: "host", File: "/tmp/db.sqlite"}
	if err := AddJob(path, job); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Jobs[0].GoogleDrivePath; got != job.Name {
		t.Fatalf("added job Google Drive path = %q, want %q", got, job.Name)
	}
}

func TestGoogleDrivePathValidation(t *testing.T) {
	c := Defaults()
	c.Jobs = []Job{{Name: "db", Enabled: true, Schedule: DefaultSchedule(), Driver: "sqlite", Source: "host", File: "/tmp/db.sqlite", GoogleDrivePath: "team/databases/db"}}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid relative path rejected: %v", err)
	}
	for _, value := range []string{"", "/absolute", "../outside", "team/../outside", "team//db", `team\\db`, "team/"} {
		c.Jobs[0].GoogleDrivePath = value
		if err := c.Validate(); err == nil {
			t.Errorf("accepted unsafe Google Drive path %q", value)
		}
	}
	c.Jobs = []Job{
		{Name: "db1", Enabled: true, Schedule: DefaultSchedule(), Driver: "sqlite", Source: "host", File: "/tmp/db1.sqlite", GoogleDrivePath: "team/db"},
		{Name: "db2", Enabled: true, Schedule: DefaultSchedule(), Driver: "sqlite", Source: "host", File: "/tmp/db2.sqlite", GoogleDrivePath: "team/db"},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("accepted duplicate Google Drive paths")
	}
}

func TestLoadResolvesRelativeRcloneConfigPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("version: 2\nstorage:\n  google_drive:\n    config_file: auth/rclone.conf\n")
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
	if err := os.WriteFile(path, []byte("version: 2\nstorage:\n  unknown: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("accepted unknown YAML field")
	}
}

func TestLoadScheduledSkipsInvalidJobsAndRejectsInvalidSharedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := `version: 2
jobs:
  - name: good
    enabled: true
    driver: sqlite
    source: host
    file: /tmp/good.sqlite
  - name: broken
    enabled: true
    driver: unknown
    source: host
  - name: disabled-broken
    enabled: false
    driver: unknown
    source: invalid
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c, jobErrors, err := LoadScheduled(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobErrors) != 1 || len(c.Jobs) != 1 || c.Jobs[0].Name != "good" {
		t.Fatalf("scheduled jobs = %#v, errors = %#v", c.Jobs, jobErrors)
	}

	if err := os.WriteFile(path, []byte("version: 2\nstorage:\n  local:\n    retention_days: -1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadScheduled(path); err == nil {
		t.Fatal("invalid shared config was accepted")
	}
	if err := os.WriteFile(path, []byte("version: [invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadScheduled(path); err == nil {
		t.Fatal("malformed YAML was accepted")
	}
}

func TestLoadScheduledAcceptsConfigWithoutJobs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("version: 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, jobErrors, err := LoadScheduled(path)
	if err != nil || len(jobErrors) != 0 || len(c.Jobs) != 0 {
		t.Fatalf("LoadScheduled() = jobs %#v, job errors %#v, error %v", c.Jobs, jobErrors, err)
	}
}

func TestLoadScheduledSkipsAllValidJobsWithDuplicateDrivePaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := `version: 2
jobs:
  - name: one
    enabled: true
    driver: sqlite
    source: host
    file: /tmp/one.sqlite
    google_drive_path: team/shared
  - name: two
    enabled: true
    driver: sqlite
    source: host
    file: /tmp/two.sqlite
    google_drive_path: team/shared
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c, jobErrors, err := LoadScheduled(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Jobs) != 0 || len(jobErrors) != 2 {
		t.Fatalf("scheduled jobs = %#v, errors = %#v; want both conflicting jobs skipped", c.Jobs, jobErrors)
	}
}

func TestLoadScheduledSkipsDuplicateEnabledNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := `version: 2
jobs:
  - name: duplicate
    enabled: true
    driver: sqlite
    source: host
    file: /tmp/one.sqlite
    google_drive_path: one
  - name: duplicate
    enabled: true
    driver: sqlite
    source: host
    file: /tmp/two.sqlite
    google_drive_path: two
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c, jobErrors, err := LoadScheduled(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Jobs) != 0 || len(jobErrors) != 2 {
		t.Fatalf("scheduled jobs = %#v, errors = %#v; want both duplicate names skipped", c.Jobs, jobErrors)
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
	for _, job := range c.Jobs {
		if job.GoogleDrivePath == "" {
			t.Errorf("job %q has no Google Drive path", job.Name)
		}
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

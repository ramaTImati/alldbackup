package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	c.Jobs = []Job{{Name: "db", Enabled: true, Driver: "postgres", Source: "host", Database: "app", Username: "backup", PasswordEnv: "DB_PASS", Host: "localhost", Port: 5432}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Storage.Local.RetentionDays = -1
	if err := c.Validate(); err == nil {
		t.Fatal("accepted negative retention")
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

package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ramaTImati/alldbackup/internal/config"
	"github.com/ramaTImati/alldbackup/internal/database"
)

func TestDatabaseDumpRestore(t *testing.T) {
	if os.Getenv("ALLDBACKUP_INTEGRATION") != "1" {
		t.Skip("set ALLDBACKUP_INTEGRATION=1 with tests/integration/docker-compose.yml running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	dumper := database.New()
	work := t.TempDir()
	secrets := map[string]string{"MYSQL_PASSWORD": "integration", "MARIA_PASSWORD": "integration", "PG_PASSWORD": "integration"}
	mysqlCNF := writeMySQLConfig(t, work, 3306)
	mariaCNF := writeMySQLConfig(t, work, 3307)
	pgPass := writePGPass(t, work)

	t.Run("mysql host and Docker", func(t *testing.T) {
		for _, source := range []string{"host", "docker"} {
			sourceDB := "alldbackup_mysql_" + source
			targetDB := sourceDB + "_restore"
			seedMySQL(t, ctx, mysqlCNF, sourceDB)
			job := config.Job{Name: "mysql-" + source, Enabled: true, Driver: "mysql", Source: source, Host: "127.0.0.1", Port: 3306, Container: "alldbackup-mysql", Database: sourceDB, Username: "root", PasswordEnv: "MYSQL_PASSWORD"}
			restoreMySQL(t, ctx, dumper, job, secrets, mysqlCNF, sourceDB, targetDB)
		}
	})
	t.Run("MariaDB host and Docker", func(t *testing.T) {
		for _, source := range []string{"host", "docker"} {
			sourceDB := "alldbackup_maria_" + source
			targetDB := sourceDB + "_restore"
			seedMySQL(t, ctx, mariaCNF, sourceDB)
			job := config.Job{Name: "maria-" + source, Enabled: true, Driver: "mariadb", Source: source, Host: "127.0.0.1", Port: 3307, Container: "alldbackup-mariadb", Database: sourceDB, Username: "root", PasswordEnv: "MARIA_PASSWORD"}
			restoreMySQL(t, ctx, dumper, job, secrets, mariaCNF, sourceDB, targetDB)
		}
	})
	t.Run("PostgreSQL host and Docker", func(t *testing.T) {
		for _, source := range []string{"host", "docker"} {
			sourceDB := "alldbackup_pg_" + source
			targetDB := sourceDB + "_restore"
			seedPostgres(t, ctx, pgPass, sourceDB)
			job := config.Job{Name: "pg-" + source, Enabled: true, Driver: "postgres", Source: source, Host: "127.0.0.1", Port: 5432, Container: "alldbackup-postgres", Database: sourceDB, Username: "backup_user", PasswordEnv: "PG_PASSWORD"}
			restorePostgres(t, ctx, dumper, job, secrets, pgPass, targetDB)
		}
	})
	t.Run("SQLite host and Docker", func(t *testing.T) {
		hostDB := filepath.Join(work, "host.sqlite")
		if _, err := run(ctx, "sqlite3", []string{hostDB, "CREATE TABLE item(id INTEGER); INSERT INTO item VALUES (1);"}, nil, nil); err != nil {
			t.Fatal(err)
		}
		dockerDB := "/tmp/alldbackup-source.sqlite"
		if _, err := run(ctx, "docker", []string{"exec", "alldbackup-sqlite", "sqlite3", dockerDB, "CREATE TABLE item(id INTEGER); INSERT INTO item VALUES (1);"}, nil, nil); err != nil {
			t.Fatal(err)
		}
		for _, test := range []struct{ source, file string }{{"host", hostDB}, {"docker", dockerDB}} {
			job := config.Job{Name: "sqlite-" + test.source, Enabled: true, Driver: "sqlite", Source: test.source, Container: "alldbackup-sqlite", File: test.file}
			var dump bytes.Buffer
			if err := dumper.Check(ctx, job, nil); err != nil {
				t.Fatalf("%s check: %v", test.source, err)
			}
			if err := dumper.Dump(ctx, job, nil, &dump); err != nil {
				t.Fatalf("%s dump: %v", test.source, err)
			}
			restored := filepath.Join(work, "restored-"+test.source+".sqlite")
			if err := os.WriteFile(restored, dump.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			out, err := run(ctx, "sqlite3", []string{"-readonly", restored, "SELECT count(*) FROM item;"}, nil, nil)
			if err != nil || strings.TrimSpace(out) != "1" {
				t.Fatalf("%s restore count=%q, err=%v", test.source, out, err)
			}
		}
	})
}

func restoreMySQL(t *testing.T, ctx context.Context, dumper database.Dumper, job config.Job, secrets map[string]string, optionFile, sourceDB, targetDB string) {
	t.Helper()
	if err := dumper.Check(ctx, job, secrets); err != nil {
		t.Fatalf("%s check: %v", job.Source, err)
	}
	var dump bytes.Buffer
	if err := dumper.Dump(ctx, job, secrets, &dump); err != nil {
		t.Fatalf("%s dump: %v", job.Source, err)
	}
	restored := strings.ReplaceAll(dump.String(), sourceDB, targetDB)
	if _, err := run(ctx, "mysql", []string{"--defaults-extra-file=" + optionFile}, strings.NewReader(restored), nil); err != nil {
		t.Fatalf("%s restore: %v", job.Source, err)
	}
	out, err := run(ctx, "mysql", []string{"--defaults-extra-file=" + optionFile, "--batch", "--skip-column-names", "--execute=SELECT COUNT(*) FROM " + targetDB + ".item"}, nil, nil)
	if err != nil || strings.TrimSpace(out) != "1" {
		t.Fatalf("%s restore count=%q, err=%v", job.Source, out, err)
	}
}

func restorePostgres(t *testing.T, ctx context.Context, dumper database.Dumper, job config.Job, secrets map[string]string, passFile, targetDB string) {
	t.Helper()
	if err := dumper.Check(ctx, job, secrets); err != nil {
		t.Fatalf("%s check: %v", job.Source, err)
	}
	var dump bytes.Buffer
	if err := dumper.Dump(ctx, job, secrets, &dump); err != nil {
		t.Fatalf("%s dump: %v", job.Source, err)
	}
	env := []string{"PGPASSFILE=" + passFile}
	if _, err := run(ctx, "createdb", []string{"--host=127.0.0.1", "--username=backup_user", targetDB}, nil, env); err != nil {
		t.Fatalf("create restore database: %v", err)
	}
	if _, err := run(ctx, "psql", []string{"--host=127.0.0.1", "--username=backup_user", "--dbname=" + targetDB, "--set=ON_ERROR_STOP=1"}, bytes.NewReader(dump.Bytes()), env); err != nil {
		t.Fatalf("%s restore: %v", job.Source, err)
	}
	out, err := run(ctx, "psql", []string{"--host=127.0.0.1", "--username=backup_user", "--dbname=" + targetDB, "--tuples-only", "--no-align", "--command=SELECT count(*) FROM item"}, nil, env)
	if err != nil || strings.TrimSpace(out) != "1" {
		t.Fatalf("%s restore count=%q, err=%v", job.Source, out, err)
	}
}

func seedMySQL(t *testing.T, ctx context.Context, optionFile, db string) {
	t.Helper()
	sql := fmt.Sprintf("CREATE DATABASE %s; CREATE TABLE %s.item(id INT); INSERT INTO %s.item VALUES (1);", db, db, db)
	if _, err := run(ctx, "mysql", []string{"--defaults-extra-file=" + optionFile, "--execute=" + sql}, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func seedPostgres(t *testing.T, ctx context.Context, passFile, db string) {
	t.Helper()
	env := []string{"PGPASSFILE=" + passFile}
	if _, err := run(ctx, "createdb", []string{"--host=127.0.0.1", "--username=backup_user", db}, nil, env); err != nil {
		t.Fatal(err)
	}
	if _, err := run(ctx, "psql", []string{"--host=127.0.0.1", "--username=backup_user", "--dbname=" + db, "--set=ON_ERROR_STOP=1", "--command=CREATE TABLE item(id integer); INSERT INTO item VALUES (1);"}, nil, env); err != nil {
		t.Fatal(err)
	}
}

func writeMySQLConfig(t *testing.T, dir string, port int) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("mysql-%d.cnf", port))
	data := fmt.Sprintf("[client]\nuser=root\npassword=\"integration\"\nhost=127.0.0.1\nport=%d\n", port)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePGPass(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "pgpass")
	if err := os.WriteFile(path, []byte("127.0.0.1:5432:*:backup_user:integration\nlocalhost:5432:*:backup_user:integration\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func run(ctx context.Context, name string, args []string, stdin io.Reader, env []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("%s: %s: %w", name, strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}

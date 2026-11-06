package database

import (
	"context"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/ramaTImati/alldbackup/internal/config"
)

type commandRecorder struct{ calls []string }

func (r *commandRecorder) Run(_ context.Context, name string, args, _ []string, stdout io.Writer) error {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	if name == "docker" && strings.Contains(strings.Join(args, " "), "mysqldump") && stdout != nil {
		_, _ = io.WriteString(stdout, "-- SQL dump\n")
	}
	return nil
}
func (r *commandRecorder) Output(_ context.Context, name string, args, _ []string) (string, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	if strings.Contains(strings.Join(args, " "), "inspect") {
		return "running", nil
	}
	return "", nil
}

func TestDockerDumpDoesNotPutPasswordInCommandArguments(t *testing.T) {
	recorder := &commandRecorder{}
	dumper := &Native{Exec: recorder}
	job := config.Job{Name: "db", Driver: "mysql", Source: "docker", Container: "mysql-db", Database: "app", Username: "backup", PasswordEnv: "DB_PASSWORD"}
	var output strings.Builder
	if err := dumper.Dump(context.Background(), job, map[string]string{"DB_PASSWORD": "private-value"}, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "-- SQL dump\n" {
		t.Fatalf("unexpected dump output %q", output.String())
	}
	for _, call := range recorder.calls {
		if strings.Contains(call, "private-value") {
			t.Fatalf("password exposed in process arguments: %s", call)
		}
	}
	if !strings.Contains(strings.Join(recorder.calls, "\n"), "docker cp") {
		t.Fatal("temporary credential file was not copied into container")
	}
	if !strings.Contains(strings.Join(recorder.calls, "\n"), "rm -f") {
		t.Fatal("temporary credential file cleanup was not attempted")
	}
}

func TestMySQLCredentialFileIsPrivateAndQuoted(t *testing.T) {
	dumper := New()
	job := config.Job{Name: "db", Driver: "mysql", Source: "host", Host: "127.0.0.1", Port: 3306, Username: "backup user"}
	err := dumper.withMySQLFile(job, `p"\ss`, func(path, _ string) error {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if runtime.GOOS == "linux" && info.Mode().Perm() != 0600 {
			t.Fatalf("credential file mode = %o", info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), `password="p\"\\ss"`) {
			t.Fatalf("password was not safely quoted: %q", data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCommandEnvironmentReplacesAndFiltersDatabasePasswords(t *testing.T) {
	t.Setenv("PGPASSFILE", "/old/passfile")
	t.Setenv("PGPASSWORD", "inherited-password")
	t.Setenv("MYSQL_PWD", "mysql-password")
	env := commandEnvironment([]string{"PGPASSFILE=/tmp/private-passfile"})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "inherited-password") || strings.Contains(joined, "mysql-password") {
		t.Fatal("database password environment leaked to child")
	}
	if strings.Contains(joined, "PGPASSFILE=/old/passfile") || !strings.Contains(joined, "PGPASSFILE=/tmp/private-passfile") {
		t.Fatalf("PGPASSFILE override failed: %s", joined)
	}
}

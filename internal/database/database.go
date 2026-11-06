package database

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ramaTImati/alldbackup/internal/config"
)

type Dumper interface {
	Check(context.Context, config.Job, map[string]string) error
	Dump(context.Context, config.Job, map[string]string, io.Writer) error
}

type Executor interface {
	Run(context.Context, string, []string, []string, io.Writer) error
	Output(context.Context, string, []string, []string) (string, error)
}

type OSExecutor struct{}

func (OSExecutor) Run(ctx context.Context, name string, args, env []string, stdout io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = commandEnvironment(env)
	cmd.Stdout = stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return fmt.Errorf("%s: %s", filepath.Base(name), message)
		}
		return fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	return nil
}

func commandEnvironment(overrides []string) []string {
	result := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || strings.EqualFold(key, "MYSQL_PWD") || strings.EqualFold(key, "PGPASSWORD") {
			continue
		}
		replaced := false
		for _, override := range overrides {
			name, _, ok := strings.Cut(override, "=")
			if ok && strings.EqualFold(name, key) {
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, entry)
		}
	}
	result = append(result, overrides...)
	return result
}

func (OSExecutor) Output(ctx context.Context, name string, args, env []string) (string, error) {
	var out bytes.Buffer
	if err := (OSExecutor{}).Run(ctx, name, args, env, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

type Native struct{ Exec Executor }

func New() *Native { return &Native{Exec: OSExecutor{}} }

func (n *Native) Check(ctx context.Context, job config.Job, secrets map[string]string) error {
	if job.Source == "docker" {
		if err := n.checkContainer(ctx, job); err != nil {
			return err
		}
	}
	switch job.Driver {
	case "sqlite":
		return n.checkSQLite(ctx, job)
	case "mysql", "mariadb":
		return n.checkMySQL(ctx, job, secrets)
	case "postgres":
		return n.checkPostgres(ctx, job, secrets)
	default:
		return fmt.Errorf("unsupported database driver %q", job.Driver)
	}
}

func (n *Native) Dump(ctx context.Context, job config.Job, secrets map[string]string, dst io.Writer) error {
	if job.Source == "docker" {
		if err := n.checkContainer(ctx, job); err != nil {
			return err
		}
	}
	switch job.Driver {
	case "sqlite":
		return n.dumpSQLite(ctx, job, dst)
	case "mysql", "mariadb":
		return n.dumpMySQL(ctx, job, secrets, dst)
	case "postgres":
		return n.dumpPostgres(ctx, job, secrets, dst)
	default:
		return fmt.Errorf("unsupported database driver %q", job.Driver)
	}
}

func (n *Native) checkContainer(ctx context.Context, job config.Job) error {
	status, err := n.Exec.Output(ctx, "docker", []string{"inspect", "--format={{.State.Status}}", job.Container}, nil)
	if err != nil {
		return fmt.Errorf("inspect Docker container %q: %w", job.Container, err)
	}
	if status != "running" {
		return fmt.Errorf("Docker container %q is %s, expected running", job.Container, status)
	}
	return nil
}

func (n *Native) mysqlDumpBinary(ctx context.Context, job config.Job) (string, error) {
	preferred := mysqlDumpBinary(job.Driver)
	choices := []string{preferred}
	if preferred == "mysqldump" {
		choices = append(choices, "mariadb-dump")
	} else {
		choices = append(choices, "mysqldump")
	}
	for _, binary := range choices {
		if job.Source == "host" {
			if _, err := exec.LookPath(binary); err != nil {
				continue
			}
		} else if err := n.Exec.Run(ctx, "docker", []string{"exec", job.Container, binary, "--version"}, nil, io.Discard); err != nil {
			continue
		}
		return binary, nil
	}
	return "", errors.New("neither mysqldump nor mariadb-dump is available")
}

func (n *Native) checkSQLite(ctx context.Context, job config.Job) error {
	if job.Source == "docker" {
		out, err := n.Exec.Output(ctx, "docker", []string{"exec", job.Container, "sqlite3", "-readonly", job.File, "PRAGMA quick_check;"}, nil)
		if err != nil {
			return fmt.Errorf("check SQLite database: %w", err)
		}
		if out != "ok" {
			return fmt.Errorf("SQLite integrity check returned %q", out)
		}
		return nil
	}
	out, err := n.Exec.Output(ctx, "sqlite3", []string{"-readonly", job.File, "PRAGMA quick_check;"}, nil)
	if err != nil {
		return fmt.Errorf("check SQLite database: %w", err)
	}
	if out != "ok" {
		return fmt.Errorf("SQLite integrity check returned %q", out)
	}
	return nil
}

func (n *Native) checkMySQL(ctx context.Context, job config.Job, secrets map[string]string) error {
	password, err := secretFor(job, secrets)
	if err != nil {
		return err
	}
	return n.withMySQLFile(job, password, func(hostPath, containerPath string) error {
		binary, err := n.mysqlDumpBinary(ctx, job)
		if err != nil {
			return err
		}
		args := []string{"--defaults-extra-file=" + hostPath, "--no-data", "--single-transaction", "--quick", "--routines", "--events", "--triggers", "--databases", job.Database}
		if job.Source == "docker" {
			return n.withContainerFile(ctx, hostPath, job.Container, containerPath, func() error {
				args[0] = "--defaults-extra-file=" + containerPath
				return n.Exec.Run(ctx, "docker", append([]string{"exec", job.Container, binary}, args...), nil, io.Discard)
			})
		}
		return n.Exec.Run(ctx, binary, args, nil, io.Discard)
	})
}

func (n *Native) checkPostgres(ctx context.Context, job config.Job, secrets map[string]string) error {
	password, err := secretFor(job, secrets)
	if err != nil {
		return err
	}
	return n.withPGPass(job, password, func(hostPath, containerPath string) error {
		args := []string{"--no-password", "--schema-only", "--host=" + connectionHost(job), "--port=" + fmt.Sprint(connectionPort(job)), "--username=" + job.Username, "--dbname=" + job.Database}
		if job.Source == "docker" {
			return n.withContainerFile(ctx, hostPath, job.Container, containerPath, func() error {
				return n.Exec.Run(ctx, "docker", append([]string{"exec", "-e", "PGPASSFILE=" + containerPath, job.Container, "pg_dump"}, args...), nil, io.Discard)
			})
		}
		if err := n.Exec.Run(ctx, "pg_dump", []string{"--version"}, nil, io.Discard); err != nil {
			return fmt.Errorf("pg_dump is required: %w", err)
		}
		if err := n.Exec.Run(ctx, "pg_dump", args, []string{"PGPASSFILE=" + hostPath}, io.Discard); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "server version") {
				return fmt.Errorf("pg_dump/server version compatibility check failed: %w", err)
			}
			return err
		}
		return nil
	})
}

func (n *Native) dumpMySQL(ctx context.Context, job config.Job, secrets map[string]string, dst io.Writer) error {
	password, err := secretFor(job, secrets)
	if err != nil {
		return err
	}
	return n.withMySQLFile(job, password, func(hostPath, containerPath string) error {
		binary, err := n.mysqlDumpBinary(ctx, job)
		if err != nil {
			return err
		}
		args := []string{"--defaults-extra-file=" + hostPath, "--single-transaction", "--quick", "--routines", "--events", "--triggers", "--databases", job.Database}
		if job.Source == "docker" {
			return n.withContainerFile(ctx, hostPath, job.Container, containerPath, func() error {
				args[0] = "--defaults-extra-file=" + containerPath
				return n.Exec.Run(ctx, "docker", append([]string{"exec", job.Container, binary}, args...), nil, dst)
			})
		}
		return n.Exec.Run(ctx, binary, args, nil, dst)
	})
}

func (n *Native) dumpPostgres(ctx context.Context, job config.Job, secrets map[string]string, dst io.Writer) error {
	password, err := secretFor(job, secrets)
	if err != nil {
		return err
	}
	return n.withPGPass(job, password, func(hostPath, containerPath string) error {
		args := []string{"--no-password", "--format=plain", "--host=" + connectionHost(job), "--port=" + fmt.Sprint(connectionPort(job)), "--username=" + job.Username, "--dbname=" + job.Database}
		if job.Source == "docker" {
			return n.withContainerFile(ctx, hostPath, job.Container, containerPath, func() error {
				return n.Exec.Run(ctx, "docker", append([]string{"exec", "-e", "PGPASSFILE=" + containerPath, job.Container, "pg_dump"}, args...), nil, dst)
			})
		}
		return n.Exec.Run(ctx, "pg_dump", args, []string{"PGPASSFILE=" + hostPath}, dst)
	})
}

func (n *Native) dumpSQLite(ctx context.Context, job config.Job, dst io.Writer) (runErr error) {
	tmp, err := os.CreateTemp("", "alldbackup-*.sqlite")
	if err != nil {
		return err
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(path)
		return err
	}
	defer os.Remove(path)
	if job.Source == "docker" {
		remote := "/tmp/alldbackup-" + filepath.Base(path)
		runErr = n.withContainerTemp(ctx, job.Container, remote, func() error {
			if err := n.Exec.Run(ctx, "docker", []string{"exec", job.Container, "sqlite3", "-readonly", job.File, ".backup \"" + remote + "\""}, nil, io.Discard); err != nil {
				return fmt.Errorf("create SQLite online backup: %w", err)
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			if err := n.Exec.Run(ctx, "docker", []string{"cp", job.Container + ":" + remote, path}, nil, io.Discard); err != nil {
				return fmt.Errorf("copy SQLite backup from container: %w", err)
			}
			return nil
		})
		if runErr != nil {
			return runErr
		}
	} else {
		command := ".backup \"" + path + "\""
		if err := n.Exec.Run(ctx, "sqlite3", []string{"-readonly", job.File, command}, nil, io.Discard); err != nil {
			return fmt.Errorf("create SQLite online backup: %w", err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open SQLite backup: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(dst, f); err != nil {
		return err
	}
	return nil
}

func (n *Native) withMySQLFile(job config.Job, password string, fn func(string, string) error) error {
	f, err := os.CreateTemp("", "alldbackup-mysql-*.cnf")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	content := "[client]\nuser=" + mysqlOption(job.Username) + "\npassword=" + mysqlOption(password) + "\n"
	if job.Source == "host" {
		content += "host=" + mysqlOption(job.Host) + "\nport=" + fmt.Sprint(job.Port) + "\n"
	}
	if _, err := io.WriteString(f, content); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return fn(path, "/tmp/"+filepath.Base(path))
}

func (n *Native) withPGPass(job config.Job, password string, fn func(string, string) error) error {
	f, err := os.CreateTemp("", "alldbackup-pgpass-*")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	line := strings.Join([]string{pgPassEscape(connectionHost(job)), fmt.Sprint(connectionPort(job)), pgPassEscape(job.Database), pgPassEscape(job.Username), pgPassEscape(password)}, ":") + "\n"
	if _, err := io.WriteString(f, line); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return fn(path, "/tmp/"+filepath.Base(path))
}

func (n *Native) copyToContainer(ctx context.Context, hostPath, container, containerPath string) error {
	if err := n.Exec.Run(ctx, "docker", []string{"cp", hostPath, container + ":" + containerPath}, nil, io.Discard); err != nil {
		return fmt.Errorf("copy temporary credentials into container: %w", err)
	}
	return nil
}

func (n *Native) withContainerFile(ctx context.Context, hostPath, container, remote string, run func() error) (runErr error) {
	defer func() {
		if err := n.removeFromContainer(container, remote); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}()
	if err := n.copyToContainer(ctx, hostPath, container, remote); err != nil {
		return err
	}
	return run()
}

func (n *Native) withContainerTemp(ctx context.Context, container, remote string, run func() error) (runErr error) {
	defer func() {
		if err := n.removeFromContainer(container, remote); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}()
	return run()
}

func (n *Native) removeFromContainer(container, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := n.Exec.Run(ctx, "docker", []string{"exec", container, "rm", "-f", path}, nil, io.Discard); err != nil {
		return fmt.Errorf("remove temporary file %q from container %q: %w", path, container, err)
	}
	return nil
}

func secretFor(job config.Job, secrets map[string]string) (string, error) {
	value, ok := os.LookupEnv(job.PasswordEnv)
	if ok {
		return value, nil
	}
	value, ok = secrets[job.PasswordEnv]
	if !ok {
		return "", fmt.Errorf("password environment variable %q is not set", job.PasswordEnv)
	}
	return value, nil
}

func mysqlOption(value string) string {
	return "\"" + strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r", "\"", "\\\"").Replace(value) + "\""
}
func pgPassEscape(value string) string {
	return strings.NewReplacer("\\", "\\\\", ":", "\\:", "\n", "\\n", "\r", "\\r").Replace(value)
}
func mysqlDumpBinary(driver string) string {
	if driver == "mariadb" {
		return "mariadb-dump"
	}
	return "mysqldump"
}
func connectionHost(job config.Job) string {
	if job.Source == "docker" {
		return "localhost"
	}
	return job.Host
}
func connectionPort(job config.Job) int {
	if job.Source == "docker" && job.Port == 0 {
		return 5432
	}
	return job.Port
}

var _ Dumper = (*Native)(nil)

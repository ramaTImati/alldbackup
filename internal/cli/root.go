package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ramaTImati/alldbackup/internal/archive"
	"github.com/ramaTImati/alldbackup/internal/backup"
	"github.com/ramaTImati/alldbackup/internal/config"
	"github.com/ramaTImati/alldbackup/internal/database"
	"github.com/ramaTImati/alldbackup/internal/storage"
	"github.com/spf13/cobra"
)

var Version = "dev"

type options struct {
	configPath string
	verbose    bool
	quiet      bool
	logger     *slog.Logger
}

func Execute() error {
	o := &options{}
	root := &cobra.Command{
		Use:           "alldbackup",
		Short:         "Back up databases to local storage and Google Drive",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, args []string) error { return cmd.Help() },
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if o.verbose && o.quiet {
				return errors.New("--verbose and --quiet cannot be used together")
			}
			level := slog.LevelInfo
			if o.verbose {
				level = slog.LevelDebug
			}
			if o.quiet {
				level = slog.LevelError
			}
			o.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
			return nil
		},
	}
	root.PersistentFlags().StringVar(&o.configPath, "config", config.DefaultPath, "configuration file")
	root.PersistentFlags().BoolVar(&o.verbose, "verbose", false, "enable debug logging")
	root.PersistentFlags().BoolVar(&o.quiet, "quiet", false, "show errors only")
	root.AddCommand(initCommand(o), configCommand(o), runCommand(o), cronCommand(o), doctorCommand(o), driveCommand(o), verifyCommand(), versionCommand())
	return root.Execute()
}

func load(o *options) (config.Config, error) {
	c, err := config.Load(o.configPath)
	if err != nil {
		return c, err
	}
	if info, err := os.Stat(o.configPath); err == nil && info.Mode().Perm()&0077 != 0 {
		o.logger.Warn("configuration file permissions are broader than 0600", "path", o.configPath, "permissions", info.Mode().Perm().String())
	}
	return c, nil
}

func initCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "init", Short: "Create a starter configuration", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(o.configPath); err == nil {
			return fmt.Errorf("configuration already exists at %s; edit it or pass --config to choose another path", o.configPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		c := config.DefaultsFor(o.configPath)
		if err := os.MkdirAll(c.Storage.Local.Path, 0700); err != nil {
			return fmt.Errorf("create local backup directory: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(c.SecretsFile), 0700); err != nil {
			return fmt.Errorf("create secrets directory: %w", err)
		}
		secret, err := os.OpenFile(c.SecretsFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, _ = secret.WriteString("# Add NAME=value entries referenced by password_env.\n")
			if err := secret.Close(); err != nil {
				return err
			}
		}
		if err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create secrets file: %w", err)
		}
		if err := config.Write(o.configPath, c); err != nil {
			return fmt.Errorf("create configuration: %w", err)
		}
		o.logger.Info("configuration initialized", "config", o.configPath, "secrets", c.SecretsFile, "storage", c.Storage.Local.Path)
		return nil
	}}
}

func configCommand(o *options) *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Manage backup configuration"}
	command.AddCommand(
		&cobra.Command{Use: "validate", Short: "Validate the configuration", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := load(o); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Configuration is valid")
			return nil
		}},
		&cobra.Command{Use: "list", Short: "List backup jobs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			c, err := load(o)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "NAME\tENABLED\tSCHEDULE\tDRIVER\tSOURCE\tDATABASE")
			for _, job := range c.Jobs {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%t\t%s\t%s\t%s\t%s\n", job.Name, job.Enabled, job.Schedule, job.Driver, job.Source, job.Database)
			}
			return nil
		}},
		addJobCommand(o),
	)
	return command
}

func addJobCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "add", Short: "Interactively add a backup job", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if info, err := os.Stdin.Stat(); err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return errors.New("config add requires an interactive terminal")
		}
		c, err := load(o)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if errors.Is(err, os.ErrNotExist) {
			c = config.DefaultsFor(o.configPath)
			if err := config.Write(o.configPath, c); err != nil {
				return err
			}
		}
		reader := bufio.NewReader(os.Stdin)
		var job config.Job
		if job.Name, err = prompt(reader, "Job name (lowercase letters, digits, _ and -)", ""); err != nil {
			return err
		}
		if job.Driver, err = prompt(reader, "Driver (mysql/mariadb/postgres/sqlite)", "mysql"); err != nil {
			return err
		}
		if job.Source, err = prompt(reader, "Source (host/docker)", "host"); err != nil {
			return err
		}
		if job.Driver == "sqlite" {
			if job.Source == "docker" {
				if job.Container, err = prompt(reader, "Docker container", ""); err != nil {
					return err
				}
			}
			if job.File, err = prompt(reader, "SQLite file path", ""); err != nil {
				return err
			}
		} else {
			if job.Database, err = prompt(reader, "Database name", ""); err != nil {
				return err
			}
			if job.Username, err = prompt(reader, "Database username", ""); err != nil {
				return err
			}
			if job.PasswordEnv, err = prompt(reader, "Password environment variable", ""); err != nil {
				return err
			}
			if job.Source == "docker" {
				if job.Container, err = prompt(reader, "Docker container", ""); err != nil {
					return err
				}
			} else {
				if job.Host, err = prompt(reader, "Database host", "127.0.0.1"); err != nil {
					return err
				}
				defaultPort := "3306"
				if job.Driver == "postgres" {
					defaultPort = "5432"
				}
				port, e := prompt(reader, "Database port", defaultPort)
				if e != nil {
					return e
				}
				if job.Port, err = strconv.Atoi(port); err != nil {
					return fmt.Errorf("invalid port %q", port)
				}
			}
		}
		if job.Schedule, err = prompt(reader, "Schedule (5-field cron)", config.DefaultJobSchedule); err != nil {
			return err
		}
		job.Enabled = true
		if err := config.AddJob(o.configPath, job); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Added job %s\n", job.Name)
		return nil
	}}
}

func prompt(reader *bufio.Reader, label, fallback string) (string, error) {
	if fallback != "" {
		fmt.Fprintf(os.Stdout, "%s [%s]: ", label, fallback)
	} else {
		fmt.Fprintf(os.Stdout, "%s: ", label)
	}
	value, err := reader.ReadString('\n')
	if err != nil && len(value) == 0 {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	return value, nil
}

func runCommand(o *options) *cobra.Command {
	var all bool
	var selected string
	command := &cobra.Command{Use: "run", Short: "Run database backups", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if all == (selected != "") {
			return errors.New("choose exactly one of --all or --job")
		}
		c, err := load(o)
		if err != nil {
			return err
		}
		var jobs []config.Job
		if all {
			jobs = c.EnabledJobs()
		} else {
			job, ok := c.FindJob(selected)
			if !ok || !job.Enabled {
				return fmt.Errorf("enabled job %q was not found", selected)
			}
			jobs = []config.Job{job}
		}
		if len(jobs) == 0 {
			return errors.New("no enabled backup jobs are configured")
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		drive := storage.NewRclone(c.Storage.GoogleDrive)
		runner := backup.Runner{Config: c, Dumper: database.New(), Uploader: drive, Logger: o.logger, Version: Version}
		return runner.Run(ctx, jobs)
	}}
	command.Flags().BoolVar(&all, "all", false, "run every enabled job")
	command.Flags().StringVar(&selected, "job", "", "run one named job")
	return command
}

func cronCommand(o *options) *cobra.Command {
	command := &cobra.Command{Use: "cron", Short: "Generate crontab entries from job schedules"}
	command.AddCommand(&cobra.Command{Use: "print", Short: "Print entries for enabled jobs with a schedule", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, err := load(o)
		if err != nil {
			return err
		}
		binary, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve alldbackup executable: %w", err)
		}
		binary, err = filepath.Abs(binary)
		if err != nil {
			return err
		}
		configPath, err := filepath.Abs(o.configPath)
		if err != nil {
			return err
		}
		lines := cronEntries(c, binary, configPath)
		if len(lines) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "# No enabled jobs have a schedule configured.")
			return nil
		}
		for _, line := range lines {
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
		return nil
	}})
	return command
}

func cronEntries(c config.Config, binary, configPath string) []string {
	lines := make([]string, 0, len(c.Jobs))
	for _, job := range c.Jobs {
		if !job.Enabled {
			continue
		}
		schedule := job.Schedule
		if strings.TrimSpace(schedule) == "" {
			schedule = config.DefaultJobSchedule
		}
		fields := strings.Fields(schedule)
		lines = append(lines, fmt.Sprintf("%s %s run --job %s --config %s", strings.Join(fields, " "), shellQuote(binary), shellQuote(job.Name), shellQuote(configPath)))
	}
	return lines
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func doctorCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Check configured dependencies and connectivity", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, err := load(o)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(c.Storage.Local.Path, 0700); err != nil {
			return fmt.Errorf("local storage is not writable: %w", err)
		}
		test, err := os.CreateTemp(c.Storage.Local.Path, ".alldbackup-doctor-*")
		if err != nil {
			return fmt.Errorf("local storage is not writable: %w", err)
		}
		_ = test.Close()
		_ = os.Remove(test.Name())
		secrets, err := config.ReadSecrets(c.SecretsFile, o.logger.Warn)
		if err != nil {
			return err
		}
		dumper := database.New()
		for _, job := range c.EnabledJobs() {
			if err := dumper.Check(cmd.Context(), job, secrets); err != nil {
				return fmt.Errorf("job %s: %w", job.Name, redactCLI(err, job, secrets))
			}
			o.logger.Info("database dependency and connectivity ok", "job", job.Name)
		}
		if c.Storage.GoogleDrive.Enabled {
			if err := storage.NewRclone(c.Storage.GoogleDrive).Test(cmd.Context()); err != nil {
				return err
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), "All configured checks passed")
		return nil
	}}
}

func driveCommand(o *options) *cobra.Command {
	command := &cobra.Command{Use: "drive", Short: "Set up or check Rclone storage"}
	command.AddCommand(
		&cobra.Command{Use: "setup", Short: "Open Rclone's interactive configuration", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := exec.LookPath("rclone"); err != nil {
				return errors.New("rclone is not installed; install it from https://rclone.org/install/")
			}
			process := exec.CommandContext(cmd.Context(), "rclone", "config")
			process.Stdin, process.Stdout, process.Stderr = os.Stdin, os.Stdout, os.Stderr
			return process.Run()
		}},
		&cobra.Command{Use: "test", Short: "Test Google Drive remote access", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			c, err := load(o)
			if err != nil {
				return err
			}
			if !c.Storage.GoogleDrive.Enabled {
				return errors.New("Google Drive storage is not enabled in the configuration")
			}
			return storage.NewRclone(c.Storage.GoogleDrive).Test(cmd.Context())
		}},
	)
	return command
}

func verifyCommand() *cobra.Command {
	var path string
	command := &cobra.Command{Use: "verify", Short: "Verify a backup ZIP and its checksum", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if path == "" {
			return errors.New("--file is required")
		}
		metadata, err := archive.Verify(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Verified %s (%s, job %s, timestamp %s)\n", path, metadata.Driver, metadata.JobName, metadata.TimestampUTC.UTC().Format(time.RFC3339))
		return nil
	}}
	command.Flags().StringVar(&path, "file", "", "backup ZIP path")
	return command
}

func versionCommand() *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print version", Args: cobra.NoArgs, Run: func(cmd *cobra.Command, args []string) { fmt.Fprintf(cmd.OutOrStdout(), "alldbackup %s\n", Version) }}
}

func redactCLI(err error, job config.Job, secrets map[string]string) error {
	message := err.Error()
	if value, ok := os.LookupEnv(job.PasswordEnv); ok && value != "" {
		message = strings.ReplaceAll(message, value, "[redacted]")
	}
	for _, value := range secrets {
		if value != "" {
			message = strings.ReplaceAll(message, value, "[redacted]")
		}
	}
	return errors.New(message)
}

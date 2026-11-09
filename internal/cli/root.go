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

	cronlib "github.com/robfig/cron/v3"
	"github.com/ramaTImati/alldbackup/internal/archive"
	"github.com/ramaTImati/alldbackup/internal/backup"
	"github.com/ramaTImati/alldbackup/internal/config"
	"github.com/ramaTImati/alldbackup/internal/database"
	"github.com/ramaTImati/alldbackup/internal/storage"
	"github.com/spf13/cobra"
)

var Version = "dev"

const banner = "        ,--.,--.   ,--.,--.                 ,--.\n" +
	" ,--,--.|  ||  | ,-|  ||  |-.  ,--,--. ,---.|  |,-.,--.,--. ,---.\n" +
	"' ,-.  ||  ||  |' .-. || .-. '' ,-.  || .--'|     /|  ||  || .-. |\n" +
	"\\ '-'  ||  ||  |\\ `-' || `-' |\\ '-'  |\\ `--.|  \\  \\'  ''  '| '-' '\n" +
	" \x60--\x60--'\x60--'\x60--' \x60---'  \x60---'  \x60--\x60--' \x60---'\x60--'\x60--'\x60----' |  |-'\n" +
	"                                                           \x60--'"

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
	root.SetHelpTemplate(banner + "\n\n" + root.HelpTemplate())
	root.SetVersionTemplate(banner + "\n\n{{.Name}} {{.Version}}\n")
	root.PersistentFlags().StringVar(&o.configPath, "config", config.DefaultPath, "configuration file")
	root.PersistentFlags().BoolVar(&o.verbose, "verbose", false, "enable debug logging")
	root.PersistentFlags().BoolVar(&o.quiet, "quiet", false, "show errors only")
	root.AddCommand(initCommand(o), configCommand(o), runCommand(o), cleanCommand(o), cronCommand(o), doctorCommand(o), driveCommand(o), verifyCommand(), versionCommand())
	return root.Execute()
}

func load(o *options) (config.Config, error) {
	c, err := config.Load(o.configPath)
	if err != nil {
		return c, err
	}
	if info, err := os.Stat(o.configPath); err == nil && info.Mode().Perm()&0022 != 0 {
		o.logger.Warn("configuration file is writable by its group or other users", "path", o.configPath, "permissions", info.Mode().Perm().String())
	}
	if info, err := os.Stat(c.Storage.GoogleDrive.ConfigFile); err == nil && info.Mode().Perm()&0077 != 0 {
		o.logger.Warn("Rclone config file permissions are broader than 0600", "path", c.Storage.GoogleDrive.ConfigFile, "permissions", info.Mode().Perm().String())
	}
	return c, nil
}

func prepareRcloneConfig(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("Rclone config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create Rclone config directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return fmt.Errorf("inspect Rclone config file: %w", statErr)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Rclone config path %q must be a regular file", path)
		}
		if info.Mode().Perm()&0077 != 0 {
			if err := os.Chmod(path, 0600); err != nil {
				return fmt.Errorf("restrict Rclone config file permissions: %w", err)
			}
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("create Rclone config file: %w", err)
	}
	return f.Close()
}

func initCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "init", Short: "Create a starter configuration", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(o.configPath); err == nil {
			return fmt.Errorf("configuration already exists at %s; edit it or pass --config to choose another path", o.configPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		c := config.DefaultsFor(o.configPath)
		if err := os.MkdirAll(c.Storage.Local.Path, 0755); err != nil {
			return fmt.Errorf("create local backup directory: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(c.SecretsFile), 0700); err != nil {
			return fmt.Errorf("create secrets directory: %w", err)
		}
		if err := prepareRcloneConfig(c.Storage.GoogleDrive.ConfigFile); err != nil {
			return err
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
		o.logger.Info("configuration initialized", "config", o.configPath, "secrets", c.SecretsFile, "rclone_config", c.Storage.GoogleDrive.ConfigFile, "storage", c.Storage.Local.Path)
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
			fmt.Fprintln(cmd.OutOrStdout(), "NAME\tENABLED\tSCHEDULE\tGOOGLE_DRIVE_PATH\tDRIVER\tSOURCE\tDATABASE")
			for _, job := range c.Jobs {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%t\t%s\t%s\t%s\t%s\t%s\n", job.Name, job.Enabled, job.Schedule, job.GoogleDrivePath, job.Driver, job.Source, job.Database)
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
		if job.GoogleDrivePath, err = prompt(reader, "Google Drive path (relative to storage.google_drive.remote)", job.Name); err != nil {
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

func cleanCommand(o *options) *cobra.Command {
	var jobName string
	var all bool
	command := &cobra.Command{Use: "clean --job NAME [BACKUP.zip | --all]", Short: "Remove a backup or all backups for a job from every enabled storage", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(jobName) == "" {
			return errors.New("--job is required")
		}
		if all == (len(args) == 1) {
			return errors.New("provide exactly one backup ZIP filename or --all")
		}
		filename := ""
		if !all {
			filename = args[0]
			if !archive.IsBackupName(filename) {
				return errors.New("backup filename must be an alldbackup ZIP basename")
			}
		}
		c, err := load(o)
		if err != nil {
			return err
		}
		job, ok := c.FindJob(jobName)
		if !ok {
			return fmt.Errorf("job %q was not found", jobName)
		}
		var failures []error
		deleted, err := storage.CleanLocal(c.Storage.Local.Path, job, filename)
		totalDeleted := deleted
		if deleted > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Cleaned %d local backup(s) for job %s\n", deleted, job.Name)
		} else if err == nil {
			if filename == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "No local backups found for job %s\n", job.Name)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "No local copy of %s found for job %s\n", filename, job.Name)
			}
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("local: %w", err))
		}
		if c.Storage.GoogleDrive.Enabled {
			drive := storage.NewRclone(c.Storage.GoogleDrive)
			deleted, err := drive.Clean(cmd.Context(), job, filename)
			totalDeleted += deleted
			if deleted > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Cleaned %d Google Drive backup(s) for job %s\n", deleted, job.Name)
			} else if err == nil {
				if filename == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "No Google Drive backups found for job %s\n", job.Name)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "No Google Drive copy of %s found for job %s\n", filename, job.Name)
				}
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("Google Drive: %w", err))
			}
		}
		if filename != "" && totalDeleted == 0 && len(failures) == 0 {
			failures = append(failures, fmt.Errorf("backup %q was not found in any active storage", filename))
		}
		return errors.Join(failures...)
	}}
	command.Flags().StringVar(&jobName, "job", "", "backup job name")
	command.Flags().BoolVar(&all, "all", false, "remove every backup for this job from all enabled storage")
	return command
}

func cronCommand(o *options) *cobra.Command {
	command := &cobra.Command{Use: "cron", Short: "Manage automatic scheduled backups"}
	command.AddCommand(&cobra.Command{Use: "print", Short: "Print the one-minute schedule dispatcher entry", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
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
		fmt.Fprintln(cmd.OutOrStdout(), cronEntry(binary, configPath))
		return nil
	}}, cronInstallCommand(o), cronRemoveCommand(o), cronTickCommand(o))
	return command
}

func cronEntry(binary, configPath string) string {
	return fmt.Sprintf("* * * * * %s --config %s cron tick", shellQuote(binary), shellQuote(configPath))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

const cronBlockStart = "# BEGIN ALLDBACKUP MANAGED"
const cronBlockEnd = "# END ALLDBACKUP MANAGED"

func cronInstallCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "install", Short: "Install the minute dispatcher in this account's crontab", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
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
		if err := updateCrontab(binary, configPath, true); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Installed alldbackup dispatcher in this account's crontab")
		return nil
	}}
}

func cronRemoveCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "remove", Short: "Remove the alldbackup dispatcher from this account's crontab", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
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
		if err := updateCrontab(binary, configPath, false); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Removed alldbackup dispatcher from this account's crontab")
		return nil
	}}
}

func updateCrontab(binary, configPath string, install bool) error {
	command := exec.Command("crontab", "-l")
	current, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || !strings.Contains(strings.ToLower(string(exitErr.Stderr)), "no crontab for") {
			return fmt.Errorf("read crontab: %w", err)
		}
		current = nil
	}
	updated, err := rewriteCrontab(string(current), binary, configPath, install)
	if err != nil {
		return err
	}
	if string(current) == updated {
		return nil
	}
	command = exec.Command("crontab", "-")
	command.Stdin = strings.NewReader(updated)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("write crontab: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func rewriteCrontab(current, binary, configPath string, install bool) (string, error) {
	var kept []string
	inBlock := false
	lines := strings.Split(strings.TrimRight(current, "\r\n"), "\n")
	if current == "" {
		lines = nil
	}
	for _, line := range lines {
		switch line {
		case cronBlockStart:
			if inBlock {
				return "", errors.New("nested alldbackup crontab block")
			}
			inBlock = true
			continue
		case cronBlockEnd:
			if !inBlock {
				return "", errors.New("alldbackup crontab block end without start")
			}
			inBlock = false
			continue
		}
		if inBlock || legacyCronEntry(line, binary, configPath) {
			continue
		}
		kept = append(kept, line)
	}
	if inBlock {
		return "", errors.New("unterminated alldbackup crontab block")
	}
	if install {
		kept = append(kept, cronBlockStart, cronEntry(binary, configPath), cronBlockEnd)
	}
	if len(kept) == 0 {
		return "", nil
	}
	return strings.Join(kept, "\n") + "\n", nil
}

func legacyCronEntry(line, binary, configPath string) bool {
	if !strings.Contains(line, shellQuote(binary)) || (!strings.Contains(line, " run --job ") && !strings.Contains(line, " run --all")) {
		return false
	}
	if strings.Contains(line, "--config") {
		return strings.Contains(line, shellQuote(configPath))
	}
	return filepath.Clean(configPath) == filepath.Clean(config.DefaultPath)
}

func cronTickCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "tick", Short: "Run jobs whose configured schedule matches this minute", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(o.configPath); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		c, jobErrors, err := config.LoadScheduled(o.configPath)
		if err != nil {
			return err
		}
		for _, jobErr := range jobErrors {
			o.logger.Error("skipping invalid scheduled job", "error", jobErr)
		}
		jobs := dueJobs(c.EnabledJobs(), time.Now())
		if len(jobs) == 0 {
			return nil
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		drive := storage.NewRclone(c.Storage.GoogleDrive)
		runner := backup.Runner{Config: c, Dumper: database.New(), Uploader: drive, Logger: o.logger, Version: Version}
		return runner.Run(ctx, jobs)
	}}
}

func dueJobs(jobs []config.Job, now time.Time) []config.Job {
	parser := cronlib.NewParser(cronlib.Minute | cronlib.Hour | cronlib.Dom | cronlib.Month | cronlib.Dow)
	minute := now.Truncate(time.Minute)
	previousMinute := minute.Add(-time.Minute)
	var due []config.Job
	for _, job := range jobs {
		schedule, err := parser.Parse(job.Schedule)
		if err != nil {
			continue
		}
		if schedule.Next(previousMinute).Equal(minute) {
			due = append(due, job)
		}
	}
	return due
}

func doctorCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Check configured dependencies and connectivity", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, err := load(o)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(c.Storage.Local.Path, 0755); err != nil {
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
			c, err := load(o)
			if err != nil {
				return err
			}
			if _, err := exec.LookPath("rclone"); err != nil {
				return errors.New("rclone is not installed; install it from https://rclone.org/install/")
			}
			if err := prepareRcloneConfig(c.Storage.GoogleDrive.ConfigFile); err != nil {
				return err
			}
			process := exec.CommandContext(cmd.Context(), "rclone", "config", "--config="+c.Storage.GoogleDrive.ConfigFile)
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
	return &cobra.Command{Use: "version", Short: "Print version", Args: cobra.NoArgs, Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), banner)
		fmt.Fprintf(cmd.OutOrStdout(), "alldbackup %s\n", Version)
	}}
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

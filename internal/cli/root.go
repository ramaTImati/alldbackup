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
	"github.com/ramaTImati/alldbackup/internal/scheduler"
	"github.com/ramaTImati/alldbackup/internal/storage"
	"github.com/ramaTImati/alldbackup/internal/updater"
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
	root.AddCommand(initCommand(o), configCommand(o), runCommand(o), cleanCommand(o), cronCommand(o), doctorCommand(o), driveCommand(o), verifyCommand(), updateCommand(), uninstallCommand(o), versionCommand())
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
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%t\t%s\t%s\t%s\t%s\t%s\n", job.Name, job.Enabled, job.Schedule.String(), job.GoogleDrivePath, job.Driver, job.Source, job.Database)
			}
			return nil
		}},
		addJobCommand(o),
		editJobCommand(o),
		deleteJobCommand(o),
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
		defaults := config.Job{Enabled: true, Driver: "mysql", Source: "host", Host: "127.0.0.1", Port: 3306, Schedule: config.DefaultSchedule()}
		job, err := promptJob(bufio.NewReader(os.Stdin), defaults)
		if err != nil {
			return err
		}
		if err := config.AddJob(o.configPath, job); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Added job %s\n", job.Name)
		return nil
	}}
}

func editJobCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "edit NAME", Short: "Interactively edit a backup job", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if info, err := os.Stdin.Stat(); err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return errors.New("config edit requires an interactive terminal")
		}
		c, err := load(o)
		if err != nil {
			return err
		}
		job, ok := c.FindJob(args[0])
		if !ok {
			return fmt.Errorf("job %q was not found", args[0])
		}
		reader := bufio.NewReader(os.Stdin)
		updated, err := promptJob(reader, job)
		if err != nil {
			return err
		}
		enabled, err := prompt(reader, "Enabled (true/false)", strconv.FormatBool(job.Enabled))
		if err != nil {
			return err
		}
		if updated.Enabled, err = strconv.ParseBool(enabled); err != nil {
			return fmt.Errorf("invalid enabled value %q", enabled)
		}
		if err := config.UpdateJob(o.configPath, job.Name, updated); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated job %s\n", updated.Name)
		return nil
	}}
}

func deleteJobCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "delete NAME", Short: "Remove a job from configuration (keeps its backups)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.DeleteJob(o.configPath, args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted job %s; existing backups were kept\n", args[0])
		return nil
	}}
}

func promptJob(reader *bufio.Reader, defaults config.Job) (config.Job, error) {
	var job config.Job
	var err error
	if job.Name, err = prompt(reader, "Job name (lowercase letters, digits, _ and -)", defaults.Name); err != nil {
		return job, err
	}
	if job.Driver, err = prompt(reader, "Driver (mysql/mariadb/postgres/sqlite)", defaults.Driver); err != nil {
		return job, err
	}
	if job.Source, err = prompt(reader, "Source (host/docker)", defaults.Source); err != nil {
		return job, err
	}
	if job.Driver == "sqlite" {
		if job.Source == "docker" {
			if job.Container, err = prompt(reader, "Docker container", defaults.Container); err != nil {
				return job, err
			}
		}
		if job.File, err = prompt(reader, "SQLite file path", defaults.File); err != nil {
			return job, err
		}
	} else {
		if job.Database, err = prompt(reader, "Database name", defaults.Database); err != nil {
			return job, err
		}
		if job.Username, err = prompt(reader, "Database username", defaults.Username); err != nil {
			return job, err
		}
		if job.PasswordEnv, err = prompt(reader, "Password environment variable", defaults.PasswordEnv); err != nil {
			return job, err
		}
		if job.Source == "docker" {
			if job.Container, err = prompt(reader, "Docker container", defaults.Container); err != nil {
				return job, err
			}
		} else {
			host := defaults.Host
			if host == "" || defaults.Source != "host" {
				host = "127.0.0.1"
			}
			if job.Host, err = prompt(reader, "Database host", host); err != nil {
				return job, err
			}
			port := defaults.Port
			if defaults.Driver != job.Driver || port == 0 {
				port = 3306
				if job.Driver == "postgres" {
					port = 5432
				}
			}
			value, promptErr := prompt(reader, "Database port", strconv.Itoa(port))
			if promptErr != nil {
				return job, promptErr
			}
			if job.Port, err = strconv.Atoi(value); err != nil {
				return job, fmt.Errorf("invalid port %q", value)
			}
		}
	}
	job.Schedule.Interval, err = prompt(reader, "Backup interval (whole hours or days, e.g. 1h, 1d, 7d)", defaults.Schedule.Interval)
	if err != nil {
		return job, err
	}
	hourAt, err := prompt(reader, "Hour at (0-23, server local time)", strconv.Itoa(defaults.Schedule.HourAt))
	if err != nil {
		return job, err
	}
	if job.Schedule.HourAt, err = strconv.Atoi(hourAt); err != nil {
		return job, fmt.Errorf("invalid schedule hour %q", hourAt)
	}
	minuteAt, err := prompt(reader, "Minute at (0, 15, 30, or 45)", strconv.Itoa(defaults.Schedule.MinuteAt))
	if err != nil {
		return job, err
	}
	if job.Schedule.MinuteAt, err = strconv.Atoi(minuteAt); err != nil {
		return job, fmt.Errorf("invalid schedule minute %q", minuteAt)
	}
	drivePath := defaults.GoogleDrivePath
	if drivePath == "" || (defaults.GoogleDrivePath == defaults.Name && job.Name != defaults.Name) {
		drivePath = job.Name
	}
	if job.GoogleDrivePath, err = prompt(reader, "Google Drive path (relative to storage.google_drive.remote)", drivePath); err != nil {
		return job, err
	}
	return job, nil
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
	command.AddCommand(&cobra.Command{Use: "print", Short: "Print the 15-minute schedule dispatcher entry", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
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
	return fmt.Sprintf("*/15 * * * * %s --config %s cron tick", shellQuote(binary), shellQuote(configPath))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

const cronBlockStart = "# BEGIN ALLDBACKUP MANAGED"
const cronBlockEnd = "# END ALLDBACKUP MANAGED"

func cronInstallCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "install", Short: "Install the 15-minute dispatcher in this account's crontab", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
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
	return &cobra.Command{Use: "tick", Short: "Run jobs due on this 15-minute tick", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		now := time.Now()
		if !isQuarterHour(now) {
			return nil
		}
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
		jobs := c.EnabledJobs()
		if len(jobs) == 0 {
			return nil
		}
		store := scheduler.New(c.Storage.Local.Path)
		unlock, locked, err := store.TryLock()
		if err != nil {
			return err
		}
		if !locked {
			return nil
		}
		defer unlock()
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		drive := storage.NewRclone(c.Storage.GoogleDrive)
		runner := backup.Runner{Config: c, Dumper: database.New(), Uploader: drive, Logger: o.logger, Version: Version}
		var failures []error
		for _, job := range jobs {
			attempt, due, err := store.Begin(job, now)
			if err != nil {
				failures = append(failures, fmt.Errorf("job %s scheduler state: %w", job.Name, err))
				continue
			}
			if !due {
				continue
			}
			err = runner.Run(ctx, []config.Job{job})
			if errors.Is(err, backup.ErrAlreadyRunning) {
				if stateErr := store.Abandon(job, attempt); stateErr != nil {
					failures = append(failures, fmt.Errorf("job %s scheduler state: %w", job.Name, stateErr))
				}
				continue
			}
			if err != nil {
				o.logger.Error("scheduled backup attempt failed", "job", job.Name, "attempt", attempt.Number, "error", err)
				if stateErr := store.Fail(job, attempt); stateErr != nil {
					failures = append(failures, fmt.Errorf("job %s scheduler state: %w", job.Name, stateErr))
				}
				failures = append(failures, err)
				continue
			}
			if err := store.Succeed(job, attempt); err != nil {
				failures = append(failures, fmt.Errorf("job %s scheduler state: %w", job.Name, err))
			}
		}
		return errors.Join(failures...)
	}}
}

func isQuarterHour(now time.Time) bool {
	return now.Minute()%15 == 0
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

func updateCommand() *cobra.Command {
	return &cobra.Command{Use: "update", Short: "Update the binary to the latest release", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		tag, updated, err := updater.Update(cmd.Context(), Version)
		if err != nil {
			return err
		}
		if updated {
			fmt.Fprintf(cmd.OutOrStdout(), "Updated alldbackup to %s\n", tag)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "alldbackup %s is already up to date\n", Version)
		}
		return nil
	}}
}

func uninstallCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "uninstall", Short: "Remove alldbackup and its configuration while keeping backups", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return errors.New("run uninstall with sudo")
		}
		if filepath.Clean(o.configPath) != filepath.Clean(config.DefaultPath) {
			return fmt.Errorf("uninstall only supports the default configuration at %s", config.DefaultPath)
		}

		configDir := filepath.Dir(config.DefaultPath)
		if info, err := os.Lstat(configDir); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("refusing to remove invalid configuration path %q", configDir)
			}
			if _, err := os.Stat(config.DefaultPath); err == nil {
				c, err := config.Load(config.DefaultPath)
				if err != nil {
					return fmt.Errorf("cannot safely uninstall with an invalid configuration: %w", err)
				}
				backupDir, err := filepath.Abs(c.Storage.Local.Path)
				if err != nil {
					return fmt.Errorf("resolve backup path: %w", err)
				}
				if resolved, err := filepath.EvalSymlinks(backupDir); err == nil {
					backupDir = resolved
				} else if !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("resolve backup path: %w", err)
				}
				configRoot, err := filepath.Abs(configDir)
				if err != nil {
					return fmt.Errorf("resolve configuration path: %w", err)
				}
				relative, err := filepath.Rel(configRoot, backupDir)
				if err != nil {
					return fmt.Errorf("compare backup and configuration paths: %w", err)
				}
				if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
					return errors.New("configured backup path is inside /etc/alldbackup; move backups before uninstalling to keep them")
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("inspect configuration file: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect configuration directory: %w", err)
		}

		binary, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve alldbackup executable: %w", err)
		}
		binary, err = filepath.Abs(binary)
		if err != nil {
			return err
		}
		configPath, err := filepath.Abs(config.DefaultPath)
		if err != nil {
			return err
		}
		if err := updateCrontab(binary, configPath, false); err != nil {
			return fmt.Errorf("remove alldbackup cron dispatcher: %w", err)
		}
		if err := os.RemoveAll(configDir); err != nil {
			return fmt.Errorf("remove configuration directory %q: %w", configDir, err)
		}
		if err := os.Remove(binary); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove executable %q: %w", binary, err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Uninstalled alldbackup; configuration and cron dispatcher were removed, local backups were kept")
		return nil
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

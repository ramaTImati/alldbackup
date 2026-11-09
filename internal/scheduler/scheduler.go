package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/ramaTImati/alldbackup/internal/config"
)

const (
	stateVersion = 1
	maxAttempts  = 3
	retryDelay   = 15 * time.Minute
)

type state struct {
	Version  int       `json:"version"`
	Schedule string    `json:"schedule"`
	NextRun  time.Time `json:"next_run"`
	CycleDue time.Time `json:"cycle_due,omitempty"`
	Attempts int       `json:"attempts"`
}

type Attempt struct {
	Number    int
	StartedAt time.Time
	previous  state
}

type Store struct {
	dir string
}

func New(root string) *Store {
	return &Store{dir: filepath.Join(root, ".scheduler")}
}

func (s *Store) TryLock() (func() error, bool, error) {
	if err := s.ensureDir(); err != nil {
		return nil, false, err
	}
	lock := flock.New(filepath.Join(s.dir, "dispatcher.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		_ = lock.Unlock()
		return nil, false, err
	}
	if !locked {
		_ = lock.Unlock()
		return nil, false, nil
	}
	return lock.Unlock, true, nil
}

func (s *Store) Begin(job config.Job, now time.Time) (*Attempt, bool, error) {
	if err := job.Schedule.Validate(); err != nil {
		return nil, false, err
	}
	if err := s.ensureDir(); err != nil {
		return nil, false, err
	}
	current, err := s.read(job.Name)
	if errors.Is(err, os.ErrNotExist) || (err == nil && current.Schedule != job.Schedule.String()) {
		current = state{Version: stateVersion, Schedule: job.Schedule.String(), NextRun: job.Schedule.FirstSlot(now)}
		err = nil
	}
	if err != nil {
		return nil, false, err
	}
	if current.Version != stateVersion || current.Schedule == "" || current.NextRun.IsZero() || current.Attempts < 0 || current.Attempts > maxAttempts {
		return nil, false, fmt.Errorf("invalid scheduler state for job %q", job.Name)
	}
	current.NextRun = current.NextRun.In(now.Location())
	if !current.CycleDue.IsZero() {
		current.CycleDue = current.CycleDue.In(now.Location())
	}
	if current.Attempts > 0 && current.CycleDue.IsZero() {
		return nil, false, fmt.Errorf("scheduler state for job %q is missing its due cycle", job.Name)
	}
	if current.Attempts == 0 && !current.CycleDue.IsZero() {
		return nil, false, fmt.Errorf("scheduler state for job %q has an unexpected due cycle", job.Name)
	}
	if current.Attempts == maxAttempts {
		current.NextRun = job.Schedule.AddInterval(current.CycleDue)
		current.CycleDue = time.Time{}
		current.Attempts = 0
	}
	if now.Before(current.NextRun) {
		if err := s.write(job.Name, current); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}

	tick := now.Truncate(retryDelay)
	if current.Attempts == 0 {
		current.CycleDue = current.NextRun
		if current.NextRun.Before(tick) {
			current.CycleDue = tick
		}
	}
	previous := current
	current.Attempts++
	current.NextRun = tick.Add(retryDelay)
	if err := s.write(job.Name, current); err != nil {
		return nil, false, err
	}
	return &Attempt{Number: current.Attempts, StartedAt: tick, previous: previous}, true, nil
}

func (s *Store) Succeed(job config.Job, attempt *Attempt) error {
	current, err := s.read(job.Name)
	if err != nil {
		return err
	}
	current.Schedule = job.Schedule.String()
	current.NextRun = job.Schedule.AddInterval(attempt.StartedAt)
	current.CycleDue = time.Time{}
	current.Attempts = 0
	return s.write(job.Name, current)
}

func (s *Store) Fail(job config.Job, attempt *Attempt) error {
	current, err := s.read(job.Name)
	if err != nil {
		return err
	}
	if current.Attempts < maxAttempts {
		current.NextRun = attempt.StartedAt.Add(retryDelay)
	} else {
		current.CycleDue = current.CycleDue.In(attempt.StartedAt.Location())
		current.NextRun = job.Schedule.AddInterval(current.CycleDue)
		current.CycleDue = time.Time{}
		current.Attempts = 0
	}
	return s.write(job.Name, current)
}

func (s *Store) Abandon(job config.Job, attempt *Attempt) error {
	return s.write(job.Name, attempt.previous)
}

func (s *Store) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return fmt.Errorf("create scheduler state directory: %w", err)
	}
	info, err := os.Lstat(s.dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("scheduler state path %q is not a directory", s.dir)
	}
	return os.Chmod(s.dir, 0700)
}

func (s *Store) read(jobName string) (state, error) {
	path := filepath.Join(s.dir, jobName+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return state{}, err
	}
	if !info.Mode().IsRegular() {
		return state{}, fmt.Errorf("scheduler state %q is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return state{}, err
	}
	defer file.Close()
	var current state
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&current); err != nil {
		return state{}, fmt.Errorf("read scheduler state for job %q: %w", jobName, err)
	}
	return current, nil
}

func (s *Store) write(jobName string, current state) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	path := filepath.Join(s.dir, jobName+".json")
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("scheduler state %q is not a regular file", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp, err := os.CreateTemp(s.dir, ".state-*.partial")
	if err != nil {
		return fmt.Errorf("create scheduler state temporary file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if err := json.NewEncoder(temp).Encode(current); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace scheduler state for job %q: %w", jobName, err)
	}
	return nil
}

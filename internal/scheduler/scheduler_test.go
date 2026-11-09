package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ramaTImati/alldbackup/internal/config"
)

func TestFirstSlotAndStateSurviveStoreRestart(t *testing.T) {
	loc := time.FixedZone("server", 7*60*60)
	job := scheduledJob("1d", 23, 0)
	now := time.Date(2026, time.November, 9, 22, 59, 0, 0, loc)
	root := t.TempDir()
	store := New(root)

	if _, due, err := store.Begin(job, now); err != nil || due {
		t.Fatalf("Begin before first slot = due %t, error %v; want not due", due, err)
	}
	restarted := New(root)
	attempt, due, err := restarted.Begin(job, time.Date(2026, time.November, 9, 23, 0, 5, 0, loc))
	if err != nil || !due || attempt.Number != 1 {
		t.Fatalf("Begin at first slot = attempt %#v, due %t, error %v", attempt, due, err)
	}

	if err := restarted.Succeed(job, attempt); err != nil {
		t.Fatal(err)
	}
	state, err := restarted.read(job.Name)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.November, 10, 23, 0, 0, 0, loc)
	if !state.NextRun.Equal(want) {
		t.Fatalf("next run = %s, want %s", state.NextRun, want)
	}
}

func TestFirstSlotChoosesNextConfiguredServerLocalTime(t *testing.T) {
	loc := time.FixedZone("server", 7*60*60)
	schedule := config.Schedule{Interval: "1h", HourAt: 20, MinuteAt: 30}
	for _, test := range []struct {
		now  time.Time
		want time.Time
	}{
		{time.Date(2026, 11, 9, 20, 29, 59, 0, loc), time.Date(2026, 11, 9, 20, 30, 0, 0, loc)},
		{time.Date(2026, 11, 9, 20, 30, 42, 0, loc), time.Date(2026, 11, 9, 20, 30, 0, 0, loc)},
		{time.Date(2026, 11, 9, 20, 31, 0, 0, loc), time.Date(2026, 11, 10, 20, 30, 0, 0, loc)},
	} {
		if got := schedule.FirstSlot(test.now); !got.Equal(test.want) {
			t.Errorf("FirstSlot(%s) = %s, want %s", test.now, got, test.want)
		}
	}
}

func TestHourlyIntervalStartsFromSuccessfulSlot(t *testing.T) {
	loc := time.FixedZone("server", 7*60*60)
	job := scheduledJob("1h", 8, 15)
	store := New(t.TempDir())
	now := time.Date(2026, 11, 9, 8, 15, 0, 0, loc)
	attempt, due, err := store.Begin(job, now)
	if err != nil || !due {
		t.Fatalf("Begin = due %t, error %v", due, err)
	}
	if err := store.Succeed(job, attempt); err != nil {
		t.Fatal(err)
	}
	state, err := store.read(job.Name)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 9, 9, 15, 0, 0, loc)
	if !state.NextRun.Equal(want) {
		t.Fatalf("hourly next run = %s, want %s", state.NextRun, want)
	}
}

func TestThreeTotalAttemptsThenWaitForNextInterval(t *testing.T) {
	loc := time.FixedZone("server", 7*60*60)
	job := scheduledJob("3d", 20, 30)
	root := t.TempDir()
	store := New(root)
	start := time.Date(2026, time.November, 9, 20, 30, 0, 0, loc)
	var third Attempt

	for n := 1; n <= maxAttempts; n++ {
		tick := start.Add(time.Duration(n-1) * retryDelay)
		attempt, due, err := store.Begin(job, tick)
		if err != nil || !due || attempt.Number != n {
			t.Fatalf("attempt %d = %#v, due %t, error %v", n, attempt, due, err)
		}
		if n == 1 {
			// Reopening the store emulates a cron process restart between retries.
			store = New(root)
		}
		if err := store.Fail(job, attempt); err != nil {
			t.Fatal(err)
		}
		if n == maxAttempts {
			third = *attempt
		}
	}

	state, err := store.read(job.Name)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.November, 12, 20, 30, 0, 0, loc)
	if state.Attempts != 0 || !state.NextRun.Equal(want) {
		t.Fatalf("post-retry state = %#v, want next run %s and no pending attempts", state, want)
	}
	if _, due, err := store.Begin(job, third.StartedAt.Add(retryDelay)); err != nil || due {
		t.Fatalf("Begin before next interval = due %t, error %v", due, err)
	}
}

func TestScheduleChangeResetsFirstSlot(t *testing.T) {
	loc := time.FixedZone("server", 7*60*60)
	root := t.TempDir()
	store := New(root)
	now := time.Date(2026, time.November, 9, 20, 30, 0, 0, loc)
	oldJob := scheduledJob("1d", 20, 30)
	attempt, due, err := store.Begin(oldJob, now)
	if err != nil || !due {
		t.Fatalf("initial Begin = due %t, error %v", due, err)
	}
	if err := store.Succeed(oldJob, attempt); err != nil {
		t.Fatal(err)
	}

	newJob := scheduledJob("7d", 21, 15)
	if _, due, err := New(root).Begin(newJob, now.Add(time.Minute)); err != nil || due {
		t.Fatalf("Begin after schedule change = due %t, error %v", due, err)
	}
	state, err := New(root).read(newJob.Name)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, time.November, 9, 21, 15, 0, 0, loc); !state.NextRun.Equal(want) {
		t.Fatalf("new first slot = %s, want %s", state.NextRun, want)
	}
}

func TestDispatcherLockIsExclusiveAndReleased(t *testing.T) {
	store := New(t.TempDir())
	unlock, locked, err := store.TryLock()
	if err != nil || !locked {
		t.Fatalf("first lock = %t, %v", locked, err)
	}
	if _, locked, err := store.TryLock(); err != nil || locked {
		t.Fatalf("second lock = %t, %v; want locked", locked, err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	unlock, locked, err = store.TryLock()
	if err != nil || !locked {
		t.Fatalf("lock after release = %t, %v", locked, err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}

func TestReadRejectsCorruptedState(t *testing.T) {
	store := New(t.TempDir())
	job := scheduledJob("1h", 0, 0)
	if err := store.ensureDir(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.dir, job.Name+".json")
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.read(job.Name); err == nil {
		t.Fatal("accepted malformed scheduler state")
	}
}

func scheduledJob(interval string, hour, minute int) config.Job {
	return config.Job{Name: "db", Schedule: config.Schedule{Interval: interval, HourAt: hour, MinuteAt: minute}}
}

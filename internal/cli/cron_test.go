package cli

import (
	"testing"
	"time"

	"github.com/ramaTImati/alldbackup/internal/config"
)

func TestRewriteCrontabInstallsIdempotentlyAndPreservesUnrelatedEntries(t *testing.T) {
	binary, configPath := "/usr/local/bin/alldbackup", "/etc/alldbackup/config.yaml"
	original := "# other task\n0 4 * * * /usr/local/bin/cleanup\n" +
		"0 2 * * * '/usr/local/bin/alldbackup' run --job 'old' --config '/etc/alldbackup/config.yaml'\n" +
		"0 3 * * * '/usr/local/bin/alldbackup' run --all --config '/etc/alldbackup/config.yaml'\n"
	got, err := rewriteCrontab(original, binary, configPath, true)
	if err != nil {
		t.Fatal(err)
	}
	want := "# other task\n0 4 * * * /usr/local/bin/cleanup\n" + cronBlockStart + "\n" +
		"*/15 * * * * '/usr/local/bin/alldbackup' --config '/etc/alldbackup/config.yaml' cron tick\n" + cronBlockEnd + "\n"
	if got != want {
		t.Fatalf("rewritten crontab = %q, want %q", got, want)
	}
	again, err := rewriteCrontab(got, binary, configPath, true)
	if err != nil || again != got {
		t.Fatalf("second install = %q, %v; want unchanged crontab", again, err)
	}
	removed, err := rewriteCrontab(got, binary, configPath, false)
	if err != nil || removed != "# other task\n0 4 * * * /usr/local/bin/cleanup\n" {
		t.Fatalf("remove = %q, %v", removed, err)
	}
}

func TestRewriteCrontabRejectsMalformedManagedBlock(t *testing.T) {
	if _, err := rewriteCrontab(cronBlockEnd+"\n", "/usr/local/bin/alldbackup", config.DefaultPath, true); err == nil {
		t.Fatal("accepted a crontab block with an unmatched end marker")
	}
}

func TestDispatcherOnlyRunsAtQuarterHourSlots(t *testing.T) {
	loc := time.FixedZone("server", 7*60*60)
	for _, minute := range []int{0, 15, 30, 45} {
		if !isQuarterHour(time.Date(2026, time.November, 9, 2, minute, 42, 0, loc)) {
			t.Errorf("minute %d was not accepted", minute)
		}
	}
	for _, minute := range []int{1, 10, 14, 16, 29, 31, 59} {
		if isQuarterHour(time.Date(2026, time.November, 9, 2, minute, 0, 0, loc)) {
			t.Errorf("minute %d was accepted", minute)
		}
	}
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	if got, want := shellQuote("/opt/alldbackup's copy"), "'/opt/alldbackup'\\''s copy'"; got != want {
		t.Fatalf("shellQuote() = %q, want %q", got, want)
	}
}

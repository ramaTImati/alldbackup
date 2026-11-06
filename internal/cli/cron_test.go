package cli

import (
	"reflect"
	"testing"

	"github.com/ramaTImati/alldbackup/internal/config"
)

func TestCronEntriesOnlyIncludeEnabledScheduledJobs(t *testing.T) {
	c := config.Config{Jobs: []config.Job{
		{Name: "daily", Enabled: true, Schedule: "0 2 * * *"},
		{Name: "manual", Enabled: true},
		{Name: "disabled", Enabled: false, Schedule: "0 3 * * *"},
	}}
	want := []string{"0 2 * * * '/opt/alldbackup' run --job 'daily' --config '/etc/alldbackup/config.yaml'"}
	if got := cronEntries(c, "/opt/alldbackup", "/etc/alldbackup/config.yaml"); !reflect.DeepEqual(got, want) {
		t.Fatalf("cronEntries() = %#v, want %#v", got, want)
	}
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	if got, want := shellQuote("/opt/alldbackup's copy"), "'/opt/alldbackup'\\''s copy'"; got != want {
		t.Fatalf("shellQuote() = %q, want %q", got, want)
	}
}

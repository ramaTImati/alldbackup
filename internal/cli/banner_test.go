package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommandPrintsBannerAndVersion(t *testing.T) {
	command := versionCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), banner) || !strings.Contains(output.String(), "alldbackup "+Version) {
		t.Fatalf("version output is missing banner or version: %q", output.String())
	}
}

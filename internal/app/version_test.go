package app

import (
	"os"
	"strings"
	"testing"
)

// The default Version must match the VERSION file so that a plain `go build`
// reports the same version as the installer/release builds.
func TestVersionMatchesFile(t *testing.T) {
	b, err := os.ReadFile("../../VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if v := strings.TrimSpace(string(b)); v != Version {
		t.Fatalf("VERSION=%q but app.Version=%q — bump both", v, Version)
	}
}

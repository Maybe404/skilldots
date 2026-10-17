package app_test

import (
	"os"
	"testing"
)

// The tests read English whatever the language of the machine: some run
// the CLI in this process, which chooses its language from the
// environment.
func TestMain(m *testing.M) {
	os.Setenv("SKILLDOTS_LANG", "en")
	os.Exit(m.Run())
}

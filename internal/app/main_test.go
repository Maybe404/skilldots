package app_test

import (
	"os"
	"testing"
)

// The tests read English whatever the language of the machine: some run
// the CLI in this process, which chooses its language from the
// environment.
//
// Every test also runs with a home directory of its own: skd keeps
// records beside the settings file (placed.json, local-trash.json,
// trash-notes.json), which must never be the real user's.
func TestMain(m *testing.M) {
	os.Setenv("SKILLDOTS_LANG", "en")
	home, err := os.MkdirTemp("", "skd-app-home")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", home+"/.config")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

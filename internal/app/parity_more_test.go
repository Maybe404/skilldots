package app_test

// The features added on top of the original flows, driven through the
// CLI and the dashboard's Ops on identical fixtures: pasted add sources,
// confirmed updates of chosen packs, the update history, edit with fork,
// settings, and skills Claude Code turned off.

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/cli"
	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/tui"
)

// cliJSON runs skd --json in process and decodes stdout into v.
func cliJSON(t *testing.T, root string, v any, args ...string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	cmd := cli.Root()
	cmd.SetArgs(append([]string{"--library", root, "--json"}, args...))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	runErr := cmd.Execute()
	w.Close()
	os.Stdout = stdout
	b, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("skd %v: %v\n%s", args, runErr, b)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("skd %v: %v\n%s", args, err, b)
	}
}

// use points HOME, the cache and git at one side's directories, as side
// does when it creates them.
func use(t *testing.T, home, up string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(home, "cache"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
}

// records drops what differs between two runs (the time) from history.
func records(recs []app.UpdateRecord) []app.UpdateRecord {
	out := []app.UpdateRecord{}
	for _, r := range recs {
		r.Time = time.Time{}
		out = append(out, r)
	}
	return out
}

func TestNewFeaturesAgree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := upstream(t)
	acme := filepath.Join(up, "acme", "skills.git")
	beta := filepath.Join(up, "beta", "tools.git")
	write(t, filepath.Join(beta, "skills", "lint", "SKILL.md"), skillMD("lint"))
	git(t, beta, "init", "-q")
	commit(t, beta, "init")

	type frontEnd struct {
		name, home, root string
		// steps return what the front end reported, for comparison.
		add     func() error
		update  func() []app.UpdateRecord
		history func() []app.UpdateRecord
		edit    func() error
		config  func() error
		claude  func() map[string]bool
	}
	pasted := "npx skills add https://github.com/acme/skills/tree/main/skills/eng --skill grill"
	editor := func(home string) string {
		p := filepath.Join(home, "bin", "code")
		write(t, p, "#!/bin/sh\nexit 0\n")
		os.Chmod(p, 0o755)
		return p
	}
	cliSide := func(home, root string) frontEnd {
		return frontEnd{name: "cli", home: home, root: root,
			add: func() error {
				runCLI(t, root, "add", pasted)
				runCLI(t, root, "add", "beta/tools", "--skill", "lint")
				return nil
			},
			update: func() []app.UpdateRecord {
				var out struct{ History []app.UpdateRecord }
				cliJSON(t, root, &out, "update", "acme/skills")
				return out.History
			},
			history: func() []app.UpdateRecord {
				var out struct{ Records []app.UpdateRecord }
				cliJSON(t, root, &out, "history")
				return out.Records
			},
			edit:   func() error { runCLI(t, root, "edit", "review", "--fork", "--as", "my-review"); return nil },
			config: func() error { runCLI(t, root, "config", "set", "editor", editor(home)); return nil },
			claude: func() map[string]bool {
				var entries []engine.Entry
				cliJSON(t, root, &entries, "list")
				out := map[string]bool{}
				for _, e := range entries {
					out[e.LinkName] = e.ClaudeDisabled
				}
				return out
			},
		}
	}
	tuiSide := func(home, root string) frontEnd {
		o := tui.NewOps(root)
		find := func(kind engine.Kind, name string) tui.Skill { return find(t, o, kind, name) }
		return frontEnd{name: "dashboard", home: home, root: root,
			add: func() error {
				// The add dialog: browse the pasted text, keep what it ticked.
				b, err := o.Browse(pasted)
				if err != nil {
					return err
				}
				var names []string
				for _, s := range b.Skills {
					if b.Preselect[s.Name] {
						names = append(names, s.Name)
					}
				}
				if _, err := o.Add(pasted, tui.AddRequest{Names: names}); err != nil {
					return err
				}
				_, err = o.Add("beta/tools", tui.AddRequest{Names: []string{"lint"}})
				return err
			},
			update: func() []app.UpdateRecord {
				// U: plan, untick beta/tools, update the rest.
				plan, err := o.PlanUpdate(nil)
				if err != nil {
					t.Fatal(err)
				}
				var chosen []string
				for _, r := range plan.Repos() {
					if r != "beta/tools" {
						chosen = append(chosen, r)
					}
				}
				if _, _, err := o.Update(chosen); err != nil {
					t.Fatal(err)
				}
				recs, err := o.History("")
				if err != nil {
					t.Fatal(err)
				}
				return recs
			},
			history: func() []app.UpdateRecord {
				recs, err := o.History("")
				if err != nil {
					t.Fatal(err)
				}
				return recs
			},
			edit: func() error {
				_, _, err := o.Edit(find(engine.KindVendor, "review"), app.EditOptions{Fork: true, As: "my-review"})
				return err
			},
			config: func() error { _, err := o.SetSetting(config.Editor, editor(home)); return err },
			claude: func() map[string]bool {
				s, err := o.Load()
				if err != nil {
					t.Fatal(err)
				}
				out := map[string]bool{}
				var all []tui.Skill
				for _, p := range s.Packs {
					all = append(all, p.Skills...)
				}
				for _, sk := range append(append(all, s.Mine...), s.Private...) {
					if sk.Installed {
						out[sk.LinkName] = sk.ClaudeOff
					}
				}
				return out
			},
		}
	}

	type result struct {
		states  []map[string]string
		updated []app.UpdateRecord
		history []app.UpdateRecord
		config  string
		claude  map[string]bool
	}
	// Both sides are set up first and every step runs on both, so they
	// see the same upstream commits.
	cliHome, cliRoot := side(t, up)
	tuiHome, tuiRoot := side(t, up)
	sides := []frontEnd{cliSide(cliHome, cliRoot), tuiSide(tuiHome, tuiRoot)}
	results := make([]result, 2)
	each := func(f func(i int, s frontEnd)) {
		for i, s := range sides {
			use(t, s.home, up)
			f(i, s)
		}
	}
	snap := func(i int, s frontEnd) { results[i].states = append(results[i].states, state(t, s.home, s.root)) }
	each(func(i int, s frontEnd) {
		if err := s.config(); err != nil {
			t.Fatalf("%s config: %v", s.name, err)
		}
		b, _ := os.ReadFile(config.Path())
		results[i].config = strings.ReplaceAll(string(b), s.home, "~")
		if err := s.add(); err != nil {
			t.Fatalf("%s add: %v", s.name, err)
		}
		snap(i, s)
	})
	// Upstream: tdd changes in acme/skills, lint in beta/tools. The CLI
	// names acme/skills; the dashboard unticks beta/tools.
	write(t, filepath.Join(acme, "skills", "eng", "tdd", "x.md"), "x\n")
	commit(t, acme, "tdd: x")
	write(t, filepath.Join(beta, "skills", "lint", "y.md"), "y\n")
	commit(t, beta, "lint: y")
	each(func(i int, s frontEnd) {
		results[i].updated = records(s.update())
		snap(i, s)
		results[i].history = records(s.history())
		if err := s.edit(); err != nil {
			t.Fatalf("%s edit: %v", s.name, err)
		}
		snap(i, s)
		write(t, filepath.Join(s.home, ".claude", "settings.json"), `{"skillOverrides": {"tdd": "off", "my-review": "off"}}`)
		results[i].claude = s.claude()
	})
	a, b := results[0], results[1]
	for i, step := range []string{"add", "update", "edit --fork"} {
		for k, want := range a.states[i] {
			if b.states[i][k] != want {
				t.Errorf("after %s, %s differs:\ncli:\n%s\ndashboard:\n%s", step, k, want, b.states[i][k])
			}
		}
		if len(a.states[i]) != len(b.states[i]) {
			t.Errorf("after %s: %d vs %d state keys", step, len(a.states[i]), len(b.states[i]))
		}
	}
	if !reflect.DeepEqual(a.updated, b.updated) || !reflect.DeepEqual(a.updated, a.history) {
		t.Errorf("update records differ:\ncli: %+v\ndashboard: %+v", a.updated, b.updated)
	}
	if !reflect.DeepEqual(a.history, b.history) {
		t.Errorf("history differs:\ncli: %+v\ndashboard: %+v", a.history, b.history)
	}
	if a.config != b.config {
		t.Errorf("config differs:\ncli:\n%s\ndashboard:\n%s", a.config, b.config)
	}
	if !reflect.DeepEqual(a.claude, b.claude) || !a.claude["tdd"] || !a.claude["my-review"] || a.claude["grill"] {
		t.Errorf("claude off: cli %v dashboard %v", a.claude, b.claude)
	}
	if !strings.Contains(a.states[0]["skills.toml"], "grill") || !strings.Contains(a.states[0]["skills.toml"], "tdd") {
		t.Errorf("the pasted command did not select eng/* and grill:\n%s", a.states[0]["skills.toml"])
	}
	if len(a.history) != 1 || a.history[0].Repo != "acme/skills" || len(a.history[0].Commits) == 0 {
		t.Errorf("history: %+v", a.history)
	}
}

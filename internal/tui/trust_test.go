package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/targets"
)

// setCustom makes skills.toml of the library at root have exactly the
// custom target name at path in mode.
func setCustom(t *testing.T, root, name, path, mode string) {
	t.Helper()
	m := loadManifest(t, root)
	m.Targets.Custom = map[string]manifest.CustomTarget{name: {Path: path, Mode: mode}}
	if err := m.Save(filepath.Join(root, manifest.FileName)); err != nil {
		t.Fatal(err)
	}
}

func targetRow(t *testing.T, o *libOps, name string) TargetRow {
	t.Helper()
	rows, err := o.Targets()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no target %s in %+v", name, rows)
	return TargetRow{}
}

// The dashboard approves what the person was shown and nothing else:
// not when a link on the way changed since, nor a mode or a path other
// than skills.toml's; and an approval is of the name with the directory
// and mode, so another name for the directory is not approved.
func TestOpsTrustTargetApprovesWhatWasShown(t *testing.T) {
	home, _ := sandbox(t)
	root := newLibrary(t, home)
	o := &libOps{root: root}
	os.MkdirAll(filepath.Join(home, "a"), 0o755)
	os.MkdirAll(filepath.Join(home, "b"), 0o755)
	via := filepath.Join(home, "via")
	os.Symlink(filepath.Join(home, "a"), via)
	setCustom(t, root, "mytool", "~/via/skills", "")
	shown := targetRow(t, o, "mytool")
	if shown.Active || !shown.Approvable || !strings.HasSuffix(shown.Path, filepath.Join("a", "skills")) {
		t.Fatalf("%+v", shown)
	}
	os.Remove(via)
	os.Symlink(filepath.Join(home, "b"), via)
	if _, err := o.TrustTarget(shown, ""); err == nil || !strings.Contains(err.Error(), "as shown") {
		t.Fatalf("approved after the link changed: %v", err)
	}
	if _, err := os.Stat(targets.ApprovalsFile()); err == nil {
		t.Fatal("approvals written")
	}
	os.Remove(via)
	os.Symlink(filepath.Join(home, "a"), via)
	for _, bad := range []TargetRow{
		{Name: shown.Name, Raw: "~/via/other/skills", Path: shown.Path, Mode: shown.Mode},
		{Name: shown.Name, Raw: shown.Raw, Path: shown.Path, Mode: "copy"},
	} {
		if _, err := o.TrustTarget(bad, ""); err == nil {
			t.Fatalf("approved %+v", bad)
		}
	}
	if _, err := o.TrustTarget(shown, ""); err != nil {
		t.Fatal(err)
	}
	if r := targetRow(t, o, "mytool"); !r.Active {
		t.Fatalf("approved, not active: %+v", r)
	}
	setCustom(t, root, "yours", "~/via/skills", "")
	if r := targetRow(t, o, "yours"); r.Active || !r.Approvable {
		t.Fatalf("another name for the approved directory: %+v", r)
	}
	setCustom(t, root, "mytool", "~/via/skills", "copy")
	if r := targetRow(t, o, "mytool"); r.Active || !r.Approvable {
		t.Fatalf("approved for link, used for copy: %+v", r)
	}
}

// A custom target whose directory is not named skills is approved in the
// dashboard only by typing its name after a warning: y and enter alone
// do not.
func TestTargetsViewTypesTheNameOfAnUnusualDirectory(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 40)
	f.targets = []TargetRow{
		{Name: "agents", Path: "/h/.agents/skills", Raw: "/h/.agents/skills", Mode: "link", Builtin: true, Active: true},
		{Name: "ext", Raw: "~/.my-editor/plugins", Path: "/h/.my-editor/plugins", Mode: "link", Approvable: true, Problem: "not approved"},
	}
	var cmd tea.Cmd
	m, cmd = press(t, m, "T")
	m = settle(t, m, cmd)
	m, _ = press(t, m, "a")
	if !strings.Contains(m.view(), "not named skills") || !strings.Contains(m.view(), "/h/.my-editor/plugins") {
		t.Fatalf("no warning:\n%s", m.view())
	}
	m, _ = press(t, m, "enter")
	if m.ov == nil || m.ov.kind != ovConfirm {
		t.Fatal("enter approved or closed the warning")
	}
	m, _ = press(t, m, "y")
	if m.ov == nil || m.ov.kind != ovInput {
		t.Fatalf("y does not ask for the name:\n%s", m.view())
	}
	for _, wrong := range []string{"y", "Ext", "yes"} {
		m = typeText(t, m, wrong)
		m, _ = press(t, m, "enter")
		if m.ov == nil || m.ov.err == "" {
			t.Fatalf("%q accepted", wrong)
		}
		for range wrong {
			m, _ = press(t, m, "backspace")
		}
	}
	m = typeText(t, m, "ext")
	m, cmd = press(t, m, "enter")
	settle(t, m, cmd)
	if !slices.Contains(f.calls, "trust ext /h/.my-editor/plugins link typed=ext") {
		t.Fatalf("calls: %v", f.calls)
	}
	if n := len(slices.DeleteFunc(slices.Clone(f.calls), func(c string) bool { return !strings.HasPrefix(c, "trust ") })); n != 1 {
		t.Fatalf("trusted %d times: %v", n, f.calls)
	}
}

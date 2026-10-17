package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// The dashboard after upgrading from an skd that kept no record of what
// it placed: turning the app off (every agent, or one) as the first
// thing removes its links outright, as `skd disable` does, never to the
// trash and never left behind.
func TestDashboardDisablesAnOlderSkdsAppLinks(t *testing.T) {
	for _, how := range []string{"disable", "one agent off"} {
		home, _ := sandbox(t)
		root := newLibrary(t, home)
		if err := library.Remember(root); err != nil {
			t.Fatal(err)
		}
		appDir := filepath.Join(home, "Applications", "Surge.app", "skills", "surge")
		write(t, filepath.Join(appDir, "SKILL.md"), skillMD("surge", "Control Surge."))
		m := loadManifest(t, root)
		m.Targets.Default = []string{"agents", "claude"}
		m.Apps = append(m.Apps, &manifest.App{Name: "surge", Path: appDir})
		if err := m.Save(filepath.Join(root, manifest.FileName)); err != nil {
			t.Fatal(err)
		}
		o := &libOps{root: root}
		if _, err := o.Apply(); err != nil {
			t.Fatal(err)
		}
		// What the older skd left: the same links, no record of them.
		if err := os.Remove(app.PlacedFile()); err != nil {
			t.Fatal(err)
		}
		s, err := o.Load()
		if err != nil || len(s.Apps) != 1 {
			t.Fatalf("%v %+v", err, s)
		}
		gone := []string{"agents", "claude"}
		if how == "disable" {
			_, err = o.Disable(s.Apps[0])
		} else {
			_, err = o.SetTargets(&s.Apps[0], "", []string{"agents"})
			gone = []string{"claude"}
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, tn := range gone {
			if _, err := os.Lstat(filepath.Join(home, "."+tn, "skills", "surge")); err == nil {
				t.Fatalf("%s: the link in %s is still there", how, tn)
			}
		}
		if how != "disable" && !isLink(filepath.Join(home, ".agents", "skills", "surge")) {
			t.Fatalf("%s: the wanted link went", how)
		}
		if trashed(t, root, "surge") {
			t.Fatalf("%s: went to the trash", how)
		}
		if _, err := os.Stat(filepath.Join(appDir, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}

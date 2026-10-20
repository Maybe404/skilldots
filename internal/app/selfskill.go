package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/targets"
)

/*
 * The skilldots skill shipped inside skilldots.app.
 *
 * The desktop app carries skills/skilldots of this repository in its bundle
 * (Contents/Resources/skills/skilldots, see desktop/mygo.json). A library made
 * by the app, or by the skd command the app installs, follows that copy with
 * an [[app]] entry instead of subscribing to SelfRepo on GitHub: it needs no
 * network, and the skill always describes the skd it ships with. skd built
 * on its own (go install) has no bundle and keeps the GitHub source.
 *
 * The entry names an absolute directory inside the app, so it goes stale when
 * the app moves (~/Applications to /Applications, a copy renamed by Finder) or
 * when the library is synced to a Mac where the app sits elsewhere. The app
 * repoints it when it opens a library (RepointAppSelfSkill): an [[app]]
 * skilldots entry whose path is some skilldots bundle's copy of the skill, and
 * not this app's, is rewritten to this app's. Only entries of that shape are
 * touched; an [[app]] skilldots the user pointed somewhere else is theirs.
 *
 * Several Macs sharing one library: the path is written with ~/ when the app
 * is under the home folder, so Macs that keep the app in the same place
 * (/Applications, or ~/Applications under any user name) write the same line
 * and nothing changes. Macs that keep it in different places each rewrite the
 * line when their app opens the library: a one-line change in skills.toml,
 * right for the Mac that made it; meanwhile the other Mac's links stay as they
 * are (apply skips an app skill whose directory is missing, it does not remove
 * the links) until its own app opens the library again. A Mac with only a
 * stand-alone skd sees the entry as missing and links nothing for it.
 */

// BundledSelfSkill returns the directory of the skilldots skill shipped with
// the running program, or "" when there is none (skd built on its own, the
// app in development, or run from its disk image). The desktop app sets it.
var BundledSelfSkill func() string

// bundledSelfSkill is BundledSelfSkill's answer, "" when it is unset.
func bundledSelfSkill() string {
	if BundledSelfSkill == nil {
		return ""
	}
	return BundledSelfSkill()
}

// bundleSkillSuffix ends the path of the skilldots skill inside any
// skilldots.app (whatever the bundle is called).
var bundleSkillSuffix = filepath.Join(".app", "Contents", "Resources", "skills", SelfSkill)

// IsBundledSelfSkill reports whether path (as written in an [[app]] entry,
// ~/ allowed) is the skilldots skill inside some app bundle: the kind of
// entry InstallSelfSkill writes and RepointAppSelfSkill may rewrite.
func IsBundledSelfSkill(path string) bool {
	home, _ := os.UserHomeDir()
	p := targets.Expand(path, home)
	return filepath.IsAbs(p) && strings.HasSuffix(p, bundleSkillSuffix) && len(p) > len(bundleSkillSuffix)+1
}

// appPath is how an [[app]] entry names dir: from ~/ when dir is inside the
// home folder, so that Macs keeping the app in the same place under
// different user names write the same line.
func appPath(dir string) string {
	dir = filepath.Clean(dir)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	for _, h := range []string{home, fsutil.Resolve(home)} {
		if rel, err := filepath.Rel(h, dir); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return dir
}

// selfApp returns the library's [[app]] entry for the skilldots skill, or nil.
func (s *Session) selfApp() *manifest.App {
	for _, a := range s.Lib.Manifest.Apps {
		if a.Name == SelfSkill {
			return a
		}
	}
	return nil
}

// installAppSelfSkill links the skilldots skill shipped at dir with an
// [[app]] entry, for the default targets and every agent installed on the
// machine resolving the library, like the GitHub source. An entry the
// library already has keeps its targets and only gets dir.
func (s *Session) installAppSelfSkill(dir string) (*engine.SyncReport, *ApplyOutcome, error) {
	if err := engine.CheckAppDir(dir); err != nil {
		return nil, nil, err
	}
	a := s.selfApp()
	if a == nil {
		t := append(append([]string{}, s.Lib.Manifest.Targets.Default...), engine.AllInstalled)
		a = &manifest.App{Name: SelfSkill, Targets: &t}
		s.Lib.Manifest.Apps = append(s.Lib.Manifest.Apps, a)
	}
	a.Path = appPath(dir)
	out, err := s.saveAndApply()
	if err != nil {
		return nil, out, err
	}
	return &engine.SyncReport{}, out, nil
}

// RepointAppSelfSkill points the library's [[app]] skilldots entry at dir,
// the skill shipped with the running app, when it names another app
// bundle's copy (the app moved, an older copy, another Mac's place; see the
// comment at the top of this file). It reports whether skills.toml changed.
// The links follow at once when nothing else was waiting to be applied;
// otherwise the change waits with the rest for the person to apply, and
// the outcome is nil.
func (s *Session) RepointAppSelfSkill(dir string) (bool, *ApplyOutcome, error) {
	a := s.selfApp()
	if dir == "" || a == nil || !IsBundledSelfSkill(a.Path) {
		return false, nil, nil
	}
	home, _ := os.UserHomeDir()
	if cur := targets.Expand(a.Path, home); cur == filepath.Clean(dir) || fsutil.Resolve(cur) == fsutil.Resolve(dir) {
		return false, nil, nil
	}
	if engine.CheckAppDir(dir) != nil {
		return false, nil, nil
	}
	entries, err := s.Resolve()
	if err != nil {
		return false, nil, err
	}
	waiting := s.Pending(entries)
	a.Path = appPath(dir)
	if waiting > 0 {
		return true, nil, s.Lib.Save()
	}
	out, err := s.saveAndApply()
	return true, out, err
}

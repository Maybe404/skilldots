package api

import (
	"log"
	"path/filepath"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/library"
)

/*
 * The skilldots skill inside the app. mygo.json ships the repository's
 * skills directory as a resource, so a packaged skilldots.app holds
 * Contents/Resources/skills/skilldots/SKILL.md, the skill written for the
 * skd in the same bundle. main.go hands BundledSelfSkill to the app layer
 * (app.BundledSelfSkill), which then links that copy with an [[app]] entry
 * instead of fetching Maybe404/skilldots from GitHub: a new library gets
 * the skill offline, and an update of the app updates the skill with it.
 * See internal/app/selfskill.go for how the entry follows the app when it
 * moves, and what that means for a library synced between Macs.
 */

// BundledSelfSkill is the skilldots skill shipped in the running app, or ""
// when there is none to link to: in development (`mygo dev` runs
// "skilldots Dev.app", go run no bundle at all), from the disk image or a
// translocated copy (read-only places that go away, appExecutable), or a
// bundle without the resource. Then the GitHub source is used, as before.
func BundledSelfSkill() string {
	exe, _, err := appExecutable()
	if err != nil {
		return ""
	}
	return skillInBundle(exe)
}

// skillInBundle is the skilldots skill inside the skilldots.app whose
// executable is exe, or "" when exe is not one or the skill is not there
// (or would not be linked, engine.CheckAppDir).
func skillInBundle(exe string) string {
	if !isAppExecutable(exe) {
		return ""
	}
	dir := filepath.Join(filepath.Dir(filepath.Dir(exe)), "Resources", "skills", app.SelfSkill)
	if engine.CheckAppDir(dir) != nil {
		return ""
	}
	return dir
}

// RepointSelfSkill makes the current library's [[app]] skilldots entry
// follow this app (see repointSelfSkill). main.go calls it at launch,
// before the window asks for anything. A function, not a method: mygo
// binds Service's methods for the window.
func RepointSelfSkill(s *Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repointSelfSkill()
}

// repointSelfSkill points the library's [[app]] skilldots entry at the
// skill inside this app when it names another skilldots.app's copy: the app
// moved, or the library came from a Mac that keeps it elsewhere. It runs
// when the app opens a library (launch, useLibrary, cloneLibrary) and
// never fails what the caller does: a problem is logged.
func (s *Service) repointSelfSkill() {
	dir := ""
	if app.BundledSelfSkill != nil {
		dir = app.BundledSelfSkill()
	}
	if dir == "" {
		return
	}
	root := s.root
	if root == "" {
		var err error
		if root, err = library.Locate(""); err != nil || root == "" {
			return
		}
	}
	ses, err := app.Open(root)
	if err != nil {
		return
	}
	if _, o, err := ses.RepointAppSelfSkill(dir); checkApply(o, err) != nil {
		log.Printf("skilldots skill: could not point %s at %s: %v", root, dir, checkApply(o, err))
	}
}

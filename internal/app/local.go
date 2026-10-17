package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// NewOutcome is the result of New.
type NewOutcome struct {
	Name  string
	Path  string
	Apply *ApplyOutcome
}

// New scaffolds a skill of the user's own in skills/ (private/ when
// private) and links it to the default agents.
func (s *Session) New(name string, private bool) (*NewOutcome, error) {
	if err := manifest.CheckName(name); err != nil {
		return nil, err
	}
	base := s.Lib.SkillsDir()
	if private {
		base = s.Lib.PrivateDir()
	}
	dir := filepath.Join(base, name)
	if err := s.writableInLibrary(dir); err != nil {
		return nil, err
	}
	if fsutil.Lexists(dir) {
		return nil, i18n.Errorf("%s already exists", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	body := fmt.Sprintf("---\nname: %s\ndescription: What this skill does and when an agent should use it.\n---\n\n# %s\n\nInstructions for the agent go here.\n", name, name)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		return nil, err
	}
	s.logf(Done, "created %s", dir)
	ap, err := s.Apply(ApplyOptions{})
	if err != nil {
		return nil, err
	}
	return &NewOutcome{Name: name, Path: dir, Apply: ap}, nil
}

// Kind of the entry New creates.
func NewKind(private bool) engine.Kind {
	if private {
		return engine.KindPrivate
	}
	return engine.KindSkill
}

// writableInLibrary reports why skd must not create dir, a new skill
// directory in the library (new, fork, adopt): a symbolic link inside
// the library on the way to it. A cloned library's skills/ (or private/)
// can be a link to anywhere, and what skd writes there, a fork of
// someone's vendored files included, would land wherever it points.
func (s *Session) writableInLibrary(dir string) error {
	if l := fsutil.LinkOnPath(s.Lib.Root, dir); l != "" {
		return i18n.Errorf("%s goes through %s, a symbolic link inside the library; skd writes nothing through it (make it a directory of its own)", dir, l)
	}
	return nil
}

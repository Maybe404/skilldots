package api

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/safetext"
)

// initLibrary creates a library in an empty directory, the way `skd init`
// does, and with withSkill also installs the skilldots skill for the
// agents on this machine (installSelfSkill). When that fails the library
// stays, and the status carries skillError.
func (s *Service) initLibrary(dir string, withSkill bool) (any, error) {
	dir, err := filepath.Abs(expand(dir))
	if err != nil {
		return nil, err
	}
	// Only an empty chosen directory is initialized. This avoids taking over
	// somebody else's repository or overwriting unrelated files.
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(entries) > 0 {
		return nil, errors.New("choose an empty directory for the new library")
	}
	if err = app.InitLibrary(dir, nil); err != nil {
		return nil, err
	}
	s.root = dir
	s.previews = map[string]*app.Fetched{}
	var skillErr error
	if withSkill {
		_, skillErr = s.installSelfSkill()
	}
	out, err := s.status()
	if err != nil {
		return nil, err
	}
	if skillErr != nil {
		out.(object)["skillError"] = skillErr.Error()
	}
	return out, nil
}

// installSelfSkill links the skilldots skill into every agent on this
// machine, unless the library already has one (a GitHub source, a fork, or
// an [[app]] entry). The packaged app links the copy inside its bundle with
// an [[app]] entry and needs no network; in development, or run from the
// disk image, it subscribes to Maybe404/skilldots on GitHub (see
// BundledSelfSkill and app.Session.InstallSelfSkill).
func (s *Service) installSelfSkill() (any, error) {
	ses, err := s.session()
	if err != nil {
		return nil, err
	}
	if !ses.HasSelfSkill() {
		_, o, err := ses.InstallSelfSkill()
		if err = checkApply(o, err); err != nil {
			return nil, err
		}
	}
	return s.status()
}

func (s *Service) cloneLibrary(url, dir string) (any, error) {
	if safetext.Has(url) || strings.HasPrefix(url, "-") || strings.TrimSpace(url) == "" {
		return nil, errors.New("invalid repository URL")
	}
	dir, err := filepath.Abs(expand(dir))
	if err != nil {
		return nil, err
	}
	if fsutil.Lexists(dir) {
		return nil, errors.New("clone destination already exists")
	}
	if _, err = config.Load(); err != nil {
		return nil, err
	}
	previousRoot := s.root
	if previousRoot == "" {
		previousRoot, _ = library.Locate("")
	}
	// The destination did not exist when checked above. If clone or any
	// post-clone validation fails, remove only what this operation created and
	// restore the remembered library selection.
	cleanup := true
	defer func() {
		if !cleanup {
			return
		}
		_ = os.RemoveAll(dir)
		if previousRoot != "" {
			_ = app.UseLibrary(previousRoot)
		}
		s.root = previousRoot
		s.previews = map[string]*app.Fetched{}
	}()
	app.SealOlderSkd()
	// A repository without a skills.toml (someone's repository of skills)
	// is removed again, with an error saying how to add it as a source.
	if err = app.CloneLibrary(url, dir); err != nil {
		return nil, err
	}
	if _, err = library.Open(dir); err != nil {
		return nil, err
	}
	// The upstream snapshots pushed with it (best effort): the merge base
	// of the next update of a skill changed on the other machine.
	_ = app.FetchSnapshots(dir)
	if err = library.EnsurePrivateGit(dir); err != nil && !errors.Is(err, library.ErrPrivateLink) {
		return nil, err
	}
	if err = app.UseLibrary(dir); err != nil {
		return nil, err
	}
	s.root = dir
	s.previews = map[string]*app.Fetched{}
	// A library from another Mac may name the skilldots skill inside the
	// app where that Mac keeps it: point it at this app's before linking.
	s.repointSelfSkill()
	ses, err := s.session()
	if err != nil {
		return nil, err
	}
	o, err := ses.Apply(app.ApplyOptions{})
	if err = checkApply(o, err); err != nil {
		return nil, err
	}
	out, err := s.status()
	if err != nil {
		return nil, err
	}
	cleanup = false
	return out, nil
}

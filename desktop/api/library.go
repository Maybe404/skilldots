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
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/safetext"
)

func (s *Service) initLibrary(dir string) (any, error) {
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
	if _, err = config.Load(); err != nil {
		return nil, err
	}
	for _, part := range []string{"skills", "private", "vendor"} {
		p := filepath.Join(dir, part)
		if fsutil.LinkOnPath(dir, p) != "" {
			return nil, errors.New("library directories cannot be symbolic links")
		}
		if err = os.MkdirAll(p, 0755); err != nil {
			return nil, err
		}
	}
	m := manifest.New([]string{"agents"})
	if err = m.Save(filepath.Join(dir, manifest.FileName)); err != nil {
		return nil, err
	}
	l := &lock.Lock{Version: 1, Packs: map[string]*lock.Pack{}, Links: map[string]*lock.Link{}}
	if err = l.Save(filepath.Join(dir, lock.FileName)); err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("/private/\n/.trash/\n.DS_Store\n.*.tmp-*\n"), 0644); err != nil {
		return nil, err
	}
	if err = library.EnsureGit(dir); err != nil {
		return nil, err
	}
	if err = library.EnsurePrivateGit(dir); err != nil {
		return nil, err
	}
	if err = app.UseLibrary(dir); err != nil {
		return nil, err
	}
	s.root = dir
	s.previews = map[string]*app.Fetched{}
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
	app.SealOlderSkd()
	if _, err = library.Git("", "clone", "--", url, dir); err != nil {
		return nil, err
	}
	if _, err = library.Open(dir); err != nil {
		return nil, err
	}
	if err = library.EnsurePrivateGit(dir); err != nil && !errors.Is(err, library.ErrPrivateLink) {
		return nil, err
	}
	if err = app.UseLibrary(dir); err != nil {
		return nil, err
	}
	s.root = dir
	s.previews = map[string]*app.Fetched{}
	ses, err := s.session()
	if err != nil {
		return nil, err
	}
	o, err := ses.Apply(app.ApplyOptions{})
	if err = checkApply(o, err); err != nil {
		return nil, err
	}
	return s.status()
}

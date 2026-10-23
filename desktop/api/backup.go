package api

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/Maybe404/skilldots/internal/app"
)

// backupZip writes a zip backup of the library into dir (see
// app.Session.BackupZip).
func (s *Service) backupZip(dir string) (any, error) {
	dir, err := filepath.Abs(expand(dir))
	if err != nil {
		return nil, err
	}
	ses, err := s.session()
	if err != nil {
		return nil, err
	}
	version := BundleVersion()
	if version == "" {
		version = "dev"
	}
	res, err := ses.BackupZip(dir, version)
	if err != nil {
		return nil, err
	}
	return object{"path": res.Path, "skills": len(res.Skills), "files": res.Files, "bytes": res.Bytes}, nil
}

// restoreZip restores the library of a zip backup into dir, which must not
// exist or be empty, makes it the current library and links its skills,
// the way cloneLibrary does with a git remote.
func (s *Service) restoreZip(zipPath, dir string) (any, error) {
	dir, err := filepath.Abs(expand(dir))
	if err != nil {
		return nil, err
	}
	app.SealOlderSkd()
	res, err := app.RestoreBackup(expand(zipPath), dir)
	if err != nil {
		return nil, err
	}
	s.root = dir
	s.previews = map[string]*app.Fetched{}
	// A backup from another Mac may name the skilldots skill inside the
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
	if st, ok := out.(object); ok {
		st["restored"] = object{"skills": len(res.Info.Skills), "skipped": nonNilList(res.Skipped)}
	}
	return out, nil
}

// revealBackup shows a backup backupZip wrote in Finder. Only such a file
// is shown: the path comes from the page.
func (s *Service) revealBackup(p string) error {
	if !filepath.IsAbs(p) || !strings.HasPrefix(filepath.Base(p), "skilldots-backup-") || filepath.Ext(p) != ".zip" {
		return errors.New("not a skilldots backup")
	}
	if s.native.Reveal == nil {
		return errors.New("reveal unavailable")
	}
	return s.native.Reveal(p)
}

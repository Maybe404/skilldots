package app

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
)

// NewOutcome is the result of New.
type NewOutcome struct {
	Name  string
	Path  string
	Apply *ApplyOutcome
	// Pack and RepoPath are set for a skill made in the user's own
	// repository (NewInPack): the pack and its directory there.
	Pack     string `json:",omitempty"`
	RepoPath string `json:",omitempty"`
}

// New scaffolds a skill of the user's own in skills/ (private/ when
// private) and links it to the default agents.
func (s *Session) New(name string, private bool) (*NewOutcome, error) {
	return s.newWithTargets(name, private, nil, "")
}

// NewForTargets creates a skill with an explicit selection before any links
// are made, so a private skill never briefly reaches the default agents.
func (s *Session) NewForTargets(name string, private bool, names []string) (*NewOutcome, error) {
	return s.NewForTargetsWithDescription(name, private, names, "")
}

// NewForTargetsWithDescription creates a skill and writes its initial
// description before any link is applied. The whole operation is rolled back
// when saving or applying fails, so callers never receive a half-created skill.
func (s *Session) NewForTargetsWithDescription(name string, private bool, names []string, description string) (*NewOutcome, error) {
	for _, n := range names {
		if _, err := s.Reg.Get(n); err != nil {
			return nil, err
		}
	}
	selected := append([]string{}, names...)
	return s.newWithTargets(name, private, &selected, description)
}

func (s *Session) newWithTargets(name string, private bool, names *[]string, description string) (_ *NewOutcome, err error) {

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
	manifestPath := filepath.Join(s.Lib.Root, manifest.FileName)
	lockPath := filepath.Join(s.Lib.Root, lock.FileName)
	manifestBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	lockBefore, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		_ = os.RemoveAll(dir)
		_ = fsutil.WriteAtomic(manifestPath, manifestBefore)
		_ = fsutil.WriteAtomic(lockPath, lockBefore)
	}()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), scaffold(name, description), 0o644); err != nil {
		return nil, err
	}
	if names != nil {
		found := false
		for _, local := range s.Lib.Manifest.Locals {
			if local.Name == name {
				local.Targets = names
				found = true
				break
			}
		}
		if !found {
			s.Lib.Manifest.Locals = append(s.Lib.Manifest.Locals, &manifest.Local{Name: name, Targets: names})
		}
		if err := s.Lib.Save(); err != nil {
			return nil, err
		}
	}
	s.logf(Done, "created %s", dir)

	ap, err := s.Apply(ApplyOptions{})
	if err != nil {
		return nil, err
	}
	if len(ap.Refused) > 0 {
		return nil, i18n.Errorf("some links were not changed: %s", strings.Join(ap.Refused, "; "))
	}
	committed = true
	return &NewOutcome{Name: name, Path: dir, Apply: ap}, nil
}

// scaffold is the SKILL.md of a new skill.
func scaffold(name, description string) []byte {
	descriptionLine := "What this skill does and when an agent should use it."
	if description != "" {
		descriptionLine = strconv.Quote(description)
	}
	return []byte(fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n\nInstructions for the agent go here.\n", name, descriptionLine, name))
}

// NewInPackOptions is what NewInPack makes besides the name.
type NewInPackOptions struct {
	// Dir is the directory of the repository the skill goes into
	// (slash-separated, "." for its top); empty: beside most of its
	// skills (skills/ when it has none).
	Dir         string
	Description string
	// Agents are the skill's targets; nil: the pack's.
	Agents []string
}

// NewInPack scaffolds a skill in the user's own repository packName (a
// pack with own = true) and links it. It is a draft: in vendor/ like the
// pack's other skills, where it is edited in place, and in the
// repository once skd contribute pushes it. Nothing is sent. The whole
// operation is rolled back when saving or linking fails.
func (s *Session) NewInPack(name, packName string, opt NewInPackOptions) (_ *NewOutcome, err error) {
	if err := manifest.CheckName(name); err != nil {
		return nil, err
	}
	m := s.Lib.Manifest
	p := m.FindPack(packName)
	if p == nil {
		return nil, i18n.Errorf("no pack %q", packName)
	}
	if !p.Own {
		return nil, i18n.Errorf("%s is not marked as your own repository; skills are made only in your own (skd own %s marks it, if it is yours)", p.Repo, p.Repo)
	}
	for _, n := range opt.Agents {
		if _, err := s.Reg.Get(n); err != nil {
			return nil, err
		}
	}
	lp := s.Lib.Lock.Packs[p.Repo]
	if lp == nil {
		return nil, i18n.Errorf("%s has not been fetched: skd update %s first", p.Repo, p.Repo)
	}
	if lp.Skills == nil {
		lp.Skills = map[string]*lock.Skill{}
	}
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if strings.EqualFold(e.LinkName, name) || e.Kind == engine.KindVendor && e.Origin == p.Repo && strings.EqualFold(e.Name, name) {
			return nil, i18n.Errorf("the library already has a skill named %s (%s): pick another name", e.LinkName, engine.Describe(e))
		}
	}
	if lp.Skills[name] != nil {
		return nil, i18n.Errorf("%s already has a skill named %s", p.Repo, name)
	}
	// Its place in the repository: given, or beside the others (as the
	// cache clone, the last update's, has them).
	rel := ""
	switch {
	case opt.Dir == "." || opt.Dir == "./":
		rel = name
	case opt.Dir != "":
		rel = path.Join(filepath.ToSlash(strings.TrimRight(opt.Dir, "/")), name)
	default:
		parent := "skills"
		if clone := pack.CachePath(p.Repo); fsutil.Exists(filepath.Join(clone, ".git")) {
			unlock := pack.Hold(p.Repo)
			up, _, serr := pack.Scan(clone)
			unlock()
			if serr == nil {
				if pack.Find(up, name) != nil {
					return nil, i18n.Errorf("%s already has a skill named %s: skd update %s brings it", p.Repo, name, p.Repo)
				}
				if parent, err = skillsParent(up, clone); err != nil {
					return nil, err
				}
			}
		}
		rel = path.Join(parent, name)
	}
	if err := checkRepoPath(rel); err != nil {
		return nil, err
	}
	cat := pack.Category(rel)
	if err := manifest.CheckCategory(cat); err != nil {
		return nil, err
	}
	packDir, err := engine.PackDir(s.Lib, p.Repo)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(packDir, filepath.FromSlash(cat), name)
	if !fsutil.Within(packDir, dir) {
		return nil, i18n.Errorf("%s is outside %s", dir, packDir)
	}
	if err := s.writableInLibrary(dir); err != nil {
		return nil, err
	}
	if fsutil.Lexists(dir) {
		return nil, i18n.Errorf("%s already exists", dir)
	}
	if err := s.Lib.Writable(); err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(s.Lib.Root, manifest.FileName)
	lockPath := filepath.Join(s.Lib.Root, lock.FileName)
	manifestBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	lockBefore, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, err
	}
	created := !fsutil.Exists(filepath.Dir(dir))
	committed := false
	defer func() {
		if committed {
			return
		}
		_ = os.RemoveAll(dir)
		if created {
			_ = os.Remove(filepath.Dir(dir))
		}
		_ = fsutil.WriteAtomic(manifestPath, manifestBefore)
		_ = fsutil.WriteAtomic(lockPath, lockBefore)
	}()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), scaffold(name, opt.Description), 0o644); err != nil {
		return nil, err
	}
	lp.Skills[name] = &lock.Skill{Category: cat, Draft: true, Path: rel}
	if !p.UsesAll() {
		p.Use = dedupe(append(p.Use, name))
	} else if !p.Selected(name, cat) {
		var skip []string
		for _, sk := range p.Skip {
			if sk != name && sk != cat+"/"+name {
				skip = append(skip, sk)
			}
		}
		p.Skip = skip
		if !p.Selected(name, cat) {
			return nil, i18n.Errorf("%s is excluded by skip = [%q] of %s in skills.toml, which excludes other skills too: edit skills.toml first", name, p.SkipPattern(name, cat), p.Repo)
		}
	}
	if opt.Agents != nil {
		t := append([]string{}, opt.Agents...)
		p.Override(name, true).Targets = &t
	}
	if err := s.Lib.Save(); err != nil {
		return nil, err
	}
	s.logf(Done, "created %s", dir)
	ap, err := s.Apply(ApplyOptions{})
	if err != nil {
		return nil, err
	}
	if len(ap.Refused) > 0 {
		return nil, i18n.Errorf("some links were not changed: %s", strings.Join(ap.Refused, "; "))
	}
	committed = true
	return &NewOutcome{Name: name, Path: dir, Apply: ap, Pack: p.Repo, RepoPath: rel}, nil
}

// SetOwn marks a pack as the user's own repository, or not (skd own).
// Only skills.toml changes.
func (s *Session) SetOwn(packName string, own bool) (*manifest.Pack, error) {
	p := s.Lib.Manifest.FindPack(packName)
	if p == nil {
		return nil, i18n.Errorf("no pack %q", packName)
	}
	if p.Own == own {
		return p, nil
	}
	p.Own = own
	if err := s.Lib.Save(); err != nil {
		p.Own = !own
		return nil, err
	}
	return p, nil
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

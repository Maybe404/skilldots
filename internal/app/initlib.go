package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/targets"
)

// SelfRepo ships the skill that teaches agents to use skd.
const (
	SelfRepo  = "Maybe404/skilldots"
	SelfSkill = "skilldots"
)

// DefaultTargets are the default targets of a new library: the shared
// ~/.agents/skills, which most agents read, and Claude Code's own
// directory when Claude Code is installed, since it does not read the
// shared one. (Not on a machine without it: that would make ~/.claude.)
func DefaultTargets() []string {
	out := []string{"agents"}
	if home, err := os.UserHomeDir(); err == nil && targets.Installed(filepath.Join(home, ".claude", "skills")) {
		out = append(out, "claude")
	}
	return out
}

const gitignore = `.DS_Store
# Machine-local skills: a separate git repository that is never pushed with this one.
/private/
# Where skd parks anything it takes out of the way. Empty with: skd trash empty
/.trash/
# Temporary files of a save that was interrupted; the next save removes them.
.*.tmp-*
`

// InitLibrary creates a library at root (an existing directory keeps its
// files) and records it in the settings as the current library. defaults
// are the default targets; none means DefaultTargets. It is `skd init` and
// the desktop's new library; the skilldots skill is InstallSelfSkill's.
func InitLibrary(root string, defaults []string) error {
	if fsutil.Exists(filepath.Join(root, manifest.FileName)) {
		return i18n.Errorf("%s already has a %s", root, manifest.FileName)
	}
	// init records the library in the settings file at the end:
	// fail before creating anything when it cannot.
	if _, err := config.Load(); err != nil {
		return i18n.Errorf("%w; init records the library there, so fix or remove it first (nothing was created)", err)
	}
	// private/ becomes a git repository of its own: never through
	// a link (init --from a directory that has one).
	if l := library.PrivateLink(root); l != "" {
		return i18n.Errorf("%s is a symbolic link; skd does not set up private/ through it (make it a directory of its own; nothing was created)", l)
	}
	for _, d := range []string{"", library.DirSkills, library.DirVendor, library.DirPrivate} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return err
		}
	}
	if len(defaults) == 0 {
		defaults = DefaultTargets()
	}
	for _, f := range []string{manifest.FileName, lock.FileName, ".gitignore"} {
		if err := library.CheckFile(root, filepath.Join(root, f)); err != nil {
			return err
		}
	}
	m := manifest.New(defaults)
	if err := m.Save(filepath.Join(root, manifest.FileName)); err != nil {
		return err
	}
	l := &lock.Lock{Version: 1, Packs: map[string]*lock.Pack{}, Links: map[string]*lock.Link{}}
	if err := l.Save(filepath.Join(root, lock.FileName)); err != nil {
		return err
	}
	gi := filepath.Join(root, ".gitignore")
	if !fsutil.Exists(gi) {
		os.WriteFile(gi, []byte(gitignore), 0o644)
	} else {
		b, _ := os.ReadFile(gi)
		for _, line := range []string{"/private/", "/.trash/", ".*.tmp-*"} {
			if !strings.Contains(string(b), line) {
				b = append(b, []byte("\n"+line+"\n")...)
			}
		}
		os.WriteFile(gi, b, 0o644)
	}
	if err := library.EnsureGit(root); err != nil {
		return err
	}
	if err := library.EnsurePrivateGit(root); err != nil {
		return err
	}
	return UseLibrary(root)
}

// Detected reports whether an agent looks installed on this machine: its
// skills directory or the directory holding it exists (~/.claude for
// ~/.claude/skills).
func Detected(path string) bool { return targets.Installed(path) }

// DetectedTargets are the library's default targets and every built-in
// agent installed on this machine: what targets = ["*"] links into here.
func (s *Session) DetectedTargets() []string {
	return engine.ExpandTargets([]string{engine.AllInstalled}, s.Lib.Manifest.Targets.Default, s.Reg)
}

// HasSelfSkill reports whether the agents already get a skilldots skill:
// SelfRepo's, or the user's own fork of it.
func (s *Session) HasSelfSkill() bool {
	entries, err := s.Resolve()
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name != SelfSkill {
			continue
		}
		if e.Kind == engine.KindSkill || e.Kind == engine.KindPrivate || (e.Kind == engine.KindVendor && strings.EqualFold(e.Origin, SelfRepo)) {
			return true
		}
	}
	return false
}

// InstallSelfSkill follows the skilldots skill (added to SelfRepo's pack
// when the library has one) for the default targets and every agent
// installed on the machine resolving the library (targets.AllInstalled,
// after the defaults for an older skd), since Claude Code, for one, does
// not read ~/.agents/skills. Agents the user chose for that pack are kept.
// It fetches from GitHub and is an extra to whatever the caller did: any
// error comes with a nil report, and callers report it and go on.
func (s *Session) InstallSelfSkill() (*engine.SyncReport, *ApplyOutcome, error) {
	src, err := pack.ParseSource(SelfRepo)
	if err != nil {
		return nil, nil, err
	}
	f, err := s.Fetch(src)
	if err != nil {
		return nil, nil, err
	}
	var agents []string
	if p := s.Lib.Manifest.FindPack(SelfRepo); p == nil || p.Targets == nil {
		agents = append(append([]string{}, s.Lib.Manifest.Targets.Default...), engine.AllInstalled)
	}
	out, err := s.Add(f, Selection{Skills: []string{SelfSkill}, Agents: agents})
	if err != nil {
		return nil, nil, err
	}
	return out.Report, out.Apply, nil
}

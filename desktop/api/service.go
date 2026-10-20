// Package api adapts the shared application layer to the desktop contract.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/pack"
)

type object = map[string]any

type Native struct {
	PickFolder func(context.Context) (string, error)
	Reveal     func(string) error
}

// Service serializes calls because app.Session and the engine registry are
// process-scoped. Each operation reopens the manifest, including after errors.
type Service struct {
	mu       sync.Mutex
	root     string
	native   Native
	previews map[string]*app.Fetched
}

func New(root string, native Native) *Service {
	return &Service{root: root, native: native, previews: map[string]*app.Fetched{}}
}

type ref struct {
	Key  string `json:"key"`
	Pack string `json:"pack"`
}

func (r ref) app() app.Ref {
	if r.Pack != "" {
		return app.PackRef(r.Pack)
	}
	return app.KeyRef(r.Key)
}
func argument[T any](args []json.RawMessage, i int) (T, error) {
	var v T
	if i >= len(args) {
		return v, fmt.Errorf("missing argument %d", i+1)
	}
	err := json.Unmarshal(args[i], &v)
	return v, err
}

func (s *Service) session() (*app.Session, error) {
	root := s.root
	if root == "" {
		var err error
		root, err = library.Locate("")
		if err != nil {
			return nil, err
		}
	}
	return app.Open(root)
}

func skill(e engine.Entry) object {
	prefix := string(e.Kind)
	if e.Kind == engine.KindVendor {
		prefix = e.Origin
	}
	return object{"ref": ref{Key: prefix + ":" + e.Name}, "name": e.Name, "linkName": e.LinkName, "kind": e.Kind, "origin": e.Origin, "category": e.Category, "description": e.Description, "dir": e.Dir, "targets": append([]string{}, e.TargetNames...), "absent": append([]string{}, e.Absent...), "conflict": e.Conflict, "missing": e.Missing}
}

// absentOf is what an apply left out because the agent is not installed
// on this machine (agent → skills); always a map, empty when nothing.
func absentOf(o *app.ApplyOutcome) map[string][]string {
	if o == nil || o.Absent == nil {
		return map[string][]string{}
	}
	return o.Absent
}

func nonNilList(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func checkApply(out *app.ApplyOutcome, err error) error {
	if err != nil {
		return err
	}
	if out != nil && len(out.Refused) > 0 {
		return fmt.Errorf("some links were not changed: %s", strings.Join(out.Refused, "; "))
	}
	return nil
}

// Invoke is the desktop-only API. It has no custom-target trust operation.
// Its only permanent deletion is the trash (emptyTrash, deleteTrash): the
// person at the window confirms it first, and the call carries what that
// confirmation showed (trash.go). The CLI JSON protocol is not involved.
func (s *Service) Invoke(ctx context.Context, method string, args []json.RawMessage) (any, error) {
	// The skd command touches no library and may wait on the password
	// prompt: it runs outside the lock, so the rest of the app keeps going.
	switch method {
	case "cliStatus":
		return cliStatus(), nil
	case "installCli":
		return installCLIHere()
	case "removeCli":
		return removeCLIHere()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch method {
	case "initLibrary":
		dir, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		// The second argument is optional: without it no skill is installed.
		withSkill := false
		if len(args) > 1 {
			if withSkill, err = argument[bool](args, 1); err != nil {
				return nil, err
			}
		}
		return s.initLibrary(dir, withSkill)
	case "installSelfSkill":
		return s.installSelfSkill()
	case "cloneLibrary":
		url, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		dir, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		return s.cloneLibrary(url, dir)
	case "config":
		return readConfig()
	case "setConfig":
		patch, err := argument[map[string]json.RawMessage](args, 0)
		if err != nil {
			return nil, err
		}
		return writeConfig(patch)
	case "pickFolder":
		if s.native.PickFolder == nil {
			return nil, errors.New("folder picker unavailable")
		}
		p, err := s.native.PickFolder(ctx)
		if p == "" {
			return nil, err
		}
		return p, err
	case "status":
		return s.status()
	case "useLibrary":
		dir, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		dir = expand(dir)
		if _, err = library.Open(dir); err != nil {
			return nil, err
		}
		if err = app.UseLibrary(dir); err != nil {
			return nil, err
		}
		s.root = dir
		s.previews = map[string]*app.Fetched{}
		// That library may name another copy of this app's skill (the app
		// moved since it was used, or it came from another Mac).
		s.repointSelfSkill()
		return s.status()
	}
	ses, err := s.session()
	if err != nil {
		return nil, err
	}
	switch method {
	case "skills":
		entries, err := ses.Resolve()
		out := []object{}
		for _, e := range entries {
			out = append(out, skill(e))
		}
		return out, err
	case "setAgentHidden":
		name, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		hidden, err := argument[bool](args, 1)
		if err != nil {
			return nil, err
		}
		if _, err = ses.Reg.Get(name); err != nil {
			return nil, err
		}
		prefs, err := readPreferences()
		if err != nil {
			return nil, err
		}
		if prefs.HiddenAgents == nil {
			prefs.HiddenAgents = map[string]bool{}
		}
		prefs.HiddenAgents[name] = hidden
		b, err := json.Marshal(prefs)
		if err != nil {
			return nil, err
		}
		return nil, fsutil.WriteAtomic(prefPath(), b)
	case "agents":
		prefs, err := readPreferences()
		if err != nil {
			return nil, err
		}
		out := []object{}
		entries, err := ses.Resolve()
		if err != nil {
			return nil, err
		}
		used := map[string]bool{}
		for _, e := range entries {
			for _, n := range e.TargetNames {
				used[n] = true
			}
		}
		for _, a := range ses.Reg.All() {
			detected := app.Detected(a.Path)
			hidden, explicit := prefs.HiddenAgents[a.Name]
			if !explicit {
				hidden = !detected && !used[a.Name] && !slices.Contains(ses.Lib.Manifest.Targets.Default, a.Name)
			}
			out = append(out, object{"name": a.Name, "label": a.Name, "path": a.Path, "detected": detected, "enabled": !slices.Contains(ses.Lib.Manifest.Targets.Disabled, a.Name), "isDefault": slices.Contains(ses.Lib.Manifest.Targets.Default, a.Name), "hidden": hidden, "builtin": a.Builtin, "readsShared": a.ReadsShared})
		}
		return out, nil
	case "packs":
		out := []object{}
		for _, p := range ses.Lib.Manifest.Packs {
			commit := ""
			count := 0
			if l := ses.Lib.Lock.Packs[p.Repo]; l != nil {
				commit = l.Commit
				count = len(l.Upstream)
			}
			src, e := pack.ParseSource(p.Repo)
			if e != nil {
				return nil, e
			}
			out = append(out, object{"repo": p.Repo, "alias": p.Alias, "ref": p.Ref, "commit": commit, "followAll": p.UsesAll(), "upstreamCount": count, "url": src.URL})
		}
		return out, nil
	case "claudeOverrides":
		out := []string{}
		for name, off := range app.ClaudeDisabled() {
			if off {
				out = append(out, name)
			}
		}
		slices.Sort(out)
		return out, nil
	case "recordedOutdated":
		return outdated(ses.RecordedOutdated()), nil
	case "outdated":
		repos := []string{}
		if len(args) > 0 {
			repos, err = argument[[]string](args, 0)
			if err != nil {
				return nil, err
			}
		}
		o, err := ses.OutdatedContext(ctx, repos, nil)
		return outdated(o), err
	case "history":
		rows, _, err := ses.History("", 0)
		return history(rows), err
	case "trash", "trashUsage", "emptyTrash", "deleteTrash":
		return s.trashOperation(ses, method, args)
	case "restore":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		// RestoreEntry puts back what remove or purge took out of
		// skills.toml and skills.lock, and links it again itself.
		o, err := ses.RestoreEntry(id)
		if o == nil {
			return nil, err
		}
		if err = checkApply(o.Apply, err); err != nil {
			return nil, err
		}
		return object{"skills": nonNilList(o.Skills), "repo": o.Repo, "selected": o.Selected, "linked": o.Apply != nil, "unused": o.Unused, "hint": o.Hint, "absent": absentOf(o.Apply)}, nil
	case "plan":
		entries, err := ses.Resolve()
		if err != nil {
			return nil, err
		}
		out := []object{}
		for _, c := range ses.Plan(entries, false).ChangeList() {
			action := map[string]string{"create": "link", "replace": "relink", "remove": "unlink", "forget": "unlink", "skip": "blocked"}[c.Action]
			out = append(out, object{"action": action, "skill": c.Name, "agent": c.Target, "path": c.Path, "reason": c.Reason})
		}
		return out, nil
	case "apply":
		opts, err := argument[struct {
			Force bool `json:"force"`
		}](args, 0)
		if err != nil {
			return nil, err
		}
		o, err := ses.Apply(app.ApplyOptions{Force: opts.Force})
		if err = checkApply(o, err); err != nil {
			return nil, err
		}
		c := o.Counts()
		return object{"linked": c["create"] + c["replace"], "unlinked": c["remove"], "trashed": len(o.Backups), "absent": absentOf(o)}, nil
	case "setDefaultAgents":
		names, err := argument[[]string](args, 0)
		if err != nil {
			return nil, err
		}
		o, err := ses.SetDefaultAgents(names)
		return nil, checkApply(o, err)
	case "setAgentEnabled":
		name, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		on, err := argument[bool](args, 1)
		if err != nil {
			return nil, err
		}
		o, err := ses.SetAgentEnabled(name, on)
		return nil, checkApply(o, err)
	case "enable", "disable", "setTargets":
		r, err := argument[ref](args, 0)
		if err != nil {
			return nil, err
		}
		names, err := argument[[]string](args, 1)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if _, err = ses.Reg.Get(n); err != nil {
				return nil, err
			}
		}
		var o *app.TargetsOutcome
		switch method {
		case "enable":
			o, err = ses.Enable(r.app(), names)
		case "disable":
			o, err = ses.Disable(r.app(), names)
		default:
			o, err = ses.SetTargets(r.app(), names)
		}
		if err != nil {
			return nil, err
		}
		if err = checkApply(o.Apply, nil); err != nil {
			return nil, err
		}
		return object{"absent": absentOf(o.Apply)}, nil
	case "remove":
		r, err := argument[ref](args, 0)
		if err != nil {
			return nil, err
		}
		o, err := ses.Remove(r.app())
		if err != nil {
			return nil, err
		}
		if o.Purged != nil {
			return nil, checkApply(o.Purged.Apply, nil)
		}
		return nil, checkApply(o.Apply, nil)
	case "purge":
		repo, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		o, err := ses.Purge(repo)
		if err != nil {
			return nil, err
		}
		return nil, checkApply(o.Apply, nil)
	case "fork":
		r, err := argument[ref](args, 0)
		if err != nil {
			return nil, err
		}
		opts, err := argument[struct {
			Private bool
			Name    string
		}](args, 1)
		if err != nil {
			return nil, err
		}
		o, err := ses.Fork(r.app(), opts.Private, opts.Name)
		if err != nil {
			return nil, err
		}
		if err = checkApply(o.Apply, nil); err != nil {
			return nil, err
		}
		return findSkill(ses, app.EntryRef(app.NewKind(opts.Private), "", filepath.Base(o.Path)))
	case "newSkill":
		opts, err := argument[struct {
			Name        string
			Description string
			Private     bool
			Agents      []string
		}](args, 0)
		if err != nil {
			return nil, err
		}
		for _, n := range opts.Agents {
			if _, err = ses.Reg.Get(n); err != nil {
				return nil, err
			}
		}
		o, err := ses.NewForTargetsWithDescription(opts.Name, opts.Private, opts.Agents, opts.Description)
		if err != nil {
			return nil, err
		}
		if err = checkApply(o.Apply, nil); err != nil {
			return nil, err
		}
		r := app.EntryRef(app.NewKind(opts.Private), "", opts.Name)
		return findSkill(ses, r)
	case "revealLibrary":
		if s.native.Reveal == nil {
			return nil, errors.New("Finder unavailable")
		}
		return nil, s.native.Reveal(ses.Lib.Root)
	case "reveal", "openInEditor", "files", "readFile", "writeFile":
		return s.fileOperation(ses, method, args)
	case "parseSource", "show", "add", "update", "updateDiff":
		return s.packOperation(ctx, ses, method, args)
	case "scan", "adopt":
		return s.scanOperation(ses, method, args)
	case "sync":
		opts, err := argument[struct {
			Remote  string
			Message string
		}](args, 0)
		if err != nil {
			return nil, err
		}
		return ses.Backup(opts.Remote, opts.Message)
	default:
		return nil, fmt.Errorf("unknown desktop operation %q", method)
	}
}

func findSkill(s *app.Session, r app.Ref) (any, error) {
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	e, err := s.Entry(entries, r)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, errors.New("skill no longer exists")
	}
	return skill(*e), nil
}
func expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		h, _ := os.UserHomeDir()
		if p == "~" {
			return h
		}
		return filepath.Join(h, strings.TrimPrefix(p, "~/"))
	}
	return p
}

// bundledSkill reports whether the skilldots skill would come from this
// app's own copy (no network needed) rather than from GitHub.
func bundledSkill() bool {
	return app.BundledSelfSkill != nil && app.BundledSelfSkill() != ""
}

func (s *Service) status() (any, error) {
	root := s.root
	if root == "" {
		root = config.Get(config.Library)
	}
	if root == "" {
		// No library yet: the first-run screen still says how many skills
		// the agents here have that a new library could adopt (read only).
		unmanaged := 0
		if rep, err := app.ScanWithoutLibrary(); err == nil {
			unmanaged = len(rep.Unmanaged())
		}
		return object{"root": nil, "branch": "", "changes": 0, "unmanaged": unmanaged, "bundledSkill": bundledSkill()}, nil
	}
	ses, err := app.Open(root)
	if err != nil {
		return nil, err
	}
	branch, _ := ses.Lib.Git("branch", "--show-current")
	remote, _ := ses.Lib.Git("remote", "get-url", "origin")
	st, err := ses.Lib.Git("status", "--porcelain")
	if err != nil {
		return nil, err
	}
	changes := 0
	if st != "" {
		changes = len(strings.Split(st, "\n"))
	}
	if n, e := ses.Lib.Git("rev-list", "--count", "@{upstream}..HEAD"); e == nil {
		v, _ := strconv.Atoi(strings.TrimSpace(n))
		changes += v
	}
	rep, err := ses.Scan()
	if err != nil {
		return nil, err
	}
	return object{"root": ses.Lib.Root, "branch": branch, "remote": remote, "changes": changes, "unmanaged": len(rep.Unmanaged()), "selfSkill": ses.HasSelfSkill(), "bundledSkill": bundledSkill()}, nil
}
func outdated(o *app.Outdated) any {
	if o == nil {
		return nil
	}
	ps := []object{}
	for _, p := range o.Packs {
		commits := []object{}
		for _, c := range p.Commits {
			commits = append(commits, object{"id": c.Hash, "title": c.Subject})
		}
		ps = append(ps, object{"repo": p.Repo, "commit": p.Commit, "latest": p.Latest, "changed": p.Changed, "moved": p.Moved, "new": p.New, "gone": p.Gone, "pending": p.Pending, "checked": p.Checked, "commits": commits, "moreCommits": p.MoreCommits, "commitsUnknown": p.CommitsUnknown})
	}
	return object{"checked": o.Checked, "packs": ps, "failed": o.Failed}
}
func history(rows []app.UpdateRecord) []object {
	return historyWithBackups(rows, nil)
}
func historyWithBackups(rows []app.UpdateRecord, backups []string) []object {
	out := []object{}
	for _, r := range rows {
		cs := []object{}
		for _, c := range r.Commits {
			cs = append(cs, object{"id": c.Hash, "title": c.Subject})
		}
		rowBackups := append([]string{}, r.Backups...)
		if len(rowBackups) == 0 && len(out) == 0 {
			rowBackups = append(rowBackups, backups...)
		}
		out = append(out, object{"id": r.Time.String() + r.Repo + r.To, "when": r.Time, "repo": r.Repo, "from": r.From, "to": r.To, "changed": r.Changed, "added": r.Added, "gone": r.Gone, "moved": r.Moved, "dropped": r.Dropped, "new": r.New, "commits": cs, "moreCommits": r.MoreCommits, "commitsUnknown": r.CommitsUnknown, "applyError": r.ApplyError, "omitted": r.Omitted, "backups": rowBackups})
	}
	return out
}

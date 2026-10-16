package tui

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/scan"
	"github.com/Maybe404/skilldots/internal/targets"
)

// Ops is everything the dashboard can do to a library. Each method opens
// the library afresh, runs one shared flow from internal/app (the same
// code the CLI commands run) and returns a one line summary. Nothing here
// deletes, except EmptyTrash, which the dashboard offers only after two
// confirmations.
type Ops interface {
	Load() (*Snapshot, error)
	// SetTargets sets the agents for a skill (s non-nil) or a pack.
	SetTargets(s *Skill, repo string, agents []string) (string, error)
	// Enable links a skill into the default agents (skd enable).
	Enable(s Skill) (string, error)
	// Disable unlinks a skill from every agent (skd disable).
	Disable(s Skill) (string, error)
	// Browse fetches a repository and lists its skills without installing.
	Browse(src string) (*Browse, error)
	// Add subscribes to src (if needed) and vendors what req selects.
	Add(src string, req AddRequest) (string, error)
	// PlanUpdate checks upstream for the packs (every pack when none) and
	// says what updating them would change, changing nothing.
	PlanUpdate(repos []string) (*app.UpdatePlan, error)
	Update(repos []string) ([]*engine.SyncReport, string, error)
	// Check fetches every pack's upstream into the cache and compares it
	// with what is vendored, changing nothing (skd outdated).
	// It stops, with ctx's error, when ctx is done.
	Check(ctx context.Context) (*app.Outdated, error)
	// Fork copies a skill into skills/ or private/ as name ("" keeps its name).
	Fork(s Skill, private bool, name string) (string, error)
	// Trash takes a skill out of use: a vendored skill is deselected and
	// its files go to the trash; anything else is unlinked and kept.
	Trash(s Skill) (string, error)
	Purge(repo string) (string, error)
	Apply() (string, error)
	Sync() (string, error)
	// New scaffolds a skill of your own and links it.
	New(name string, private bool) (string, error)
	// Scan inventories the agent directories and proposes adoptions.
	Scan() (*ScanResult, error)
	// Adopt carries out the accepted proposals.
	Adopt(props []app.Proposal) (string, error)
	// TrashList lists the library trash, newest first.
	TrashList() ([]engine.TrashEntry, error)
	// Restore puts a trash entry back where it came from.
	Restore(id string) (string, error)
	// EmptyTrash permanently deletes the trash.
	EmptyTrash() (string, error)
	// History lists the recorded updates, newest first, of one pack or
	// of all ("") (skd history). It never touches the network.
	History(pack string) ([]app.UpdateRecord, error)
	// Edit opens a skill's directory in the editor (skd edit): opts fork
	// a vendored or app skill first, or force opening its own files. A
	// windowed editor is started here; a terminal editor is returned for
	// the dashboard to run in the foreground.
	Edit(s Skill, opts app.EditOptions) (msg string, foreground *exec.Cmd, err error)
	// Settings lists the user settings (skd config list).
	Settings() ([]Setting, error)
	// SetSetting changes one (skd config set; "" unsets it).
	SetSetting(key, value string) (string, error)
	// Targets lists the agent targets: those in use, and the custom ones
	// of skills.toml that are not (skd targets).
	Targets() ([]TargetRow, error)
	// TrustTarget approves the custom target the person was shown as t
	// (its path in skills.toml, directory and mode); typed is the name
	// they typed to confirm one whose directory is not named skills
	// (skd targets trust).
	TrustTarget(t TargetRow, typed string) (string, error)
}

// TargetRow is one agent target. A custom target not in use has Active
// false, and Problem says why; Approvable when a person can approve it.
type TargetRow struct {
	Name string
	// Path is the directory (links resolved for a custom target not in
	// use); Raw is the path as skills.toml has it.
	Path, Raw, Mode string
	Builtin, Active bool
	Approvable      bool
	Problem         string
}

// Setting is one user setting with its current value.
type Setting struct {
	config.Key
	Value string
}

// AddRequest is what the add dialog collected: skills by name (a whole
// category is its skills), or everything; an optional alias and agents.
type AddRequest struct {
	Names  []string
	All    bool
	Alias  string
	Agents []string
}

// Browse is a fetched repository's skill list.
type Browse struct {
	Repo   string
	Commit string
	Skills []pack.Skill
	// Have marks skills already selected in the library.
	Have map[string]bool
	// Subscribed is set when the library follows the repository already;
	// Alias and Targets are then the pack's (Targets nil: the default).
	Subscribed bool
	Alias      string
	Targets    []string
	// Preselect marks the skills the pasted text named (a link, --skill);
	// Missing are the names or paths it gave that matched nothing.
	Preselect map[string]bool
	Missing   []string
}

// ScanResult is an inventory plus what adopting it would do.
type ScanResult struct {
	Report    scan.Report
	Proposals []app.Proposal
}

// libOps implements Ops on a library directory.
type libOps struct {
	root string
}

// NewOps returns the Ops the dashboard uses for the library at root.
func NewOps(root string) Ops { return &libOps{root: root} }

func (o *libOps) Load() (*Snapshot, error) { return LoadSnapshot(o.root) }

func (o *libOps) open() (*app.Session, error) { return app.Open(o.root) }

func ref(s Skill) app.Ref { return app.EntryRef(s.Kind, s.Origin, s.Name) }

// applySummary is a one-line account of an apply for the status line.
func applySummary(out *app.ApplyOutcome) string {
	if out == nil {
		return ""
	}
	n := out.Counts()
	var parts []string
	if n["create"]+n["replace"]+n["remove"] > 0 {
		parts = append(parts, fmt.Sprintf("+%d ↻%d −%d", n["create"], n["replace"], n["remove"]))
	}
	if n["skip"] > 0 {
		var why []string
		for _, op := range out.Plan.Ops {
			if op.Action == "skip" && len(why) < 2 {
				why = append(why, op.Target.Name+"/"+op.Name+": "+op.Reason)
			}
		}
		parts = append(parts, fmt.Sprintf("%d skipped (%s; skd apply -n shows all)", n["skip"], strings.Join(why, "; ")))
	}
	if len(out.Backups) > 0 {
		parts = append(parts, fmt.Sprintf("%d moved to trash", len(out.Backups)))
	}
	if len(out.Refused) > 0 {
		parts = append(parts, fmt.Sprintf("%d not changed: %s", len(out.Refused), out.Refused[0]))
	}
	return strings.Join(parts, " · ")
}

func withSummary(msg string, out *app.ApplyOutcome) string {
	if sum := applySummary(out); sum != "" {
		return msg + " · " + sum
	}
	return msg
}

func (o *libOps) Apply() (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	out, err := s.Apply(app.ApplyOptions{})
	if err != nil {
		return "", err
	}
	if sum := applySummary(out); sum != "" {
		return "applied " + sum, nil
	}
	return "already up to date", nil
}

func targetsMsg(out *app.TargetsOutcome) string {
	return withSummary(out.Label+" → "+joinOr(out.Targets, "no agents (kept in library)"), out.Apply)
}

func (o *libOps) SetTargets(sk *Skill, repo string, agents []string) (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	r := app.PackRef(repo)
	if sk != nil {
		r = ref(*sk)
	}
	out, err := s.SetTargets(r, agents)
	if err != nil {
		return "", err
	}
	return targetsMsg(out), nil
}

func (o *libOps) Enable(sk Skill) (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	out, err := s.Enable(ref(sk), nil)
	if err != nil {
		return "", err
	}
	return targetsMsg(out), nil
}

func (o *libOps) Disable(sk Skill) (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	out, err := s.Disable(ref(sk), nil)
	if err != nil {
		return "", err
	}
	return targetsMsg(out), nil
}

func (o *libOps) Browse(arg string) (*Browse, error) {
	s, err := o.open()
	if err != nil {
		return nil, err
	}
	src, hint, err := s.ParseAddText(arg, "")
	if err != nil {
		return nil, err
	}
	f, err := s.Fetch(src)
	if err != nil {
		return nil, err
	}
	if len(f.Skills) == 0 {
		if len(f.Rejected) > 0 {
			return nil, fmt.Errorf("%s: no usable SKILL.md (%s)", src.Repo, f.Rejected[0])
		}
		return nil, fmt.Errorf("%s contains no SKILL.md", src.Repo)
	}
	b := &Browse{Repo: src.Repo, Commit: f.Commit, Skills: f.Skills, Have: f.Have, Subscribed: f.Subscribed, Preselect: map[string]bool{}}
	pre, missing := f.Preselect(hint)
	for _, n := range pre {
		b.Preselect[n] = true
	}
	b.Missing = missing
	if p := s.Lib.Manifest.FindPack(src.Repo); p != nil {
		b.Alias = p.Alias
		if p.Targets != nil {
			b.Targets = append([]string{}, *p.Targets...)
		}
	}
	return b, nil
}

// Add mirrors `skd add <src> --skill a,b [--all] [--alias x] [--agent y]`;
// src is anything the add dialog accepts (see app.ParseAddText).
func (o *libOps) Add(arg string, req AddRequest) (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	src, _, err := s.ParseAddText(arg, req.Alias)
	if err != nil {
		return "", err
	}
	f, err := s.Fetch(src)
	if err != nil {
		return "", err
	}
	out, err := s.Add(f, app.Selection{Skills: req.Names, All: req.All, Alias: req.Alias, Agents: req.Agents})
	if err != nil {
		return "", err
	}
	if out.Nothing {
		return "nothing selected", nil
	}
	rep := out.Report
	msg := fmt.Sprintf("vendored %d from %s @ %s", len(rep.Added)+len(rep.Changed)+len(rep.Unchanged), out.Repo, short(rep.NewCommit))
	if len(rep.Renamed) > 0 {
		var renamed []string
		for n, as := range rep.Renamed {
			renamed = append(renamed, n+" as "+as)
		}
		sort.Strings(renamed)
		msg += " · name taken, linked " + strings.Join(renamed, ", ")
	}
	if rep.NoLicense {
		msg += " · no license found: check before redistributing"
	}
	return withSummary(msg, out.Apply), nil
}

// Update mirrors `skd update [pack…]`.
func (o *libOps) Update(repos []string) ([]*engine.SyncReport, string, error) {
	s, err := o.open()
	if err != nil {
		return nil, "", err
	}
	if len(repos) == 0 && len(s.Lib.Manifest.Packs) == 0 {
		return nil, "no packs to update", nil
	}
	out, err := s.Update(repos, nil)
	if out == nil {
		return nil, "", err
	}
	reports := out.Reports
	if err != nil {
		return reports, "", err
	}
	var changed []string
	added, fresh := 0, 0
	for _, r := range reports {
		changed = append(changed, r.Changed...)
		added += len(r.Added)
		fresh += len(r.UpstreamNew)
	}
	what := "0 changed"
	if len(changed) > 0 {
		sort.Strings(changed)
		what = "changed " + strings.Join(changed, ", ")
		if len(changed) > 6 {
			what = fmt.Sprintf("changed %s … %d more", strings.Join(changed[:6], ", "), len(changed)-6)
		}
	}
	msg := withSummary(fmt.Sprintf("updated %d pack(s): %s · %d added · %d new upstream", len(reports), what, added, fresh), out.Apply)
	if len(out.History) > 0 {
		msg += " · h shows what changed"
	}
	if len(out.Failed) > 0 {
		var failed []string
		for _, f := range out.Failed {
			failed = append(failed, f.Repo+": "+f.Error)
		}
		return reports, msg, fmt.Errorf("%s", strings.Join(failed, "; "))
	}
	return reports, msg, nil
}

// PlanUpdate is the check `skd update` runs at a terminal before asking.
func (o *libOps) PlanUpdate(repos []string) (*app.UpdatePlan, error) {
	s, err := o.open()
	if err != nil {
		return nil, err
	}
	return s.PlanUpdate(context.Background(), repos, nil)
}

// Check mirrors `skd outdated`.
func (o *libOps) Check(ctx context.Context) (*app.Outdated, error) {
	s, err := o.open()
	if err != nil {
		return nil, err
	}
	return s.OutdatedContext(ctx, nil, nil)
}

// Fork mirrors `skd fork <skill> [--private] [--as name]`.
func (o *libOps) Fork(sk Skill, private bool, name string) (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	out, err := s.Fork(ref(sk), private, name)
	if err != nil {
		return "", err
	}
	return withSummary("forked "+out.From.LinkName+" → "+s.Lib.Rel(out.Path), out.Apply), nil
}

// Trash mirrors `skd remove` for vendored skills and `skd disable` for
// everything else (the CLI never deletes your own or an app's files).
func (o *libOps) Trash(sk Skill) (string, error) {
	if sk.Kind != engine.KindVendor {
		if _, err := o.Disable(sk); err != nil {
			return "", err
		}
		return "unlinked " + sk.LinkName + " from every agent; its files stay in " + tildify(sk.Dir), nil
	}
	s, err := o.open()
	if err != nil {
		return "", err
	}
	out, err := s.Remove(ref(sk))
	if err != nil {
		return "", err
	}
	if out.Purged != nil {
		return out.Purged.Repo + " had no skills left · " + purgeMsg(out.Purged), nil
	}
	return withSummary("removed "+out.Entry.LinkName+"; its files are in the library trash (t)", out.Apply), nil
}

func purgeMsg(out *app.PurgeOutcome) string {
	return withSummary("purged "+out.Repo+"; vendor/"+out.Repo+" is in the library trash (t)", out.Apply)
}

// Purge mirrors `skd purge <pack>` once the user has confirmed.
func (o *libOps) Purge(repo string) (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	out, err := s.Purge(repo)
	if err != nil {
		return "", err
	}
	return purgeMsg(out), nil
}

// Sync mirrors `skd sync`.
func (o *libOps) Sync() (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	out, err := s.Backup("", "")
	if err != nil {
		return "", err
	}
	var parts []string
	if out.Committed {
		parts = append(parts, "committed")
	} else {
		parts = append(parts, "nothing to commit")
	}
	if out.Pushed {
		parts = append(parts, "pushed to "+tildifyAll(out.Remote))
	}
	if out.PrivateUncommitted {
		parts = append(parts, "private/ has uncommitted changes (a separate repo, never pushed here)")
	}
	msg := strings.Join(parts, " · ")
	if out.Remote == "" {
		// The status line has room for one problem: say it first.
		return "", warning("no remote configured; add one with: skd sync --remote <git-url> · " + msg)
	}
	return msg, nil
}

// New mirrors `skd new <name> [--private]`.
func (o *libOps) New(name string, private bool) (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	out, err := s.New(name, private)
	if err != nil {
		return "", err
	}
	return withSummary("created "+tildify(out.Path), out.Apply), nil
}

// Scan mirrors `skd scan` and `skd adopt --dry-run`.
func (o *libOps) Scan() (*ScanResult, error) {
	s, err := o.open()
	if err != nil {
		return nil, err
	}
	rep, err := s.Scan()
	if err != nil {
		return nil, err
	}
	return &ScanResult{Report: rep, Proposals: s.Proposals(rep)}, nil
}

// Adopt mirrors `skd adopt` with the proposals the user accepted.
func (o *libOps) Adopt(props []app.Proposal) (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	var warns []string
	s.Log = func(l app.Level, msg string) {
		if l == app.Warn || l == app.Fail {
			warns = append(warns, msg)
		}
	}
	out, err := s.Adopt(props)
	msg := fmt.Sprintf("adopted %d", len(props))
	if out != nil {
		msg = withSummary(msg, out.Apply)
	}
	if err != nil {
		if len(warns) > 0 {
			return "", fmt.Errorf("%v · %s", err, strings.Join(warns, " · "))
		}
		return "", err
	}
	if len(warns) > 0 {
		return "", warning(msg + " · " + strings.Join(warns, " · "))
	}
	return msg, nil
}

func (o *libOps) TrashList() ([]engine.TrashEntry, error) {
	s, err := o.open()
	if err != nil {
		return nil, err
	}
	return s.Trash()
}

func (o *libOps) Restore(id string) (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	dest, err := s.Restore(id)
	if err != nil {
		return "", err
	}
	// Like `skd trash restore`, nothing is re-linked: the header shows
	// what `A` would apply.
	return "restored " + tildify(dest), nil
}

func (o *libOps) EmptyTrash() (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	n, err := s.EmptyTrash()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("deleted %d item(s) from the trash for good", n), nil
}

func (o *libOps) History(p string) ([]app.UpdateRecord, error) {
	s, err := o.open()
	if err != nil {
		return nil, err
	}
	recs, skipped, err := s.History(p, 0)
	if err == nil && skipped > 0 {
		return recs, warning(fmt.Sprintf("%s: %d line(s) could not be read and were skipped", app.HistoryFile, skipped))
	}
	return recs, err
}

// Edit mirrors `skd edit <skill> [--fork [--private] [--as]] [--force]`.
func (o *libOps) Edit(sk Skill, opts app.EditOptions) (string, *exec.Cmd, error) {
	s, err := o.open()
	if err != nil {
		return "", nil, err
	}
	out, err := s.Edit(ref(sk), opts)
	if err != nil {
		return "", nil, err
	}
	msg := "opened " + tildify(out.Dir) + " in " + out.Editor.Name()
	if out.Fork != nil {
		msg = withSummary("forked "+out.Entry.LinkName+" → "+s.Lib.Rel(out.Fork.Path)+" · "+msg, out.Fork.Apply)
	}
	if out.Editor.Terminal {
		return msg, out.Editor.Command(out.Dir), nil
	}
	if err := out.Editor.Start(out.Dir); err != nil {
		return "", nil, fmt.Errorf("%s: %w", out.Editor.Name(), err)
	}
	return msg, nil, nil
}

func (o *libOps) Settings() ([]Setting, error) {
	all, err := config.Load()
	if err != nil {
		return nil, err
	}
	var out []Setting
	for _, k := range config.Keys {
		out = append(out, Setting{Key: k, Value: all[k.Name]})
	}
	return out, nil
}

func (o *libOps) Targets() ([]TargetRow, error) {
	s, err := o.open()
	if err != nil {
		return nil, err
	}
	out := []TargetRow{}
	for _, t := range s.Reg.All() {
		raw := t.Path
		if c, ok := s.Lib.Manifest.Targets.Custom[t.Name]; ok && !t.Builtin {
			raw = c.Path
		}
		out = append(out, TargetRow{Name: t.Name, Path: t.Path, Raw: raw, Mode: t.Mode, Builtin: t.Builtin, Active: true})
	}
	for _, w := range s.Reg.Withheld() {
		out = append(out, TargetRow{Name: w.Name, Path: w.Real, Raw: w.Path, Mode: w.Mode, Approvable: w.Approvable, Problem: w.Problem})
	}
	return out, nil
}

func (o *libOps) TrustTarget(t TargetRow, typed string) (string, error) {
	s, err := o.open()
	if err != nil {
		return "", err
	}
	if err := s.Reg.Approve(targets.Withheld{Name: t.Name, Path: t.Raw, Real: t.Path, Mode: t.Mode}, typed); err != nil {
		return "", err
	}
	return "approved custom target " + t.Name + " → " + t.Path + " · A applies", nil
}

func (o *libOps) SetSetting(key, value string) (string, error) {
	if err := config.Set(key, value); err != nil {
		return "", err
	}
	msg := key + " = " + value
	if value == "" {
		msg = key + " unset"
	}
	switch key {
	case config.Theme:
		msg += " · takes effect the next time skd starts"
	case config.Lang:
		msg += " · reserved: only English exists so far"
	}
	return msg, nil
}

// warning is an error that is only worth a yellow status line: the
// operation did what it could.
type warning string

func (w warning) Error() string { return string(w) }

func short(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func joinOr(items []string, empty string) string {
	if len(items) == 0 {
		return empty
	}
	c := append([]string{}, items...)
	sort.Strings(c)
	return strings.Join(c, ", ")
}

package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
)

// Contributing sends the user's edits to a vendored skill back to its
// upstream repository: pushed to the pack's branch when the user may push
// there (--push), or as a pull request from a branch (--pr; GitHub only,
// through gh, from a fork when the user may not push). Nothing is ever
// sent unless a person asked for it: the CLI wants --yes (or a yes at a
// terminal), the desktop app a confirmation that names the repository,
// the branch and the diff.
//
// The change is made in a clone of its own in the cache
// (<cache>/<repo>.contrib), never in the shallow clone updates use: it is
// reset to upstream's tip, the skill's files are mirrored over the
// skill's upstream directory, and that is committed with the user's own
// git identity.

// Modes of a contribution (ContributeOptions, Contribute).
const (
	ModePR   = "pr"
	ModePush = "push"
)

// Hosts (ContributeOptions.Host).
const (
	HostGitHub = "github"
	HostOther  = "other"
)

// ContributeOptions is what contributing a skill upstream can do, worked
// out before anything is sent: the desktop shows it in its confirmation,
// `skd contribute --dry-run` prints it.
type ContributeOptions struct {
	// Repo is the pack's repository; Branch the branch a push goes to
	// (and a pull request into): the pack's ref, else upstream's default.
	Repo   string `json:"repo"`
	Host   string `json:"host"`
	Branch string `json:"branch"`
	// CanPush and CanPR say which modes are possible; Reason says why one
	// is not (or why neither is, Behind).
	CanPush bool   `json:"canPush"`
	CanPR   bool   `json:"canPR"`
	GH      string `json:"gh"`
	// Behind is set when a skill is not based on upstream's current
	// version (or its update is shelved): skd update first. For the
	// user's own repository (Own) Contribute pulls it first (Pull).
	Behind         bool   `json:"behind"`
	Reason         string `json:"reason"`
	DefaultMessage string `json:"defaultMessage"`
	// Own is set for the user's own repository (manifest.Pack.Own): a
	// push is the default, and Contribute pulls before (Pull, when
	// Behind) and after pushing.
	Own  bool `json:"own"`
	Pull bool `json:"pull"`
	// Skills are the link names of the skills sent, Drafts those of them
	// that are new (skd new --in), with Paths their directories in the
	// repository.
	Skills []string          `json:"skills"`
	Drafts []string          `json:"drafts"`
	Paths  map[string]string `json:"paths,omitempty"`

	plan *contribPlan
}

// contribPlan is what Contribute needs beyond the options.
type contribPlan struct {
	pack        *manifest.Pack
	items       []contribItem
	url         string // the repository's git URL
	owner, name string // on GitHub
	clone       string // the work clone
}

// contribItem is one skill of a contribution.
type contribItem struct {
	entry     engine.Entry
	ls        *lock.Skill
	skillPath string // the skill's directory in the repository
	draft     bool
}

// github.com URLs: https://github.com/o/r(.git), git@github.com:o/r(.git),
// ssh://git@github.com/o/r(.git).
var githubURL = regexp.MustCompile(`^(?:https?://(?:[^@/]+@)?github\.com/|git@github\.com:|ssh://git@github\.com/)([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)

// DefaultContributionMessage is the commit message of a contribution
// when none is given. It is data (a commit), never translated.
func DefaultContributionMessage(skill string) string {
	return "Update " + skill + "\n\nEdited with skilldots"
}

// defaultMessage is the commit message of a contribution of the skills
// updated and added (drafts): "Add c; update a, b".
func defaultMessage(updated, added []string) string {
	if len(added) == 0 && len(updated) == 1 {
		return DefaultContributionMessage(updated[0])
	}
	var parts []string
	if len(added) > 0 {
		parts = append(parts, "Add "+strings.Join(added, ", "))
	}
	if len(updated) > 0 {
		parts = append(parts, "update "+strings.Join(updated, ", "))
	}
	if len(parts) > 0 {
		parts[0] = strings.ToUpper(parts[0][:1]) + parts[0][1:]
	}
	return strings.Join(parts, "; ") + "\n\nEdited with skilldots"
}

// contribClone is where a pack's contributions are made.
func contribClone(repo string) string { return pack.CachePath(repo) + ".contrib" }

// ContributeOptions works out how the user's edits to a vendored skill
// could be sent upstream: it fetches upstream's tip into the work clone
// (network), checks the skill is based on it, and asks gh what the user
// may do. Nothing is sent.
func (s *Session) ContributeOptions(ctx context.Context, r Ref) (*ContributeOptions, error) {
	p, items, err := s.contribSkill(r)
	if err != nil {
		return nil, err
	}
	return s.contribOptions(ctx, p, items)
}

// ContributePackOptions is ContributeOptions for every skill of a pack
// the user changed, and every draft made in it (skd contribute <pack>).
func (s *Session) ContributePackOptions(ctx context.Context, packName string) (*ContributeOptions, error) {
	p, items, err := s.contribPack(packName)
	if err != nil {
		return nil, err
	}
	return s.contribOptions(ctx, p, items)
}

// contribSkill is the one skill r names, as a contribution.
func (s *Session) contribSkill(r Ref) (*manifest.Pack, []contribItem, error) {
	entries, err := s.Resolve()
	if err != nil {
		return nil, nil, err
	}
	e, err := s.mustEntry(entries, r)
	if err != nil {
		return nil, nil, err
	}
	ls := s.vendoredLock(e)
	if ls == nil {
		return nil, nil, i18n.Errorf("%q is not a vendored skill; only changes to a skill from an upstream repository are contributed", e.LinkName)
	}
	if !s.edited(e) {
		return nil, nil, i18n.Errorf("%s has no changes of yours to contribute (skd diff %s shows them)", e.LinkName, e.LinkName)
	}
	p := s.Lib.Manifest.FindPack(e.Origin)
	if p == nil {
		return nil, nil, i18n.Errorf("no pack %q", e.Origin)
	}
	return p, []contribItem{{entry: *e, ls: ls, draft: ls.IsDraft()}}, nil
}

// contribPack is every skill of a pack the user changed, drafts included.
func (s *Session) contribPack(packName string) (*manifest.Pack, []contribItem, error) {
	p := s.Lib.Manifest.FindPack(packName)
	if p == nil {
		return nil, nil, i18n.Errorf("no pack %q", packName)
	}
	entries, err := s.Resolve()
	if err != nil {
		return nil, nil, err
	}
	var items []contribItem
	for i := range entries {
		e := &entries[i]
		if e.Kind != engine.KindVendor || e.Origin != p.Repo || !s.edited(e) {
			continue
		}
		ls := s.vendoredLock(e)
		items = append(items, contribItem{entry: *e, ls: ls, draft: ls.IsDraft()})
	}
	if len(items) == 0 {
		return nil, nil, i18n.Errorf("%s has no changes of yours to contribute: no skill of it is changed, and no new one was made in it (skd list %s marks them)", p.Repo, p.Repo)
	}
	return p, items, nil
}

// contribOptions works out the options of contributing items, skills
// of p: see ContributeOptions.
func (s *Session) contribOptions(ctx context.Context, p *manifest.Pack, items []contribItem) (*ContributeOptions, error) {
	src, err := pack.ParseSource(p.Repo)
	if err != nil {
		return nil, err
	}
	o := &ContributeOptions{Repo: p.Repo, Host: HostOther, GH: GHMissing, Own: p.Own, Skills: []string{}, Drafts: []string{},
		plan: &contribPlan{pack: p, url: src.URL, clone: contribClone(p.Repo)}}
	if m := githubURL.FindStringSubmatch(src.URL); m != nil {
		o.Host, o.plan.owner, o.plan.name = HostGitHub, m[1], m[2]
		o.GH = gh().Status(ctx)
	} else if _, err := exec.LookPath("gh"); err == nil {
		// Not asked whether it is logged in: elsewhere than GitHub it
		// plays no part.
		o.GH = GHOK
	}
	unlock, err := pack.HoldContext(ctx, p.Repo+".contrib")
	defer unlock()
	if err != nil {
		return nil, err
	}
	if o.Branch, err = s.prepareClone(ctx, o.plan.clone, src.URL, p.Ref); err != nil {
		return nil, err
	}
	up, _, err := pack.Scan(o.plan.clone)
	if err != nil {
		return nil, err
	}
	var reasons []string
	var behind, updated, added []string
	for _, it := range items {
		e := it.entry
		o.Skills = append(o.Skills, e.LinkName)
		if it.draft {
			o.Drafts = append(o.Drafts, e.LinkName)
			added = append(added, e.Name)
			if pack.Find(up, e.Name) != nil {
				// Made in the repository meanwhile (pushed from elsewhere):
				// an update brings it, and merges it with this one.
				behind = append(behind, e.LinkName)
				continue
			}
			rel, err := draftPath(up, o.plan.clone, e.Name, it.ls.Path)
			if err != nil {
				return nil, i18n.Errorf("%s: %w", e.LinkName, err)
			}
			if fsutil.Lexists(filepath.Join(o.plan.clone, filepath.FromSlash(rel))) {
				return nil, i18n.Errorf("%s: %s already exists in %s, and is not a skill: choose another place for it (skd new %s --in %s --path <dir>, after skd remove %s)", e.LinkName, rel, p.Repo, e.Name, p.Repo, e.LinkName)
			}
			it.skillPath = rel
		} else {
			updated = append(updated, e.Name)
			sk := pack.Find(up, e.Name)
			if sk == nil {
				return nil, i18n.Errorf("%s is no longer in %s", e.Name, p.Repo)
			}
			rel, err := filepath.Rel(o.plan.clone, sk.Dir)
			if err != nil {
				return nil, err
			}
			it.skillPath = filepath.ToSlash(rel)
			h, err := fsutil.HashTree(sk.Dir)
			if err != nil {
				return nil, err
			}
			if it.ls.Shelved != "" || it.ls.Resolving || h != it.ls.Hash {
				behind = append(behind, e.LinkName)
			}
		}
		if it.skillPath == "." && len(items) > 1 {
			return nil, i18n.Errorf("%s is the whole repository %s: contribute it on its own", e.LinkName, p.Repo)
		}
		o.plan.items = append(o.plan.items, it)
	}
	o.Paths = map[string]string{}
	for _, it := range o.plan.items {
		if it.draft {
			o.Paths[it.entry.LinkName] = it.skillPath
		}
	}
	o.DefaultMessage = defaultMessage(updated, added)
	if len(behind) > 0 {
		o.Behind = true
		if p.Own {
			// Pulled first: Contribute updates the pack, merging.
			o.Pull = true
		} else if len(items) == 1 {
			reasons = append(reasons, i18n.Sprintf("%s is not based on upstream's current version: skd update %s first (and skd resolve, if it is shelved)", behind[0], p.Repo))
		} else {
			reasons = append(reasons, i18n.Sprintf("%s are not based on upstream's current version: skd update %s first (and skd resolve, if one is shelved)", strings.Join(behind, ", "), p.Repo))
		}
	}
	// Pushing: GitHub says whether the user may; elsewhere git tries.
	if o.Host == HostGitHub && o.GH == GHOK {
		o.CanPush, err = gh().CanPush(ctx, o.plan.owner, o.plan.name)
		if err != nil {
			o.CanPush = false
		}
	} else {
		_, err := library.GitContext(ctx, o.plan.clone, "push", "-q", "--dry-run", "origin", "HEAD:refs/heads/"+o.Branch)
		o.CanPush = err == nil
	}
	if !o.CanPush {
		reasons = append(reasons, i18n.Sprintf("you cannot push to %s: open a pull request (--pr)", p.Repo))
	}
	switch {
	case o.Host != HostGitHub:
		reasons = append(reasons, i18n.T("pull requests are opened on GitHub only; for another host push (--push) when you may, or send a patch: git format-patch"))
	case o.GH == GHMissing:
		reasons = append(reasons, i18n.T("a pull request needs gh, GitHub's command line, which is not installed: https://cli.github.com"))
	case o.GH == GHLoggedOut:
		reasons = append(reasons, i18n.T("a pull request needs gh to be logged in: run gh auth login"))
	default:
		o.CanPR = true
	}
	o.Reason = strings.Join(reasons, "; ")
	return o, nil
}

// draftPath is where in the repository (whose clone is at root, with
// skills up) a draft named name goes: at want when it says (skd new --in
// recorded it), else beside most of the repository's skills.
func draftPath(up []pack.Skill, root, name, want string) (string, error) {
	if want != "" {
		if err := checkRepoPath(want); err != nil {
			return "", err
		}
		return want, nil
	}
	parent, err := skillsParent(up, root)
	if err != nil {
		return "", err
	}
	return path.Join(parent, name), nil
}

// skillsParent is the directory of a repository (whose clone is at root)
// most of its skills are in, "skills" when it has none. A repository
// that is one skill at its top has no place for another.
func skillsParent(up []pack.Skill, root string) (string, error) {
	count := map[string]int{}
	for _, sk := range up {
		rel, err := filepath.Rel(root, sk.Dir)
		if err != nil {
			return "", err
		}
		if rel == "." {
			return "", i18n.New("the repository is a single skill at its top, where another skill would be nested in it and never found: give its place with --path")
		}
		count[path.Dir(filepath.ToSlash(rel))]++
	}
	best := "skills"
	for d, n := range count {
		if b := count[best]; n > b || n == b && (len(d) < len(best) || len(d) == len(best) && d < best) {
			best = d
		}
	}
	return best, nil
}

// checkRepoPath refuses a path in a repository (slash-separated, from
// skills.lock or --path) that is not a plain relative path to a
// directory skd's scan finds skills in.
func checkRepoPath(rel string) error {
	if rel == "" || path.IsAbs(rel) || strings.Contains(rel, `\`) || path.Clean(rel) != rel || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return i18n.Errorf("%q is not a relative path in the repository", rel)
	}
	if !pack.Scanned(rel) {
		return i18n.Errorf("%q is not a place skd looks for skills (a hidden, test or dependency directory)", rel)
	}
	return nil
}

// prepareClone makes dir a clone of url at the tip of ref (the default
// branch when ""), with nothing else in its work tree, and returns the
// branch.
func (s *Session) prepareClone(ctx context.Context, dir, url, ref string) (string, error) {
	if !fsutil.Exists(filepath.Join(dir, ".git")) {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return "", err
		}
		os.RemoveAll(dir)
		if _, err := library.GitContext(ctx, "", "clone", "-q", "--filter=blob:none", "--no-checkout", "--", url, dir); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	} else if _, err := library.GitContext(ctx, dir, "remote", "set-url", "--", "origin", url); err != nil {
		return "", err
	}
	branch := ref
	if branch == "" {
		out, err := library.GitContext(ctx, dir, "ls-remote", "--symref", "origin", "HEAD")
		if err != nil {
			return "", err
		}
		for _, l := range strings.Split(out, "\n") {
			if rest, ok := strings.CutPrefix(l, "ref: refs/heads/"); ok {
				branch, _, _ = strings.Cut(rest, "\t")
			}
		}
		if branch == "" {
			return "", i18n.Errorf("cannot tell the default branch of %s", url)
		}
	} else if out, err := library.GitContext(ctx, dir, "ls-remote", "--heads", "origin", "refs/heads/"+ref); err != nil {
		return "", err
	} else if out == "" {
		return "", i18n.Errorf("the pack is pinned to %s, which is not a branch upstream: there is nothing to push to", ref)
	}
	if _, err := library.GitContext(ctx, dir, "fetch", "-q", "origin", "refs/heads/"+branch); err != nil {
		return "", err
	}
	if _, err := library.GitContext(ctx, dir, "checkout", "-q", "--force", "-B", "skilldots-contribute", "FETCH_HEAD"); err != nil {
		return "", err
	}
	if _, err := library.GitContext(ctx, dir, "clean", "-q", "-fdx"); err != nil {
		return "", err
	}
	return branch, nil
}

// ContributeOutcome is the result of Contribute; its JSON form is what
// `skd contribute --json` prints.
type ContributeOutcome struct {
	Repo   string `json:"repo"`
	Mode   string `json:"mode"`
	Branch string `json:"branch"`
	URL    string `json:"url,omitempty"`
	Number int    `json:"number,omitempty"`
	// Pushed is set once the change left this machine.
	Pushed bool `json:"pushed"`
	// Skills are the link names of the skills sent.
	Skills []string `json:"skills"`
	// Pulled is the update of the user's own repository made before
	// sending (nil when there was none), PulledBack the one after, which
	// brings what was pushed back as the version the lock records.
	// PullBackError is set when that failed: the push happened, and skd
	// update finishes it.
	Pulled        *engine.SyncReport `json:"pulled,omitempty"`
	PulledBack    *engine.SyncReport `json:"pulledBack,omitempty"`
	PullBackError string             `json:"pullBackError,omitempty"`
}

// Contribute sends the user's edits to a vendored skill upstream, in
// mode (ModePush or ModePR), with msg as the commit message (its first
// line titles a pull request; the options' DefaultMessage when empty).
// Only a person's request may call it. It records the contribution in
// skills.lock.
func (s *Session) Contribute(ctx context.Context, r Ref, mode, msg string) (*ContributeOutcome, error) {
	return s.contribute(ctx, func() (*manifest.Pack, []contribItem, error) { return s.contribSkill(r) }, mode, msg)
}

// ContributePack is Contribute for every skill of a pack the user
// changed and every draft made in it, in one commit (one pull request).
func (s *Session) ContributePack(ctx context.Context, packName, mode, msg string) (*ContributeOutcome, error) {
	return s.contribute(ctx, func() (*manifest.Pack, []contribItem, error) { return s.contribPack(packName) }, mode, msg)
}

func (s *Session) contribute(ctx context.Context, what func() (*manifest.Pack, []contribItem, error), mode, msg string) (*ContributeOutcome, error) {
	if mode != ModePR && mode != ModePush {
		return nil, i18n.Errorf("choose %s or %s, not %q", ModePR, ModePush, mode)
	}
	p, items, err := what()
	if err != nil {
		return nil, err
	}
	o, err := s.contribOptions(ctx, p, items)
	if err != nil {
		return nil, err
	}
	var pulled *engine.SyncReport
	if o.Pull {
		// The user's own repository moved on: pull it first, merging, as
		// git pull before git push.
		if pulled, err = s.pullPack(p.Repo); err != nil {
			return nil, i18n.Errorf("pulling %s before pushing: %w", p.Repo, err)
		}
		if len(pulled.Shelved) > 0 {
			return &ContributeOutcome{Repo: p.Repo, Mode: mode, Skills: []string{}, Pulled: pulled},
				i18n.Errorf("pulled %s, but %s conflict with what is there: nothing was sent; skd resolve, then contribute again", p.Repo, strings.Join(pulled.ShelvedNames(), ", "))
		}
		if p, items, err = what(); err != nil {
			return &ContributeOutcome{Repo: o.Repo, Mode: mode, Skills: []string{}, Pulled: pulled},
				i18n.Errorf("pulled %s: %w", o.Repo, err)
		}
		if o, err = s.contribOptions(ctx, p, items); err != nil {
			return nil, err
		}
	}
	pl := o.plan
	first := pl.items[0].entry.LinkName
	switch {
	case o.Behind && len(pl.items) == 1:
		return nil, i18n.Errorf("%s is not based on upstream's current version: skd update %s first (and skd resolve, if it is shelved)", first, o.Repo)
	case o.Behind:
		return nil, i18n.Errorf("not every skill is based on upstream's current version: skd update %s first (and skd resolve, if one is shelved)", o.Repo)
	case mode == ModePush && !o.CanPush:
		return nil, i18n.Errorf("you cannot push to %s; open a pull request instead: skd contribute %s --pr", o.Repo, first)
	case mode == ModePR && !o.CanPR:
		return nil, i18n.Errorf("cannot open a pull request: %s", o.Reason)
	}
	if err := s.Lib.Writable(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(msg) == "" {
		msg = o.DefaultMessage
	}
	unlock, err := pack.HoldContext(ctx, o.Repo+".contrib")
	defer unlock()
	if err != nil {
		return nil, err
	}
	// Each skill's files over its directory there, deletions too.
	for _, it := range pl.items {
		ours, err := engine.ReadFileSet(it.entry.Dir)
		if err != nil {
			return nil, err
		}
		target := filepath.Join(pl.clone, filepath.FromSlash(it.skillPath))
		if !fsutil.Within(pl.clone, target) {
			return nil, i18n.Errorf("%s is outside %s", it.skillPath, o.Repo)
		}
		if err := engine.WriteFileSet(target, ours, nil); err != nil {
			return nil, err
		}
		spec := ":(literal)" + it.skillPath
		if it.skillPath == "." {
			spec = "."
		}
		if _, err := library.GitContext(ctx, pl.clone, "add", "-A", "--", spec); err != nil {
			return nil, err
		}
	}
	if _, err := library.GitContext(ctx, pl.clone, "diff", "--cached", "--quiet"); err == nil {
		if len(pl.items) == 1 {
			return nil, i18n.Errorf("%s is the same as upstream's: nothing to contribute", first)
		}
		return nil, i18n.Errorf("these skills are the same as upstream's: nothing to contribute")
	}
	if _, err := library.GitContext(ctx, pl.clone, "commit", "-q", "-m", msg); err != nil {
		return nil, err
	}
	out := &ContributeOutcome{Repo: o.Repo, Mode: mode, Skills: o.Skills, Pulled: pulled}
	c := &lock.Contribution{Kind: mode, At: time.Now().UTC().Format(time.RFC3339)}
	if mode == ModePush {
		out.Branch = o.Branch
		if _, err := library.GitContext(ctx, pl.clone, "push", "-q", "origin", "HEAD:refs/heads/"+o.Branch); err != nil {
			return nil, i18n.Errorf("%s refused the push to %s (%v); open a pull request instead: skd contribute %s --pr", o.Repo, o.Branch, err, first)
		}
		out.Pushed = true
		c.Branch, c.State = o.Branch, "pushed"
	} else {
		label := pl.items[0].entry.Name
		if len(pl.items) > 1 {
			label = path.Base(strings.TrimSuffix(o.Repo, ".git"))
		}
		branch := "skilldots/" + label + "-" + time.Now().Format("20060102-1504")
		out.Branch = branch
		head := branch
		if o.CanPush {
			if _, err := library.GitContext(ctx, pl.clone, "push", "-q", "origin", "HEAD:refs/heads/"+branch); err != nil {
				return nil, err
			}
		} else {
			forkOwner, forkName, err := gh().Fork(ctx, pl.owner, pl.name)
			if err != nil {
				return nil, err
			}
			if _, err := library.GitContext(ctx, pl.clone, "push", "-q", "--", forkURL(pl.url, forkOwner, forkName), "HEAD:refs/heads/"+branch); err != nil {
				return nil, err
			}
			head = forkOwner + ":" + branch
		}
		out.Pushed = true
		title, body, _ := strings.Cut(msg, "\n")
		body = strings.TrimSpace(body)
		if body != "" {
			body += "\n\n"
		}
		body += "Sent with skilldots (https://github.com/Maybe404/skilldots)."
		url, number, err := gh().CreatePR(ctx, pl.owner, pl.name, o.Branch, head, strings.TrimSpace(title), body)
		if err != nil {
			return out, i18n.Errorf("the branch %s was pushed, but opening the pull request failed: %w", branch, err)
		}
		out.URL, out.Number = url, number
		c.URL, c.Number, c.Branch, c.State = url, number, branch, "open"
	}
	lp := s.Lib.Lock.Packs[pl.pack.Repo]
	for _, it := range pl.items {
		if lp == nil || lp.Skills[it.entry.Name] == nil {
			continue
		}
		ls := lp.Skills[it.entry.Name]
		cc := *c
		ls.Contribution = &cc
		if it.draft {
			// Where it went, should it be sent again before it is pulled.
			ls.Path = it.skillPath
		}
	}
	if err := s.Lib.Save(); err != nil {
		return out, err
	}
	if p.Own && mode == ModePush {
		// Pulled back: the lock records the version just pushed, so the
		// skills are no longer changed, and nothing is reported later.
		if rep, err := s.pullPack(p.Repo); err != nil {
			out.PullBackError = err.Error()
			s.logf(Warn, "pushed to %s, but pulling it back failed (%v): skd update %s finishes it", p.Repo, err, p.Repo)
		} else {
			out.PulledBack = rep
		}
	}
	return out, nil
}

// pullPack updates one pack (skd update <pack>) and returns its report.
func (s *Session) pullPack(repo string) (*engine.SyncReport, error) {
	up, err := s.Update([]string{repo}, nil)
	if err != nil {
		return nil, err
	}
	if len(up.Failed) > 0 {
		return nil, errors.New(up.Failed[0].Error)
	}
	if len(up.Reports) == 0 {
		return nil, i18n.Errorf("no pack %q", repo)
	}
	return up.Reports[0], nil
}

// forkURL is a fork's URL in the form of the original's (https or ssh).
func forkURL(orig, owner, name string) string {
	if strings.HasPrefix(orig, "git@github.com:") {
		return "git@github.com:" + owner + "/" + name + ".git"
	}
	if strings.HasPrefix(orig, "ssh://") {
		return "ssh://git@github.com/" + owner + "/" + name + ".git"
	}
	return "https://github.com/" + owner + "/" + name + ".git"
}

// ContributionInfo is a skill with a contribution on record.
type ContributionInfo struct {
	Repo string `json:"repo"`
	Name string `json:"name"`
	lock.Contribution
}

// Contributions lists the skills with a contribution on record.
func (s *Session) Contributions() []ContributionInfo {
	out := []ContributionInfo{}
	for repo, lp := range s.Lib.Lock.Packs {
		if lp == nil {
			continue
		}
		for n, ls := range lp.Skills {
			if ls != nil && ls.Contribution != nil {
				out = append(out, ContributionInfo{Repo: repo, Name: n, Contribution: *ls.Contribution})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// RefreshContributions asks GitHub for the state of the open pull
// requests on record and saves what changed. Best effort: without gh, or
// offline, nothing changes and nothing fails; a pull request whose state
// cannot be read keeps its own.
func (s *Session) RefreshContributions(ctx context.Context) error {
	var open []*lock.Contribution
	for _, lp := range s.Lib.Lock.Packs {
		if lp == nil {
			continue
		}
		for _, ls := range lp.Skills {
			if ls == nil {
				continue
			}
			if c := ls.Contribution; c != nil && c.Kind == ModePR && c.State == "open" && c.URL != "" {
				open = append(open, c)
			}
		}
	}
	if len(open) == 0 || gh().Status(ctx) != GHOK {
		return nil
	}
	changed := false
	for _, c := range open {
		st, err := gh().PRState(ctx, c.URL)
		if err != nil || st == "" || st == c.State {
			continue
		}
		c.State, changed = st, true
	}
	if !changed {
		return nil
	}
	return s.Lib.Save()
}

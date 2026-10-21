package app

import (
	"context"
	"os"
	"os/exec"
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
	// Behind is set when the skill is not based on upstream's current
	// version (or its update is shelved): skd update first.
	Behind         bool   `json:"behind"`
	Reason         string `json:"reason"`
	DefaultMessage string `json:"defaultMessage"`

	plan *contribPlan
}

// contribPlan is what Contribute needs beyond the options.
type contribPlan struct {
	entry       engine.Entry
	ls          *lock.Skill
	url         string // the repository's git URL
	owner, name string // on GitHub
	clone       string // the work clone
	skillPath   string // the skill's directory in the repository
}

// github.com URLs: https://github.com/o/r(.git), git@github.com:o/r(.git),
// ssh://git@github.com/o/r(.git).
var githubURL = regexp.MustCompile(`^(?:https?://(?:[^@/]+@)?github\.com/|git@github\.com:|ssh://git@github\.com/)([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)

// DefaultContributionMessage is the commit message of a contribution
// when none is given. It is data (a commit), never translated.
func DefaultContributionMessage(skill string) string {
	return "Update " + skill + "\n\nEdited with skilldots"
}

// contribClone is where a pack's contributions are made.
func contribClone(repo string) string { return pack.CachePath(repo) + ".contrib" }

// ContributeOptions works out how the user's edits to a vendored skill
// could be sent upstream: it fetches upstream's tip into the work clone
// (network), checks the skill is based on it, and asks gh what the user
// may do. Nothing is sent.
func (s *Session) ContributeOptions(ctx context.Context, r Ref) (*ContributeOptions, error) {
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	e, err := s.mustEntry(entries, r)
	if err != nil {
		return nil, err
	}
	ls := s.vendoredLock(e)
	if ls == nil {
		return nil, i18n.Errorf("%q is not a vendored skill; only changes to a skill from an upstream repository are contributed", e.LinkName)
	}
	if !s.edited(e) {
		return nil, i18n.Errorf("%s has no changes of yours to contribute (skd diff %s shows them)", e.LinkName, e.LinkName)
	}
	p := s.Lib.Manifest.FindPack(e.Origin)
	if p == nil {
		return nil, i18n.Errorf("no pack %q", e.Origin)
	}
	src, err := pack.ParseSource(p.Repo)
	if err != nil {
		return nil, err
	}
	o := &ContributeOptions{Repo: p.Repo, Host: HostOther, GH: GHMissing, DefaultMessage: DefaultContributionMessage(e.Name),
		plan: &contribPlan{entry: *e, ls: ls, url: src.URL, clone: contribClone(p.Repo)}}
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
	sk := pack.Find(up, e.Name)
	if sk == nil {
		return nil, i18n.Errorf("%s is no longer in %s", e.Name, p.Repo)
	}
	rel, err := filepath.Rel(o.plan.clone, sk.Dir)
	if err != nil {
		return nil, err
	}
	o.plan.skillPath = filepath.ToSlash(rel)
	h, err := fsutil.HashTree(sk.Dir)
	if err != nil {
		return nil, err
	}
	var reasons []string
	if ls.Shelved != "" || ls.Resolving || h != ls.Hash {
		o.Behind = true
		reasons = append(reasons, i18n.Sprintf("%s is not based on upstream's current version: skd update %s first (and skd resolve, if it is shelved)", e.LinkName, p.Repo))
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
}

// Contribute sends the user's edits to a vendored skill upstream, in
// mode (ModePush or ModePR), with msg as the commit message (its first
// line titles a pull request; DefaultContributionMessage when empty).
// Only a person's request may call it. It records the contribution in
// skills.lock.
func (s *Session) Contribute(ctx context.Context, r Ref, mode, msg string) (*ContributeOutcome, error) {
	o, err := s.ContributeOptions(ctx, r)
	if err != nil {
		return nil, err
	}
	pl := o.plan
	switch {
	case mode != ModePR && mode != ModePush:
		return nil, i18n.Errorf("choose %s or %s, not %q", ModePR, ModePush, mode)
	case o.Behind:
		return nil, i18n.Errorf("%s is not based on upstream's current version: skd update %s first (and skd resolve, if it is shelved)", pl.entry.LinkName, o.Repo)
	case mode == ModePush && !o.CanPush:
		return nil, i18n.Errorf("you cannot push to %s; open a pull request instead: skd contribute %s --pr", o.Repo, pl.entry.LinkName)
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
	// The skill's files over its upstream directory, deletions too.
	ours, err := engine.ReadFileSet(pl.entry.Dir)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(pl.clone, filepath.FromSlash(pl.skillPath))
	if err := engine.WriteFileSet(target, ours, nil); err != nil {
		return nil, err
	}
	spec := ":(literal)" + pl.skillPath
	if pl.skillPath == "." {
		spec = "."
	}
	if _, err := library.GitContext(ctx, pl.clone, "add", "-A", "--", spec); err != nil {
		return nil, err
	}
	if _, err := library.GitContext(ctx, pl.clone, "diff", "--cached", "--quiet"); err == nil {
		return nil, i18n.Errorf("%s is the same as upstream's: nothing to contribute", pl.entry.LinkName)
	}
	if _, err := library.GitContext(ctx, pl.clone, "commit", "-q", "-m", msg); err != nil {
		return nil, err
	}
	out := &ContributeOutcome{Repo: o.Repo, Mode: mode}
	c := &lock.Contribution{Kind: mode, At: time.Now().UTC().Format(time.RFC3339)}
	if mode == ModePush {
		out.Branch = o.Branch
		if _, err := library.GitContext(ctx, pl.clone, "push", "-q", "origin", "HEAD:refs/heads/"+o.Branch); err != nil {
			return nil, i18n.Errorf("%s refused the push to %s (%v); open a pull request instead: skd contribute %s --pr", o.Repo, o.Branch, err, pl.entry.LinkName)
		}
		out.Pushed = true
		c.Branch, c.State = o.Branch, "pushed"
	} else {
		branch := "skilldots/" + pl.entry.Name + "-" + time.Now().Format("20060102-1504")
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
	if lp := s.Lib.Lock.Packs[pl.entry.Origin]; lp != nil && lp.Skills[pl.entry.Name] != nil {
		lp.Skills[pl.entry.Name].Contribution = c
	}
	return out, s.Lib.Save()
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

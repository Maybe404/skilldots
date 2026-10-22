package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/pack"
)

type source struct {
	Repo   string   `json:"repo"`
	URL    string   `json:"url"`
	Ref    string   `json:"ref,omitempty"`
	Skills []string `json:"skills"`
	Form   string   `json:"form"`
	Input  string   `json:"input,omitempty"`
}

func (s *Service) packOperation(ctx context.Context, ses *app.Session, method string, args []json.RawMessage) (any, error) {
	switch method {
	case "parseSource":
		text, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		src, h, err := ses.ParseAddText(text, "")
		if err != nil {
			return nil, err
		}
		form := "shorthand"
		switch h.From {
		case "command":
			form = "npx"
		case "skills.sh":
			form = "skills.sh"
		case "github link":
			form = "tree"
		default:
			if strings.HasPrefix(text, "http") {
				form = "github"
			} else if strings.Contains(text, ":") {
				form = "git"
			}
		}
		return source{Repo: src.Repo, URL: src.URL, Ref: src.Ref, Skills: h.Skills, Form: form, Input: text}, nil
	case "show":
		src, err := argument[source](args, 0)
		if err != nil {
			return nil, err
		}
		input := src.Input
		if input == "" {
			input = src.URL
			if src.Ref != "" {
				input += "@" + src.Ref
			}
		}
		parsed, h, err := ses.ParseAddText(input, "")
		if err != nil {
			return nil, err
		}
		f, err := ses.Fetch(parsed)
		if err != nil {
			return nil, err
		}
		s.previews[parsed.Repo] = f
		// Names the input asked for that the repository does not have are
		// reported with the preview, not as an error: the person then
		// picks from what is there. Preselect also matches category/name
		// and directory names, so the desktop takes both lists from here.
		selected, missing := f.Preselect(h)
		if selected == nil {
			selected = []string{}
		}
		if missing == nil {
			missing = []string{}
		}
		src.Repo = parsed.Repo
		src.Skills = selected
		skills := []object{}
		for _, sk := range f.Skills {
			skills = append(skills, object{"name": sk.Name, "description": sk.Description, "category": sk.Category, "license": sk.License})
		}
		rejected := []object{}
		for _, r := range f.Rejected {
			rejected = append(rejected, object{"name": r.Name, "dir": r.Dir, "reason": r.Reason})
		}
		have := []string{}
		for name := range f.Have {
			have = append(have, name)
		}
		slices.Sort(have)
		return object{"source": src, "commit": f.Commit, "skills": skills, "rejected": rejected, "subscribed": f.Subscribed, "have": have, "preselected": selected, "missing": missing}, nil
	case "add":
		preview, err := argument[struct {
			Source source
			Commit string
		}](args, 0)
		if err != nil {
			return nil, err
		}
		sel, err := argument[app.Selection](args, 1)
		if err != nil {
			return nil, err
		}
		f := s.previews[preview.Source.Repo]
		if f == nil || f.Commit != preview.Commit {
			return nil, errors.New("preview expired; inspect the repository again")
		}
		o, err := ses.Add(f, sel)
		if err != nil {
			return nil, err
		}
		if err = checkApply(o.Apply, nil); err != nil {
			return nil, err
		}
		renamed := map[string]string{}
		if o.Report != nil && o.Report.Renamed != nil {
			renamed = o.Report.Renamed
		}
		return object{"repo": o.Repo, "linked": o.Linked, "renamed": renamed, "nothing": o.Nothing, "absent": absentOf(o.Apply), "own": o.Report != nil && o.Report.Own}, nil
	case "updateDiff":
		repo, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		latest := ""
		if len(args) > 1 {
			if latest, err = argument[string](args, 1); err != nil {
				return nil, err
			}
		}
		return ses.UpdateDiff(ctx, repo, latest)
	case "update":
		repos, err := argument[[]string](args, 0)
		if err != nil {
			return nil, err
		}
		if len(repos) == 0 {
			return []object{}, nil
		}
		opts := struct{ AddNew map[string][]string }{}
		if len(args) > 1 {
			opts, err = argument[struct{ AddNew map[string][]string }](args, 1)
			if err != nil {
				return nil, err
			}
		}
		// Validate all selections before changing the manifest. A selected name
		// must still exist upstream; never silently install an unrelated skill.
		for repo, names := range opts.AddNew {
			if !slices.Contains(repos, repo) {
				return nil, errors.New("new skills must belong to a selected repository")
			}
			p := ses.Lib.Manifest.FindPack(repo)
			if p == nil {
				return nil, errors.New("repository no longer subscribed")
			}
			src, err := pack.ParseSource(repo)
			if err != nil {
				return nil, err
			}
			src.Ref = p.Ref
			f, err := ses.Fetch(src)
			if err != nil {
				return nil, err
			}
			for _, name := range names {
				for _, sk := range f.Skills {
					if sk.Name == name && p.SkipPattern(sk.Name, sk.Category) != "" {
						return nil, fmt.Errorf("skill %q is excluded by a skip rule; change the subscription first", name)
					}
				}
				if !slices.ContainsFunc(f.Skills, func(sk pack.Skill) bool { return sk.Name == name }) {
					return nil, fmt.Errorf("skill %q no longer exists upstream", name)
				}
			}
		}
		for repo, names := range opts.AddNew {
			p := ses.Lib.Manifest.FindPack(repo)
			if !p.UsesAll() {
				for _, name := range names {
					if !slices.Contains(p.Use, name) {
						p.Use = append(p.Use, name)
					}
				}
			}
		}
		o, err := ses.Update(repos, nil)
		if err != nil {
			return nil, err
		}
		if len(o.Failed) > 0 {
			return nil, fmt.Errorf("update incomplete: %v", o.Failed)
		}
		if err = checkApply(o.Apply, nil); err != nil {
			return nil, err
		}
		backups := []string{}
		if o.Apply != nil {
			backups = o.Apply.Backups
		}
		// The conflicted files of what this update shelved, from its reports.
		files := map[string][]string{}
		for _, rep := range o.Reports {
			for _, sh := range rep.Shelved {
				files[rep.Repo+"\x00"+sh.Name] = sh.Files
			}
		}
		return historyRows(o.History, backups, shelvedFiles(ses, files)), nil
	}
	return nil, errors.New("unknown pack operation")
}

func (s *Service) scanOperation(ses *app.Session, method string, args []json.RawMessage) (any, error) {
	rep, err := ses.Scan()
	if err != nil {
		return nil, err
	}
	props := ses.Proposals(rep)
	if method == "adopt" {
		// Each pick names the row and what the person saw would happen to
		// it. The proposals are worked out again (upstream may have moved
		// meanwhile): a row whose kind changed is refused, not adopted
		// some other way than the confirmation said.
		picks, err := argument[[]struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		}](args, 0)
		if err != nil {
			return nil, err
		}
		selected := []app.Proposal{}
		for _, pick := range picks {
			found := false
			for _, p := range props {
				if p.Item.Target+"/"+p.Item.Name == pick.ID && p.Actionable() {
					if p.Kind != pick.Kind {
						return nil, fmt.Errorf("scan item %q changed (now %s, shown as %s); scan again", pick.ID, p.Kind, pick.Kind)
					}
					selected = append(selected, p)
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("scan item %q changed; scan again", pick.ID)
			}
		}
		o, err := ses.Adopt(selected)
		if err != nil {
			return nil, err
		}
		if err = checkApply(o.Apply, nil); err != nil {
			return nil, err
		}
		return object{"adopted": len(selected)}, nil
	}
	// The rows are the unmanaged entries, the same count the status (and the
	// first-run screen) reports; links skd manages are not listed.
	entries, _ := ses.Resolve()
	out := []object{}
	for _, it := range rep.Unmanaged() {
		kind, label, repo, upstream, differs, sharedCopy := "skip", "", "", "", false, false
		// as: the name an "own" copy gets in skills/; inLibrary: the
		// library skill an "enable" installs (another name when the copy
		// was found by its content).
		as, inLibrary := "", ""
		for _, p := range props {
			if p.Item.Path == it.Path {
				kind, label, repo, upstream, differs, as = p.Kind, p.Label, p.Repo, p.Upstream, p.Differs, p.As
				if p.Entry != "" {
					inLibrary = p.Entry[strings.LastIndex(p.Entry, ":")+1:]
				}
				sharedCopy = p.Kind == "enable" && secondCopy(ses, entries, p)
				break
			}
		}
		out = append(out, object{"id": it.Target + "/" + it.Name, "agent": it.Target, "name": it.Name, "path": it.Path, "state": it.State, "linkTo": it.LinkTo, "source": it.Source, "proposal": kind, "label": label, "repo": repo, "upstream": upstream, "differs": differs, "sharedCopy": sharedCopy, "as": as, "inLibrary": inLibrary})
	}
	return out, nil
}

// secondCopy reports whether an "enable" proposal is for a copy the agent
// does not need: it already reads the library's entry from ~/.agents/skills
// (engine.ReachesViaShared), so adopting moves this copy to the trash
// rather than linking anything (app.Proposals).
func secondCopy(ses *app.Session, entries []engine.Entry, p app.Proposal) bool {
	t, err := ses.Reg.Get(p.Item.Target)
	if err != nil || p.Entry == "" {
		return false
	}
	for i := range entries {
		if engine.Qualified(entries[i]) == p.Entry {
			return engine.ReachesViaShared(ses.Lib, &entries[i], t)
		}
	}
	return false
}

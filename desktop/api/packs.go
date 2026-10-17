package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Maybe404/skilldots/internal/app"
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
		selected, missing := f.Preselect(h)
		if len(missing) > 0 {
			return nil, fmt.Errorf("skills not found: %s", strings.Join(missing, ", "))
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
		return object{"source": src, "commit": f.Commit, "skills": skills, "rejected": rejected, "subscribed": f.Subscribed, "have": have}, nil
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
		return object{"repo": o.Repo, "linked": o.Linked, "renamed": renamed, "nothing": o.Nothing}, nil
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
		return history(o.History), nil
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
		ids, err := argument[[]string](args, 0)
		if err != nil {
			return nil, err
		}
		selected := []app.Proposal{}
		for _, id := range ids {
			found := false
			for _, p := range props {
				if p.Item.Target+"/"+p.Item.Name == id && p.Actionable() {
					selected = append(selected, p)
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("scan item %q changed; scan again", id)
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
	out := []object{}
	for _, it := range rep.Items {
		kind, label, repo := "skip", "", ""
		for _, p := range props {
			if p.Item.Path == it.Path {
				kind, label, repo = p.Kind, p.Label, p.Repo
				break
			}
		}
		out = append(out, object{"id": it.Target + "/" + it.Name, "agent": it.Target, "name": it.Name, "path": it.Path, "state": it.State, "linkTo": it.LinkTo, "source": it.Source, "proposal": kind, "label": label, "repo": repo})
	}
	return out, nil
}

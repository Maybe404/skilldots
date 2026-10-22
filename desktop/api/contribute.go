package api

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Maybe404/skilldots/internal/app"
)

// contributeOperation is the user's edits and sending them upstream:
// localChanges(ref), contributeOptions(ref), contribute(ref, mode,
// message), contributePackOptions(repo), contributePack(repo, mode,
// message) (every skill of a pack the user changed or made, at once) and
// refreshContributions(). contribute sends only what the
// person confirmed in the window: the app shows contributeOptions (the
// repository, the branch, the mode) and the diff first.
func contributeOperation(ctx context.Context, ses *app.Session, method string, args []json.RawMessage) (any, error) {
	switch method {
	case "refreshContributions":
		return nil, ses.RefreshContributions(ctx)
	case "contributePackOptions":
		repo, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		return ses.ContributePackOptions(ctx, repo)
	case "contributePack":
		repo, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		mode, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		msg := ""
		if len(args) > 2 {
			if msg, err = argument[string](args, 2); err != nil {
				return nil, err
			}
		}
		return contributed(ses.ContributePack(ctx, repo, mode, msg))
	}
	rf, err := argument[ref](args, 0)
	if err != nil {
		return nil, err
	}
	switch method {
	case "localChanges":
		return ses.LocalChanges(ctx, rf.app())
	case "contributeOptions":
		return ses.ContributeOptions(ctx, rf.app())
	case "contribute":
		mode, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		msg := ""
		if len(args) > 2 {
			if msg, err = argument[string](args, 2); err != nil {
				return nil, err
			}
		}
		return contributed(ses.Contribute(ctx, rf.app(), mode, msg))
	}
	return nil, errors.New("unknown contribute operation")
}

// contributed is a contribution's outcome for the desktop.
func contributed(o *app.ContributeOutcome, err error) (any, error) {
	if o == nil || err != nil {
		return nil, err
	}
	return object{"mode": o.Mode, "url": o.URL, "number": o.Number, "branch": o.Branch, "pushed": o.Pushed, "skills": o.Skills,
		"pulled": o.Pulled != nil, "pulledBack": o.PulledBack != nil, "pullBackError": o.PullBackError}, nil
}

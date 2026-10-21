package api

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Maybe404/skilldots/internal/app"
)

// contributeOperation is the user's edits and sending them upstream:
// localChanges(ref), contributeOptions(ref), contribute(ref, mode,
// message) and refreshContributions(). contribute sends only what the
// person confirmed in the window: the app shows contributeOptions (the
// repository, the branch, the mode) and the diff first.
func contributeOperation(ctx context.Context, ses *app.Session, method string, args []json.RawMessage) (any, error) {
	if method == "refreshContributions" {
		return nil, ses.RefreshContributions(ctx)
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
		o, err := ses.Contribute(ctx, rf.app(), mode, msg)
		if o == nil {
			return nil, err
		}
		if err != nil {
			return nil, err
		}
		return object{"mode": o.Mode, "url": o.URL, "number": o.Number, "branch": o.Branch, "pushed": o.Pushed}, nil
	}
	return nil, errors.New("unknown contribute operation")
}

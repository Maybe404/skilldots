package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Maybe404/skilldots/internal/app"
)

// shelvedFiles maps "repo\x00name" of every skill whose update is
// shelved to its conflicted files, from the lock; known (an update's own
// report) wins.
func shelvedFiles(ses *app.Session, known map[string][]string) map[string][]string {
	out := map[string][]string{}
	if sh, err := ses.Shelved(); err == nil {
		for _, s := range sh {
			out[s.Repo+"\x00"+s.Name] = s.Files
		}
	}
	for k, v := range known {
		out[k] = v
	}
	return out
}

// historyDiff is historyDiff(repo, time): what the update of repo
// recorded at time (the record's "when", as history sends it) changed
// upstream, skill by skill.
func historyDiff(ctx context.Context, ses *app.Session, args []json.RawMessage) (any, error) {
	repo, err := argument[string](args, 0)
	if err != nil {
		return nil, err
	}
	when, err := argument[time.Time](args, 1)
	if err != nil {
		return nil, err
	}
	r, err := ses.FindRecord(repo, when)
	if err != nil {
		return nil, err
	}
	d, err := ses.HistoryDiff(ctx, r, "")
	if err != nil {
		return nil, err
	}
	return object{"skills": d.Skills, "source": d.Source}, nil
}

// resolveState is the conflicts left in a skill being resolved: files
// already resolved are not listed.
func resolveState(ses *app.Session, r app.Ref) (any, error) {
	files, err := ses.ConflictFiles(r)
	if err != nil {
		return nil, err
	}
	out := []app.ConflictFile{}
	for _, f := range files {
		if !f.Resolved {
			out = append(out, f)
		}
	}
	return object{"files": out}, nil
}

// resolveOperation is skd resolve for the desktop. Every method takes the
// skill's ref first; resolvePick then the file's path, the conflict's
// index in it (a file removed on one side, or not text, is taken whole:
// its index is ignored, 0 or -1) and "local", "upstream" or "both".
func resolveOperation(ses *app.Session, method string, args []json.RawMessage) (any, error) {
	rf, err := argument[ref](args, 0)
	if err != nil {
		return nil, err
	}
	if rf.Key == app.SyncRef {
		return syncResolveOperation(ses, method, args)
	}
	r := rf.app()
	switch method {
	case "resolveStart":
		o, err := ses.ResolveStart(r)
		if err != nil {
			return nil, err
		}
		if o.Merged {
			// The merge turned out clean: it is finished, nothing is left.
			return object{"files": []app.ConflictFile{}, "merged": true}, nil
		}
		return resolveState(ses, r)
	case "resolveState":
		return resolveState(ses, r)
	case "resolvePick":
		path, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		block, err := argument[int](args, 2)
		if err != nil {
			return nil, err
		}
		choice, err := argument[string](args, 3)
		if err != nil {
			return nil, err
		}
		return nil, ses.ResolvePick(r, path, block, choice)
	case "resolveContinue":
		o, err := ses.ResolveContinue(r)
		if o == nil {
			return nil, err
		}
		if err = checkApply(o.Apply, err); err != nil {
			return nil, err
		}
		return object{"absent": absentOf(o.Apply)}, nil
	case "resolveAbort":
		o, err := ses.ResolveAbort(r)
		if o == nil {
			return nil, err
		}
		return nil, checkApply(o.Apply, err)
	}
	return nil, errors.New("unknown resolve operation")
}

// syncResolveOperation is a sync's conflicts through the resolve calls,
// with the ref {key: "@sync"}: resolveStart is skd sync --resolve,
// resolveContinue --continue (and pushes), resolveAbort --abort. Paths
// are relative to the library's root.
func syncResolveOperation(ses *app.Session, method string, args []json.RawMessage) (any, error) {
	switch method {
	case "resolveStart":
		res, err := ses.SyncResolve()
		if err != nil {
			return nil, err
		}
		if res.Sync != nil {
			return object{"files": []app.ConflictFile{}, "merged": true}, nil
		}
		return syncState(ses)
	case "resolveState":
		return syncState(ses)
	case "resolvePick":
		path, err := argument[string](args, 1)
		if err != nil {
			return nil, err
		}
		block, err := argument[int](args, 2)
		if err != nil {
			return nil, err
		}
		choice, err := argument[string](args, 3)
		if err != nil {
			return nil, err
		}
		return nil, ses.SyncPick(path, block, choice)
	case "resolveContinue":
		o, err := ses.SyncContinue()
		if o == nil {
			return nil, err
		}
		if err = checkApply(o.Apply, err); err != nil {
			return nil, err
		}
		return object{"absent": absentOf(o.Apply)}, nil
	case "resolveAbort":
		return nil, ses.SyncAbort()
	}
	return nil, errors.New("unknown resolve operation")
}

func syncState(ses *app.Session) (any, error) {
	files, err := ses.SyncConflictFiles()
	if err != nil {
		return nil, err
	}
	out := []app.ConflictFile{}
	for _, f := range files {
		if !f.Resolved {
			out = append(out, f)
		}
	}
	return object{"files": out}, nil
}

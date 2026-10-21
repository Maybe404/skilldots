package api

import (
	"encoding/json"
	"errors"

	"github.com/Maybe404/skilldots/internal/app"
)

// trashOperation lists the trash with sizes and deletes from it. Deleting
// is permanent: the desktop asks the person first and sends back what it
// showed them (the number of entries, or the one entry), so nothing is
// deleted that the confirmation did not name.
func (s *Service) trashOperation(ses *app.Session, method string, args []json.RawMessage) (any, error) {
	switch method {
	case "trash":
		u, err := ses.TrashUsage()
		if err != nil {
			return nil, err
		}
		out := []object{}
		for _, e := range u.Entries {
			out = append(out, object{"id": e.ID, "name": e.Name, "origin": e.Origin, "path": e.Path, "when": e.When, "restorable": e.Restorable, "problem": e.Problem, "unsafe": e.Unsafe, "size": u.Sizes[e.ID], "skills": nonNilList(e.Skills), "reason": e.Reason, "repo": e.Repo})
		}
		return out, nil
	case "trashUsage":
		u, err := ses.TrashUsage()
		if err != nil {
			return nil, err
		}
		return object{"count": len(u.Entries), "bytes": u.Total}, nil
	case "emptyTrash":
		// The IDs the confirmation listed: the trash must hold exactly
		// those. A count alone would let one entry in and one out unseen.
		opts, err := argument[struct {
			IDs *[]string `json:"ids"`
		}](args, 0)
		if err != nil {
			return nil, err
		}
		if opts.IDs == nil {
			return nil, errors.New("emptying the trash needs the entries the person confirmed")
		}
		u, err := ses.TrashUsage()
		if err != nil {
			return nil, err
		}
		shown := map[string]bool{}
		for _, id := range *opts.IDs {
			shown[id] = true
		}
		same := len(shown) == len(u.Entries)
		for _, e := range u.Entries {
			same = same && shown[e.ID]
		}
		if !same {
			return nil, errors.New("the trash changed since it was shown; look at it again")
		}
		n, err := ses.EmptyTrash()
		return object{"deleted": n}, err
	case "deleteTrash":
		id, err := argument[string](args, 0)
		if err != nil {
			return nil, err
		}
		return nil, ses.DeleteTrashEntry(id)
	}
	return nil, errors.New("unknown trash operation")
}

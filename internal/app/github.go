package app

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/Maybe404/skilldots/internal/i18n"
)

// What gh (GitHub's command line) is like on this machine
// (GitHub.Status, ContributeOptions.GH).
const (
	GHOK        = "ok"
	GHMissing   = "missing"
	GHLoggedOut = "logged-out"
)

// GitHub is what skd asks GitHub, through gh, when it contributes a
// change upstream. Pushing is plain git; only these go through gh. Tests
// replace it (SetGitHub): nothing they run reaches GitHub.
type GitHub interface {
	// Status says whether gh is installed and logged in.
	Status(ctx context.Context) string
	// CanPush reports whether the logged-in user may push to owner/repo.
	CanPush(ctx context.Context, owner, repo string) (bool, error)
	// Fork forks owner/repo to the logged-in user (an existing fork is
	// reused) and returns the fork's owner and name.
	Fork(ctx context.Context, owner, repo string) (forkOwner, forkName string, err error)
	// CreatePR opens a pull request on owner/repo from head into base.
	CreatePR(ctx context.Context, owner, repo, base, head, title, body string) (url string, number int, err error)
	// PRState is a pull request's state: "open", "merged" or "closed".
	PRState(ctx context.Context, url string) (string, error)
}

var (
	githubMu sync.Mutex
	github   GitHub = ghCLI{}
)

// SetGitHub replaces what skd asks GitHub (tests) and returns the
// function that puts the previous one back.
func SetGitHub(g GitHub) (restore func()) {
	githubMu.Lock()
	defer githubMu.Unlock()
	prev := github
	github = g
	return func() {
		githubMu.Lock()
		defer githubMu.Unlock()
		github = prev
	}
}

func gh() GitHub {
	githubMu.Lock()
	defer githubMu.Unlock()
	return github
}

// ghCLI is GitHub through the gh command.
type ghCLI struct{}

func (ghCLI) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", i18n.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}

func (g ghCLI) Status(ctx context.Context) string {
	if _, err := exec.LookPath("gh"); err != nil {
		return GHMissing
	}
	if _, err := g.run(ctx, "auth", "status"); err != nil {
		return GHLoggedOut
	}
	return GHOK
}

func (g ghCLI) CanPush(ctx context.Context, owner, repo string) (bool, error) {
	out, err := g.run(ctx, "api", "repos/"+owner+"/"+repo, "--jq", ".permissions.push")
	if err != nil {
		return false, err
	}
	return out == "true", nil
}

func (g ghCLI) Fork(ctx context.Context, owner, repo string) (string, string, error) {
	if _, err := g.run(ctx, "repo", "fork", owner+"/"+repo, "--clone=false"); err != nil {
		return "", "", err
	}
	login, err := g.run(ctx, "api", "user", "--jq", ".login")
	if err != nil {
		return "", "", err
	}
	// The fork, new or made before: one made before may have been
	// renamed, so it is looked for among the repository's forks.
	if out, err := g.run(ctx, "api", "repos/"+owner+"/"+repo+"/forks", "--paginate", "--jq", ".[].full_name"); err == nil {
		for _, full := range strings.Fields(out) {
			if o, n, ok := strings.Cut(full, "/"); ok && strings.EqualFold(o, login) {
				return o, n, nil
			}
		}
	}
	return login, repo, nil
}

func (g ghCLI) CreatePR(ctx context.Context, owner, repo, base, head, title, body string) (string, int, error) {
	out, err := g.run(ctx, "pr", "create", "--repo", owner+"/"+repo, "--base", base, "--head", head, "--title", title, "--body", body)
	if err != nil {
		return "", 0, err
	}
	url := out
	if i := strings.LastIndex(out, "\n"); i >= 0 {
		url = strings.TrimSpace(out[i+1:])
	}
	return url, PRNumber(url), nil
}

func (g ghCLI) PRState(ctx context.Context, url string) (string, error) {
	out, err := g.run(ctx, "pr", "view", url, "--json", "state", "--jq", ".state")
	if err != nil {
		return "", err
	}
	return strings.ToLower(out), nil
}

// PRNumber is the number at the end of a pull request's URL
// (…/pull/12), 0 when there is none.
func PRNumber(url string) int {
	i := strings.LastIndex(url, "/pull/")
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimRight(url[i+len("/pull/"):], "/"))
	return n
}

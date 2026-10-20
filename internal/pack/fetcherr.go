package pack

import (
	"context"
	"errors"
	"strings"

	"github.com/Maybe404/skilldots/internal/i18n"
)

// gitFetchEnv is added to git's environment for clone and fetch. git never
// asks for a user name or password on the terminal (a dashboard, an agent
// or a pipe has none to answer, and GitHub answers a repository that does
// not exist with a request to log in), and it speaks English, so
// fetchError can tell what went wrong whatever the system's language.
// SSH and credential helpers work as before.
var gitFetchEnv = []string{"GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}

// Why a fetch failed (FetchError.Kind).
const (
	// FetchNotFound: no such repository, or one that needs a login skd
	// cannot give (a private repository over HTTPS).
	FetchNotFound = "not-found"
	// FetchNoAccess: the SSH key has no access, or the repository does
	// not exist (over SSH the two look the same).
	FetchNoAccess = "no-access"
	// FetchNoRef: the repository has no such branch or tag.
	FetchNoRef = "no-ref"
	// FetchOffline: the host could not be reached.
	FetchOffline = "offline"
)

// FetchError is a clone or fetch git refused, said for a person: what
// went wrong and what to do, with git's own words after it.
type FetchError struct {
	Repo string
	URL  string
	Ref  string
	Kind string
	// Git is the error git gave.
	Git error
}

func (e *FetchError) Error() string {
	return e.human() + "\n" + i18n.Sprintf("(git said: %s)", e.Git.Error())
}

func (e *FetchError) Unwrap() error { return e.Git }

func (e *FetchError) human() string {
	switch e.Kind {
	case FetchNotFound:
		if gh := githubRepo(e.URL); gh != "" {
			return i18n.Sprintf("cannot find the repository %s on GitHub, or it is private (for a private repository use its SSH address: git@github.com:%s.git)", gh, gh)
		}
		if !strings.Contains(e.URL, "://") && !strings.Contains(e.URL, "@") {
			return i18n.Sprintf("there is no git repository at %s", e.URL)
		}
		return i18n.Sprintf("cannot find the repository %s, or it is private (for a private repository use its SSH address)", e.URL)
	case FetchNoAccess:
		return i18n.Sprintf("cannot read the repository %s: it does not exist, or your SSH key has no access to it", e.URL)
	case FetchNoRef:
		return i18n.Sprintf("the repository %s has no branch or tag %q", e.Repo, e.Ref)
	case FetchOffline:
		return i18n.Sprintf("cannot reach %s: check the network connection", e.URL)
	}
	return ""
}

// githubRepo is "owner/repo" for an HTTPS GitHub URL, else "".
func githubRepo(url string) string {
	for _, pre := range []string{"https://github.com/", "http://github.com/"} {
		if strings.HasPrefix(url, pre) {
			return strings.TrimSuffix(strings.TrimPrefix(url, pre), ".git")
		}
	}
	return ""
}

// fetchError says for a person why git could not clone or fetch src,
// when that is one of the reasons people meet; any other error, and one
// of a cancelled fetch, is returned as it is.
func fetchError(src Source, err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	kind := fetchKind(err.Error(), src.Ref != "")
	if kind == "" {
		return err
	}
	return &FetchError{Repo: src.Repo, URL: src.URL, Ref: src.Ref, Kind: kind, Git: err}
}

// fetchKind classifies git's (English) message msg.
func fetchKind(msg string, ref bool) string {
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(msg, s) {
				return true
			}
		}
		return false
	}
	switch {
	case ref && has("Remote branch", "couldn't find remote ref", "not found in upstream"):
		return FetchNoRef
	case has("Permission denied (publickey", "Host key verification failed"):
		return FetchNoAccess
	case has("could not read Username", "could not read Password", "terminal prompts disabled",
		"Repository not found", "Authentication failed", "returned error: 401", "returned error: 403",
		"returned error: 404", "does not appear to be a git repository", "does not exist"):
		return FetchNotFound
	case has("Could not resolve host", "Failed to connect", "Connection timed out", "Connection refused",
		"Network is unreachable", "SSL_connect", "SSL_ERROR", "Operation timed out", "Could not resolve hostname"):
		return FetchOffline
	}
	return ""
}

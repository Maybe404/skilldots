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
	// FetchHostKey: ssh could not verify the host's key (known_hosts).
	FetchHostKey = "host-key"
	// FetchAuth: the host refused the credentials git gave (an expired
	// token, a changed password).
	FetchAuth = "auth"
	// FetchForbidden: the host answered 403: no access with these
	// credentials, or it is limiting requests.
	FetchForbidden = "forbidden"
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
	case FetchHostKey:
		return i18n.Sprintf("ssh cannot verify the host key for %s: the host is not in ~/.ssh/known_hosts, or its key changed; check it, add it (connect once with ssh) and try again", e.URL)
	case FetchAuth:
		return i18n.Sprintf("%s refused the credentials git gave (an expired or revoked token, a changed password): update them in your git credential helper and try again", e.URL)
	case FetchForbidden:
		return i18n.Sprintf("%s refused access (HTTP 403): the credentials git gave have no access to it, or the host is limiting requests for now; check them, or try again later", e.URL)
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
	kind := fetchKind(err.Error(), src.Ref != "", sshURL(src.URL))
	if kind == "" {
		return err
	}
	return &FetchError{Repo: src.Repo, URL: src.URL, Ref: src.Ref, Kind: kind, Git: err}
}

// sshURL reports whether git reaches url over SSH (user@host:path or
// ssh://): a missing repository then looks like one the key cannot read,
// and the SSH address is no way out of it.
func sshURL(url string) bool {
	return strings.HasPrefix(url, "ssh://") || (strings.Contains(url, "@") && !strings.Contains(url, "://"))
}

// fetchKind classifies git's (English) message msg, for a fetch of a ref
// (ref) over SSH (ssh) or not.
func fetchKind(msg string, ref, ssh bool) string {
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(msg, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("Host key verification failed", "REMOTE HOST IDENTIFICATION HAS CHANGED"):
		return FetchHostKey
	// Before anything about the repository: over SSH a connection that
	// failed also ends in "Could not read from remote repository".
	case has("Could not resolve host", "Could not resolve hostname", "Failed to connect", "Connection timed out",
		"Connection refused", "Network is unreachable", "No route to host", "SSL_connect", "SSL_ERROR",
		"Operation timed out", "Connection closed by", "Connection reset", "kex_exchange_identification",
		"Broken pipe", "nodename nor servname", "Name or service not known", "Temporary failure in name resolution"):
		return FetchOffline
	case ref && has("Remote branch", "couldn't find remote ref", "not found in upstream"):
		return FetchNoRef
	case has("Permission denied (publickey"):
		return FetchNoAccess
	case has("Authentication failed", "returned error: 401"):
		return FetchAuth
	case has("returned error: 403", "rate limit"):
		return FetchForbidden
	case ssh && has("Repository not found", "Could not read from remote repository", "does not appear to be a git repository"):
		return FetchNoAccess
	case has("could not read Username", "could not read Password", "terminal prompts disabled",
		"Repository not found", "returned error: 404", "does not appear to be a git repository", "does not exist"):
		return FetchNotFound
	}
	return ""
}

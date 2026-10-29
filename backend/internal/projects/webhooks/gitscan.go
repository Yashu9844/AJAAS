package webhooks

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Bounds on what one provider call can create (resource exhaustion).
const (
	maxCommitsScanned = 20
	maxCandidates     = 50
	maxScanText       = 2000
	maxBranchLength   = 200
)

var commitID = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// gitPayload is the part of a GitHub or Gitea webhook body the integration reads. Titles and messages are scanned for
// ticket keys and then dropped: they are never stored.
type gitPayload struct {
	Action      string `json:"action"`
	Ref         string `json:"ref"`
	PullRequest *struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		Title   string `json:"title"`
		Merged  bool   `json:"merged"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
	} `json:"pull_request"`
	Commits []struct {
		ID      string `json:"id"`
		URL     string `json:"url"`
		Message string `json:"message"`
	} `json:"commits"`
	Repository struct {
		HTMLURL string `json:"html_url"`
	} `json:"repository"`
}

// candidate is one link a provider event asks for; number is the task number inside the integration's project.
type candidate struct {
	number     int
	kind       string
	externalID string
	url        string
	state      string
}

// scanKeys returns the task numbers of every KEY-<n> in the texts. The key must stand alone (not part of a longer word or
// number) and is matched case-insensitively, so branch names like proj-12-fix work.
func scanKeys(key string, texts ...string) []int {
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(key) + `-(\d{1,9})`)
	seen := map[int]bool{}
	var out []int
	for _, t := range texts {
		if len(t) > maxScanText {
			t = t[:maxScanText]
		}
		for _, m := range re.FindAllStringSubmatchIndex(t, -1) {
			if (m[0] > 0 && isWordByte(t[m[0]-1])) || (m[1] < len(t) && isWordByte(t[m[1]])) {
				continue
			}
			n, err := strconv.Atoi(t[m[2]:m[3]])
			if err == nil && n > 0 && !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// extract turns a provider event into the links it asks for (pull_request opened/reopened/closed, push).
func extract(provider, event, key string, p gitPayload) []candidate {
	var out []candidate
	switch event {
	case "pull_request":
		out = pullRequestLinks(key, p)
	case "push":
		out = pushLinks(provider, key, p)
	}
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out
}

func pullRequestLinks(key string, p gitPayload) []candidate {
	pr := p.PullRequest
	if pr == nil || pr.Number <= 0 {
		return nil
	}
	state := StateOpen
	switch p.Action {
	case "opened", "reopened":
	case "closed":
		state = StateClosed
		if pr.Merged {
			state = StateMerged
		}
	default:
		return nil
	}
	link, err := ValidateStoredURL(pr.HTMLURL)
	if err != nil {
		return nil
	}
	var out []candidate
	for _, n := range scanKeys(key, pr.Title, pr.Head.Ref) {
		out = append(out, candidate{number: n, kind: KindPR, externalID: strconv.Itoa(pr.Number), url: link, state: state})
	}
	return out
}

func pushLinks(provider, key string, p gitPayload) []candidate {
	var out []candidate
	if branch, ok := strings.CutPrefix(p.Ref, "refs/heads/"); ok && len(branch) <= maxBranchLength {
		if link, ok := branchURL(provider, p.Repository.HTMLURL, branch); ok {
			for _, n := range scanKeys(key, branch) {
				out = append(out, candidate{number: n, kind: KindBranch, externalID: branch, url: link, state: StateOpen})
			}
		}
	}
	for i, c := range p.Commits {
		if i >= maxCommitsScanned {
			break
		}
		link, err := ValidateStoredURL(c.URL)
		if err != nil || !commitID.MatchString(c.ID) {
			continue
		}
		for _, n := range scanKeys(key, c.Message) {
			out = append(out, candidate{number: n, kind: KindCommit, externalID: c.ID, url: link, state: StateOpen})
		}
	}
	return out
}

// branchURL builds the branch page link from the repository page: /tree/<branch> on GitHub, /src/branch/<branch> on Gitea.
func branchURL(provider, repo, branch string) (string, bool) {
	base, err := ValidateStoredURL(repo)
	if err != nil {
		return "", false
	}
	segs := strings.Split(branch, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	prefix := "/tree/"
	if provider == ProviderGitea {
		prefix = "/src/branch/"
	}
	link, err := ValidateStoredURL(strings.TrimRight(base, "/") + prefix + strings.Join(segs, "/"))
	return link, err == nil
}

package webhooks

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestScanKeys(t *testing.T) {
	cases := []struct {
		texts []string
		want  []int
	}{
		{[]string{"PRJ-12 fix"}, []int{12}},
		{[]string{"prj-12-fix", "feature/PRJ-3"}, []int{12, 3}},
		{[]string{"PRJ-1, PRJ-2 and PRJ-1 again"}, []int{1, 2}},
		{[]string{"PRJ-1 PRJ-2"}, []int{1, 2}},
		{[]string{"(PRJ-7)", "[PRJ-8]", "PRJ-9."}, []int{7, 8, 9}},
		{[]string{"XPRJ-5", "PRJ-5x", "PRJ-123456789012", "my_PRJ-4", "OTHER-1", "PRJ-0", "PRJ-"}, nil},
		{[]string{"PRJ-5" + strings.Repeat(" ", maxScanText) + "PRJ-6"}, []int{5}},
		{nil, nil},
	}
	for _, c := range cases {
		if got := scanKeys("PRJ", c.texts...); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q -> %v want %v", c.texts, got, c.want)
		}
	}
}

func payload(t *testing.T, s string) gitPayload {
	t.Helper()
	var p gitPayload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractPullRequestStates(t *testing.T) {
	pr := func(action, merged string) gitPayload {
		return payload(t, `{"action":"`+action+`","pull_request":{"number":9,"html_url":"https://github.com/o/r/pull/9","title":"PRJ-1 add","merged":`+merged+`,"head":{"ref":"feature/prj-2"}}}`)
	}
	for _, c := range []struct{ action, merged, state string }{{"opened", "false", StateOpen}, {"reopened", "false", StateOpen}, {"closed", "true", StateMerged}, {"closed", "false", StateClosed}} {
		got := extract(ProviderGitHub, "pull_request", "PRJ", pr(c.action, c.merged))
		if len(got) != 2 || got[0].state != c.state || got[0].kind != KindPR || got[0].externalID != "9" || got[0].number != 1 || got[1].number != 2 {
			t.Errorf("%s/%s: %+v", c.action, c.merged, got)
		}
	}
	for _, action := range []string{"edited", "synchronize", "labeled", ""} {
		if got := extract(ProviderGitHub, "pull_request", "PRJ", pr(action, "false")); got != nil {
			t.Errorf("action %q is ignored: %+v", action, got)
		}
	}
	for _, raw := range []string{`{"action":"opened"}`, `{"action":"opened","pull_request":{"number":0,"html_url":"https://x.y/1","title":"PRJ-1"}}`,
		`{"action":"opened","pull_request":{"number":3,"html_url":"http://x.y/1","title":"PRJ-1"}}`, `{"action":"opened","pull_request":{"number":3,"html_url":"https://u:p@x.y/1","title":"PRJ-1"}}`} {
		if got := extract(ProviderGitHub, "pull_request", "PRJ", payload(t, raw)); got != nil {
			t.Errorf("%s must yield nothing: %+v", raw, got)
		}
	}
	if extract(ProviderGitHub, "issues", "PRJ", pr("opened", "false")) != nil {
		t.Error("other events are ignored")
	}
}

func TestExtractPush(t *testing.T) {
	p := payload(t, `{"ref":"refs/heads/feature/PRJ-4-login","repository":{"html_url":"https://git.example.com/o/r"},
		"commits":[{"id":"abcdef1234567","url":"https://git.example.com/o/r/commit/abcdef1234567","message":"PRJ-4 wire it"},
		{"id":"NOTHEX","url":"https://git.example.com/o/r/commit/x","message":"PRJ-4"},
		{"id":"1234567abcdef","url":"http://insecure/x","message":"PRJ-4"},
		{"id":"1234567abcdef","url":"https://git.example.com/o/r/commit/1234567abcdef","message":"no key here"}]}`)
	gh := extract(ProviderGitHub, "push", "PRJ", p)
	if len(gh) != 2 || gh[0].kind != KindBranch || gh[0].externalID != "feature/PRJ-4-login" || gh[0].url != "https://git.example.com/o/r/tree/feature/PRJ-4-login" ||
		gh[1].kind != KindCommit || gh[1].externalID != "abcdef1234567" || gh[1].state != StateOpen {
		t.Fatalf("github push: %+v", gh)
	}
	if ge := extract(ProviderGitea, "push", "PRJ", p); ge[0].url != "https://git.example.com/o/r/src/branch/feature/PRJ-4-login" {
		t.Fatalf("gitea branch url: %+v", ge[0])
	}
	if got := extract(ProviderGitHub, "push", "PRJ", payload(t, `{"ref":"refs/tags/PRJ-1","repository":{"html_url":"https://x.y/r"}}`)); got != nil {
		t.Errorf("tags are not branches: %+v", got)
	}
	if got := extract(ProviderGitHub, "push", "PRJ", payload(t, `{"ref":"refs/heads/PRJ-1","repository":{"html_url":"http://x.y/r"}}`)); got != nil {
		t.Errorf("an insecure repository url yields no branch link: %+v", got)
	}
	long := payload(t, `{"ref":"refs/heads/PRJ-1-`+strings.Repeat("a", maxBranchLength)+`","repository":{"html_url":"https://x.y/r"}}`)
	if extract(ProviderGitHub, "push", "PRJ", long) != nil {
		t.Error("over-long branch names are ignored")
	}
}

func TestExtractIsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"ref":"refs/heads/x","repository":{"html_url":"https://x.y/r"},"commits":[`)
	for i := 0; i < 60; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"id":"abcdef` + strings.Repeat("0", 6) + `","url":"https://x.y/c","message":"PRJ-` + string(rune('1'+i%9)) + ` PRJ-` + string(rune('1'+(i+1)%9)) + `"}`)
	}
	b.WriteString("]}")
	if got := extract(ProviderGitHub, "push", "PRJ", payload(t, b.String())); len(got) > maxCandidates || len(got) == 0 {
		t.Fatalf("candidates are capped at %d, got %d", maxCandidates, len(got))
	}
}

func TestSignatureMatches(t *testing.T) {
	good := hmacHex([]byte("s3cret"), []byte("body"))
	cases := []struct {
		provider, header string
		want             bool
	}{
		{ProviderGitHub, "sha256=" + good, true},
		{ProviderGitHub, " sha256=" + good + " ", true},
		{ProviderGitHub, good, false},
		{ProviderGitHub, "sha1=" + good, false},
		{ProviderGitHub, "sha256=", false},
		{ProviderGitHub, "", false},
		{ProviderGitHub, "sha256=zz", false},
		{ProviderGitea, good, true},
		{ProviderGitea, "sha256=" + good, false},
		{ProviderGitea, good[:len(good)-2], false},
		{ProviderGitea, "", false},
		{ProviderGitea, hmacHex([]byte("other"), []byte("body")), false},
	}
	for _, c := range cases {
		if got := signatureMatches(c.provider, c.header, good); got != c.want {
			t.Errorf("%s %q -> %v", c.provider, c.header, got)
		}
	}
}

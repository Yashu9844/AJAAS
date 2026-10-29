package webhooks

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
)

// SSRF guard (SECURITY section 9 exception): every URL form an attacker might use to reach an internal service.
func TestValidateURLRefusals(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want error
	}{
		{"plain http", "http://example.com/hook", ErrURLScheme},
		{"ftp", "ftp://example.com/x", ErrURLScheme},
		{"javascript", "javascript:alert(1)", ErrURLInvalid},
		{"empty", "", ErrURLInvalid},
		{"no host", "https://", ErrURLInvalid},
		{"space in host", "https://exa mple.com/", ErrURLInvalid},
		{"backslash trick", `https://example.com\@evil.test/`, ErrURLInvalid},
		{"userinfo", "https://user:pw@example.com/hook", ErrURLUserinfo},
		{"userinfo only name", "https://user@example.com/", ErrURLUserinfo},
		{"loopback v4", "https://127.0.0.1/", ErrURLHost},
		{"loopback 127.1", "https://127.1/", ErrURLHost},
		{"hex loopback", "https://0x7f.1/", ErrURLHost},
		{"hex whole", "https://0x7f000001/", ErrURLHost},
		{"decimal int", "https://2130706433/", ErrURLHost},
		{"octal int", "https://017700000001/", ErrURLHost},
		{"mapped v6 loopback", "https://[::ffff:127.0.0.1]/", ErrURLHost},
		{"mapped v6 private", "https://[::ffff:10.0.0.1]/", ErrURLHost},
		{"v6 loopback", "https://[::1]/", ErrURLHost},
		{"v6 unique local", "https://[fc00::1]/", ErrURLHost},
		{"v6 link local", "https://[fe80::1]/", ErrURLHost},
		{"v6 unspecified", "https://[::]/", ErrURLHost},
		{"localhost", "https://localhost/", ErrURLHost},
		{"localhost dot", "https://localhost./", ErrURLHost},
		{"sub localhost", "https://app.localhost/", ErrURLHost},
		{"dot local", "https://printer.local/", ErrURLHost},
		{"metadata ip", "https://169.254.169.254/latest/meta-data/", ErrURLHost},
		{"metadata host", "https://metadata.google.internal/computeMetadata/v1/", ErrURLHost},
		{"single label", "https://intranet/", ErrURLHost},
		{"private 10", "https://10.1.2.3/", ErrURLHost},
		{"private 172", "https://172.16.0.1/", ErrURLHost},
		{"private 192", "https://192.168.0.10/", ErrURLHost},
		{"cgnat", "https://100.64.0.1/", ErrURLHost},
		{"unspecified", "https://0.0.0.0/", ErrURLHost},
		{"multicast", "https://224.0.0.1/", ErrURLHost},
		{"port 80", "https://example.com:80/", ErrURLPort},
		{"port 22", "https://example.com:22/", ErrURLPort},
		{"port 8080", "https://example.com:8080/", ErrURLPort},
		{"too long", "https://example.com/" + strings.Repeat("a", MaxURLLength), ErrURLInvalid},
	}
	for _, c := range cases {
		if _, err := ValidateURL(c.url); !errors.Is(err, c.want) {
			t.Errorf("%s: %q -> %v, want %v", c.name, c.url, err, c.want)
		}
	}
}

func TestValidateURLAccepts(t *testing.T) {
	for _, raw := range []string{"https://hooks.slack.com/services/T0/B0/xxx", "https://example.com:8443/hook", "https://example.com:443/", "https://203.0.114.5/x", "https://[2606:4700::1111]/x"} {
		if _, err := ValidateURL(raw); err != nil {
			t.Errorf("%q must be accepted: %v", raw, err)
		}
	}
}

func TestBlockedAddr(t *testing.T) {
	blocked := []string{"127.0.0.1", "127.255.255.255", "10.0.0.1", "172.16.0.1", "172.31.255.255", "192.168.1.1", "100.64.0.1", "100.127.255.255",
		"169.254.169.254", "169.254.0.1", "0.0.0.0", "0.1.2.3", "224.0.0.1", "239.255.255.255", "240.0.0.1", "255.255.255.255", "192.0.2.1", "198.18.0.1",
		"::", "::1", "fc00::1", "fd12:3456::1", "fe80::1", "ff02::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:169.254.169.254",
		"64:ff9b::7f00:1", "2002:7f00:1::1", "2001:db8::1", "fe80::1%eth0"}
	for _, s := range blocked {
		if !BlockedAddr(netip.MustParseAddr(s)) {
			t.Errorf("%s must be blocked", s)
		}
	}
	allowed := []string{"8.8.8.8", "93.184.216.34", "172.15.255.255", "172.32.0.1", "100.63.255.255", "100.128.0.1", "2606:4700:4700::1111", "::ffff:8.8.8.8"}
	for _, s := range allowed {
		if BlockedAddr(netip.MustParseAddr(s)) {
			t.Errorf("%s must be allowed", s)
		}
	}
	if !BlockedAddr(netip.Addr{}) {
		t.Error("an invalid address is blocked")
	}
}

func TestValidateStoredURL(t *testing.T) {
	if u, err := ValidateStoredURL("https://github.com/o/r/pull/3"); err != nil || u == "" {
		t.Fatalf("https links are accepted: %v", err)
	}
	for _, bad := range []string{"", "http://github.com/x", "https://u:p@github.com/x", "https:///x", "https://git hub.com/", `https://a.com\x`, "https://a.com/" + strings.Repeat("x", MaxURLLength), "://"} {
		if _, err := ValidateStoredURL(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestNumericLooking(t *testing.T) {
	for h, want := range map[string]bool{"127.0.0.1": true, "0x7f.1": true, "2130706433": true, "example.com": false, "1.example.com": false, "a..b": false, "1..2": true, "": true} {
		if numericLooking(h) != want {
			t.Errorf("numericLooking(%q) != %v", h, want)
		}
	}
	if !hostRefused("", urlRules{}) {
		t.Error("an empty host is refused")
	}
	if hostRefused("127.0.0.1", urlRules{allowLoopback: true}) || hostRefused("localhost", urlRules{allowLoopback: true}) {
		t.Error("the test-only loopback rule relaxes literal and named loopback hosts")
	}
}

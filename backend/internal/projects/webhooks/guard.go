// Package webhooks is the outgoing-webhook and git-integration context of M2 (specification section 9, phase 4).
//
// It holds the ONE controlled exception to "the backend never fetches user-controlled URLs" (SECURITY section 9): the
// webhook sender. Every outgoing request goes through Sender.Do, which enforces the guard in guard.go and client.go.
package webhooks

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// MaxURLLength bounds a stored URL.
const MaxURLLength = 500

// Errors a URL or address can be refused with. They carry no URL, host or address so they are safe to store and return.
var (
	ErrURLInvalid     = errors.New("url is not valid")
	ErrURLScheme      = errors.New("url must use https")
	ErrURLUserinfo    = errors.New("url must not contain credentials")
	ErrURLPort        = errors.New("url port must be 443 or 8443")
	ErrURLHost        = errors.New("url host is not allowed")
	ErrAddressBlocked = errors.New("address is not allowed")
)

// blockedPrefixes are the address ranges a webhook must never reach (IPv4 and IPv6 equivalents; IPv4-mapped forms are
// unmapped before the check).
var blockedPrefixes = parsePrefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24",
	"192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8", "64:ff9b::/96", "2002::/16", "100::/64", "2001:db8::/32",
)

func parsePrefixes(list ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

// BlockedAddr reports whether a connection to ip must be refused: loopback, private, link-local (including the cloud
// metadata address 169.254.169.254), carrier-grade NAT, multicast, unspecified, reserved and their IPv6 equivalents.
func BlockedAddr(ip netip.Addr) bool {
	ip = ip.Unmap().WithZone("")
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// urlRules relax the guard for tests only; the production value is the zero value.
type urlRules struct {
	allowHTTP     bool // plain http (test servers)
	allowLoopback bool // literal loopback hosts and any port (test servers)
}

// ValidateURL checks a webhook URL at create/update time: https only, no credentials, ports 443 and 8443, and a host
// that is not an internal or numeric-looking name or a blocked address literal. The send path validates again.
func ValidateURL(raw string) (*url.URL, error) { return validateURL(raw, urlRules{}) }

func validateURL(raw string, rules urlRules) (*url.URL, error) {
	if raw == "" || len(raw) > MaxURLLength || strings.ContainsAny(raw, " \t\r\n\\") {
		return nil, ErrURLInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return nil, ErrURLInvalid
	}
	if u.Scheme != "https" && !(rules.allowHTTP && u.Scheme == "http") {
		return nil, ErrURLScheme
	}
	if u.User != nil || strings.Contains(u.Host, "@") {
		return nil, ErrURLUserinfo
	}
	if !rules.allowLoopback && !portAllowed(u.Port()) {
		return nil, ErrURLPort
	}
	if hostRefused(u.Hostname(), rules) {
		return nil, ErrURLHost
	}
	return u, nil
}

func portAllowed(p string) bool { return p == "" || p == "443" || p == "8443" }

// hostRefused rejects internal names, address literals that are blocked, and numeric-looking hosts (0x7f.1, 2130706433,
// 127.1) that some resolvers read as IPv4 addresses.
func hostRefused(host string, rules urlRules) bool {
	h := strings.ToLower(strings.TrimRight(host, "."))
	if h == "" {
		return true
	}
	if ip, err := netip.ParseAddr(h); err == nil {
		return BlockedAddr(ip) && !rules.allowLoopback
	}
	if rules.allowLoopback {
		return false
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") ||
		strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".localdomain") || !strings.Contains(h, ".") {
		return true
	}
	return numericLooking(h)
}

// numericLooking is true when every dot-separated label is a decimal or 0x-prefixed number.
func numericLooking(h string) bool {
	for _, label := range strings.Split(h, ".") {
		if label == "" {
			return true
		}
		if _, err := strconv.ParseUint(label, 0, 64); err != nil {
			return false
		}
	}
	return true
}

// ValidateStoredURL checks a URL that is only stored and shown (git links): https, no credentials, bounded length. It is
// never fetched by the backend, so address checks are not needed.
func ValidateStoredURL(raw string) (string, error) {
	if raw == "" || len(raw) > MaxURLLength || strings.ContainsAny(raw, " \t\r\n\\") {
		return "", ErrURLInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return "", ErrURLInvalid
	}
	if u.Scheme != "https" {
		return "", ErrURLScheme
	}
	if u.User != nil {
		return "", ErrURLUserinfo
	}
	return u.String(), nil
}

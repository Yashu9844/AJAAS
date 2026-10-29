package webhooks

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"syscall"
	"time"
)

// Limits of one outgoing request (SECURITY section 9 exception).
const (
	SendTimeout      = 5 * time.Second
	MaxRequestBody   = 64 << 10
	MaxResponseRead  = 4 << 10
	userAgentWebhook = "JAAS-Webhook/1"
)

// Short, fixed failure descriptions; raw errors (which carry URLs and addresses) are never stored.
const (
	errBlocked  = "destination address is not allowed"
	errRedirect = "redirect refused"
	errTimeout  = "timeout"
	errTLS      = "tls error"
	errConnect  = "connection failed"
	errBadURL   = "url is not allowed"
	errTooLarge = "request body too large"
	errSecret   = "secret unavailable"
	errNoEvent  = "event unavailable"
)

var errRedirectRefused = errors.New("redirect refused")

// Request is one outgoing delivery.
type Request struct {
	URL     string
	Body    []byte
	Headers map[string]string
}

// Result is the outcome of one send: Status is 0 when no response was received, Err is a short fixed description.
type Result struct {
	Status int
	Err    string
}

// OK reports a 2xx response.
func (r Result) OK() bool { return r.Err == "" && r.Status >= 200 && r.Status < 300 }

// Doer sends one request; the Service uses a Sender and tests substitute a fake.
type Doer interface {
	Do(ctx context.Context, r Request) Result
}

// lookupFunc resolves a host to addresses (net.Resolver in production, a fake in rebinding tests).
type lookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)

// senderOptions are the test-only relaxations; NewSender uses the zero value and nothing reads them from configuration.
type senderOptions struct {
	rules   urlRules
	lookup  lookupFunc
	tls     *tls.Config
	timeout time.Duration                                                     // shorter than SendTimeout
	connect func(ctx context.Context, network, addr string) (net.Conn, error) // replaces the socket dial after the address checks
}

// Sender sends requests through the SSRF guard: https only, ports 443/8443, no credentials, no redirects, validated
// connected IPs, a 5 s total timeout, bounded request and response sizes, and no proxy.
type Sender struct {
	opts   senderOptions
	client *http.Client
}

// NewSender returns the production sender.
func NewSender() *Sender { return newSender(senderOptions{}) }

func newSender(o senderOptions) *Sender {
	if o.lookup == nil {
		o.lookup = systemLookup
	}
	if o.timeout <= 0 {
		o.timeout = SendTimeout
	}
	s := &Sender{opts: o}
	tr := &http.Transport{
		Proxy:                  nil, // proxy environment variables are ignored
		DialContext:            s.dial,
		TLSClientConfig:        o.tls,
		DisableKeepAlives:      true,
		ForceAttemptHTTP2:      false,
		TLSHandshakeTimeout:    SendTimeout,
		ResponseHeaderTimeout:  SendTimeout,
		MaxResponseHeaderBytes: 16 << 10,
	}
	s.client = &http.Client{Transport: tr, Timeout: o.timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return errRedirectRefused }}
	return s
}

func systemLookup(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// dial resolves the host, refuses the whole host when any answer is blocked, then connects to a validated address
// literal. The socket control re-checks the address actually connected, so a resolver that answers differently the next
// time (DNS rebinding) cannot change where the connection goes.
func (s *Sender) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, ErrURLInvalid
	}
	ips, err := s.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: s.opts.timeout, Control: s.control}
	dialFn := d.DialContext
	if s.opts.connect != nil {
		dialFn = s.opts.connect
	}
	var last error = errors.New("no address")
	for _, ip := range ips {
		conn, derr := dialFn(ctx, network, net.JoinHostPort(ip.String(), port))
		if derr == nil {
			return conn, nil
		}
		last = derr
	}
	return nil, last
}

func (s *Sender) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return s.checked([]netip.Addr{ip})
	}
	ips, err := s.opts.lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	return s.checked(ips)
}

func (s *Sender) checked(ips []netip.Addr) ([]netip.Addr, error) {
	if len(ips) == 0 {
		return nil, ErrAddressBlocked
	}
	for _, ip := range ips {
		if BlockedAddr(ip) && !s.opts.rules.allowLoopback {
			return nil, ErrAddressBlocked
		}
	}
	return ips, nil
}

// control runs on the socket before connect with the real destination.
func (s *Sender) control(_, address string, _ syscall.RawConn) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return ErrAddressBlocked
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || (BlockedAddr(ip) && !s.opts.rules.allowLoopback) {
		return ErrAddressBlocked
	}
	if !s.opts.rules.allowLoopback && !portAllowed(port) {
		return ErrURLPort
	}
	return nil
}

// Do validates the URL again, sends the request and classifies the outcome. The response body is read up to 4 KB and
// discarded; nothing from the response is kept.
func (s *Sender) Do(ctx context.Context, r Request) Result {
	if len(r.Body) > MaxRequestBody {
		return Result{Err: errTooLarge}
	}
	u, err := validateURL(r.URL, s.opts.rules)
	if err != nil {
		return Result{Err: errBadURL}
	}
	ctx, cancel := context.WithTimeout(ctx, s.opts.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(r.Body))
	if err != nil {
		return Result{Err: errBadURL}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgentWebhook)
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return Result{Err: classify(err)}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, MaxResponseRead))
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return Result{Status: resp.StatusCode, Err: errRedirect}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{Status: resp.StatusCode, Err: "http " + strconv.Itoa(resp.StatusCode)}
	}
	return Result{Status: resp.StatusCode}
}

func classify(err error) string {
	var tlsErr *tls.CertificateVerificationError
	var netErr net.Error
	switch {
	case errors.Is(err, errRedirectRefused):
		return errRedirect
	case errors.Is(err, ErrAddressBlocked), errors.Is(err, ErrURLPort):
		return errBlocked
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return errTimeout
	case errors.As(err, &tlsErr):
		return errTLS
	}
	return errConnect
}

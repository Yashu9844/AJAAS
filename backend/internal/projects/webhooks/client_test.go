package webhooks

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testSender talks to an httptest TLS server through the test-only relaxations (loopback and any port are allowed, the
// server certificate is trusted); everything else - redirects, sizes, timeouts, proxy - is the production code path.
func testSender(t *testing.T, srv *httptest.Server, timeout time.Duration) *Sender {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return newSender(senderOptions{rules: urlRules{allowLoopback: true}, tls: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, timeout: timeout})
}

func TestSenderDeliversSignedRequest(t *testing.T) {
	var gotBody, gotSig, gotEvent, gotMethod string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotSig, gotEvent, gotMethod = string(b), r.Header.Get("X-JAAS-Signature"), r.Header.Get("X-JAAS-Event"), r.Method
		w.WriteHeader(204)
	}))
	defer srv.Close()
	body := []byte(`{"a":1}`)
	res := testSender(t, srv, 0).Do(context.Background(), Request{URL: srv.URL + "/hook", Body: body,
		Headers: map[string]string{"X-JAAS-Event": "task.created", "X-JAAS-Signature": Sign([]byte("k"), 100, body)}})
	if !res.OK() || res.Status != 204 {
		t.Fatalf("delivery: %+v", res)
	}
	if gotMethod != "POST" || gotBody != string(body) || gotEvent != "task.created" || !strings.HasPrefix(gotSig, "t=100,v1=") {
		t.Fatalf("server saw %s %q %q %q", gotMethod, gotBody, gotEvent, gotSig)
	}
}

func TestSenderStatusAndTLSFailures(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	if res := testSender(t, srv, 0).Do(context.Background(), Request{URL: srv.URL, Body: []byte("{}")}); res.OK() || res.Status != 500 || res.Err != "http 500" {
		t.Fatalf("5xx must be a failure with the code: %+v", res)
	}
	// An untrusted certificate (production roots) is a TLS failure, and the error text never carries the URL.
	plain := newSender(senderOptions{rules: urlRules{allowLoopback: true}})
	res := plain.Do(context.Background(), Request{URL: srv.URL + "/secret-path", Body: []byte("{}")})
	if res.OK() || res.Err != errTLS || strings.Contains(res.Err, "secret-path") {
		t.Fatalf("untrusted certificate: %+v", res)
	}
}

func TestSenderRefusesRedirects(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		}
	}))
	defer srv.Close()
	res := testSender(t, srv, 0).Do(context.Background(), Request{URL: srv.URL + "/start", Body: []byte("{}")})
	if res.OK() || res.Err != errRedirect {
		t.Fatalf("a redirect is a failure: %+v", res)
	}
	if hits.Load() != 1 {
		t.Fatalf("the redirect target must not be requested, hits=%d", hits.Load())
	}
	bare := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusMovedPermanently) }))
	defer bare.Close()
	if res := testSender(t, bare, 0).Do(context.Background(), Request{URL: bare.URL, Body: []byte("{}")}); res.OK() || res.Err != errRedirect || res.Status != 301 {
		t.Fatalf("a 3xx without Location is still a failure: %+v", res)
	}
}

func TestSenderTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	start := time.Now()
	res := testSender(t, srv, 150*time.Millisecond).Do(context.Background(), Request{URL: srv.URL, Body: []byte("{}")})
	if res.OK() || res.Err != errTimeout {
		t.Fatalf("a slow receiver times out: %+v", res)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("the timeout did not cut the request short")
	}
	if SendTimeout != 5*time.Second || MaxRequestBody != 64<<10 || MaxResponseRead != 4<<10 {
		t.Fatal("the documented limits changed")
	}
}

func TestSenderSizeLimits(t *testing.T) {
	var written atomic.Int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 64<<10)
		for i := 0; i < 1024; i++ { // up to 64 MB offered; the client reads 4 KB and hangs up
			n, err := w.Write(chunk)
			written.Add(int64(n))
			if err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	s := testSender(t, srv, 0)
	if res := s.Do(context.Background(), Request{URL: srv.URL, Body: []byte("{}")}); !res.OK() {
		t.Fatalf("a large response body is read in part and discarded: %+v", res)
	}
	if written.Load() >= 64<<20 {
		t.Fatal("the response was read to the end")
	}
	big := make([]byte, MaxRequestBody+1)
	if res := s.Do(context.Background(), Request{URL: srv.URL, Body: big}); res.Err != errTooLarge {
		t.Fatalf("request bodies over 64 KB are refused before sending: %+v", res)
	}
	if res := s.Do(context.Background(), Request{URL: srv.URL, Body: make([]byte, MaxRequestBody)}); !res.OK() {
		t.Fatalf("exactly 64 KB is allowed: %+v", res)
	}
}

func TestSenderIgnoresProxyEnvironment(t *testing.T) {
	var hit atomic.Bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit.Store(true) }))
	defer srv.Close()
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	if res := testSender(t, srv, 0).Do(context.Background(), Request{URL: srv.URL, Body: []byte("{}")}); !res.OK() || !hit.Load() {
		t.Fatalf("the proxy variables must be ignored: %+v", res)
	}
}

func TestSenderRefusesBadURLsAndAddresses(t *testing.T) {
	s := NewSender()
	for _, raw := range []string{"http://example.com/", "https://127.0.0.1/", "https://[::ffff:127.0.0.1]/", "https://169.254.169.254/latest/meta-data/",
		"https://user:pw@example.com/", "https://example.com:8080/", "https://localhost/", "https://0x7f.1/", ""} {
		if res := s.Do(context.Background(), Request{URL: raw, Body: []byte("{}")}); res.OK() || res.Err != errBadURL || res.Status != 0 {
			t.Errorf("%q must be refused at send time: %+v", raw, res)
		}
	}
	if res := s.Do(context.Background(), Request{URL: "https://example.com/\x7f", Body: nil}); res.OK() {
		t.Error("a control character in the URL fails")
	}
}

// DNS rebinding: the host is resolved when the connection is made and every answer is checked, so a name that first
// resolves to a public address and later to an internal one is stopped on the second request, before any connection.
func TestSenderStopsDNSRebinding(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	var lookups, connects atomic.Int32
	answers := [][]netip.Addr{{netip.MustParseAddr("93.184.216.34")}, {netip.MustParseAddr("127.0.0.1")}, {netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.7")},
		{netip.MustParseAddr("::ffff:169.254.169.254")}, {}}
	s := newSender(senderOptions{
		tls: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		lookup: func(_ context.Context, host string) ([]netip.Addr, error) {
			n := int(lookups.Add(1)) - 1
			if host != "example.com" || n >= len(answers) {
				return nil, errors.New("no such host")
			}
			return answers[n], nil
		},
		connect: func(ctx context.Context, network, addr string) (net.Conn, error) {
			connects.Add(1)
			if addr != "93.184.216.34:443" {
				t.Errorf("unexpected dial target %s", addr)
			}
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		},
	})
	req := Request{URL: "https://example.com/hook", Body: []byte("{}")}
	if res := s.Do(context.Background(), req); !res.OK() {
		t.Fatalf("a public answer is allowed: %+v", res)
	}
	for i := 1; i < len(answers); i++ {
		if res := s.Do(context.Background(), req); res.OK() || res.Err != errBlocked {
			t.Fatalf("answer %d (%v) must be blocked: %+v", i, answers[i], res)
		}
	}
	if connects.Load() != 1 {
		t.Fatalf("no connection may be opened to a blocked answer, connects=%d", connects.Load())
	}
	if res := s.Do(context.Background(), Request{URL: "https://other.example.org/", Body: []byte("{}")}); res.OK() || res.Err != errConnect {
		t.Fatalf("a resolver failure is a connection failure: %+v", res)
	}
}

// The socket-level control re-checks the address actually connected to (second line of defence).
func TestSocketControlChecksTheConnectedAddress(t *testing.T) {
	s := NewSender()
	for addr, want := range map[string]error{"127.0.0.1:443": ErrAddressBlocked, "10.0.0.1:443": ErrAddressBlocked, "[::1]:443": ErrAddressBlocked,
		"169.254.169.254:80": ErrAddressBlocked, "93.184.216.34:22": ErrURLPort, "bad": ErrAddressBlocked, "name:443": ErrAddressBlocked} {
		if err := s.control("tcp", addr, nil); !errors.Is(err, want) {
			t.Errorf("control(%s) = %v, want %v", addr, err, want)
		}
	}
	for _, addr := range []string{"93.184.216.34:443", "[2606:4700::1111]:8443"} {
		if err := s.control("tcp", addr, nil); err != nil {
			t.Errorf("control(%s) = %v", addr, err)
		}
	}
	if _, err := s.dial(context.Background(), "tcp", "no-port"); err == nil {
		t.Error("an address without port is refused")
	}
	if _, err := s.dial(context.Background(), "tcp", "127.0.0.1:443"); !errors.Is(err, ErrAddressBlocked) {
		t.Errorf("a literal loopback is refused before any socket: %v", err)
	}
}

func TestClassifyErrors(t *testing.T) {
	if classify(errors.New("anything with https://secret.example/token")) != errConnect {
		t.Error("unknown errors collapse to a fixed text")
	}
	if classify(context.DeadlineExceeded) != errTimeout || classify(ErrAddressBlocked) != errBlocked || classify(&tls.CertificateVerificationError{}) != errTLS {
		t.Error("classification")
	}
}

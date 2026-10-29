// Package pmclient is a small REST client for the JAAS Projects & Tasks API. The CLI (cmd/jaas-pm) and the MCP server
// (cmd/jaas-pm-mcp) are built on it; it depends on the standard library only and talks to the public contract
// (backend/api/swagger.yaml), never to internal packages.
package pmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// maxResponse bounds how much of a response body is read (protects the CLI and the MCP server from huge payloads).
const maxResponse = 4 << 20

// Tokens are the credentials of a signed-in user.
type Tokens struct {
	Access    string    `json:"access_token"`
	Refresh   string    `json:"refresh_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// APIError is an error envelope returned by the API.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("%s (%d): %s", e.Code, e.Status, e.Message) }

// Client calls the API of one tenant.
type Client struct {
	BaseURL string // for example http://localhost:8080
	Tenant  string // tenant slug; the API resolves the tenant from the Host header <slug>.<base host>
	Host    string // optional Host header override
	HTTP    *http.Client
	// OnTokens, when set, is called after every login or refresh so callers can persist the new credentials.
	OnTokens func(Tokens)

	mu     sync.Mutex
	tokens Tokens
}

// New builds a client with a 30 second timeout.
func New(baseURL, tenant string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Tenant: tenant, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Tokens returns a copy of the current credentials (for saving them).
func (c *Client) Tokens() Tokens {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens
}

// SetTokens installs credentials loaded from storage.
func (c *Client) SetTokens(t Tokens) {
	c.mu.Lock()
	c.tokens = t
	c.mu.Unlock()
}

func (c *Client) store(t Tokens) {
	c.SetTokens(t)
	if c.OnTokens != nil {
		c.OnTokens(t)
	}
}

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Meta  json.RawMessage `json:"meta"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Login signs in with e-mail and password and keeps the tokens.
func (c *Client) Login(ctx context.Context, email, password string) error {
	var t Tokens
	err := c.send(ctx, req{method: http.MethodPost, path: "/api/v1/auth/login", noAuth: true,
		body: map[string]string{"tenant_slug": c.Tenant, "email": email, "password": password}}, &t, nil)
	if err != nil {
		return err
	}
	c.store(t)
	return nil
}

func (c *Client) refresh(ctx context.Context) error {
	cur := c.Tokens()
	if cur.Refresh == "" {
		return errors.New("not signed in: run login first")
	}
	var t Tokens
	if err := c.send(ctx, req{method: http.MethodPost, path: "/api/v1/auth/refresh", noAuth: true,
		body: map[string]string{"refresh_token": cur.Refresh}}, &t, nil); err != nil {
		return err
	}
	c.store(t)
	return nil
}

// req describes one HTTP call.
type req struct {
	method, path string
	query        url.Values
	body         any
	noAuth       bool
	headers      map[string]string
}

// CallOpts carries the optional query string and JSON body of a call.
type CallOpts struct {
	Query url.Values
	Body  any
}

// Call performs an authenticated call and decodes the data of the envelope into out (may be nil). meta, when not nil,
// receives the raw meta object (pagination). A 401 triggers one token refresh and one retry.
func (c *Client) Call(ctx context.Context, method, path string, o CallOpts, out any) (json.RawMessage, error) {
	var meta json.RawMessage
	r := req{method: method, path: "/api/v1" + path, query: o.Query, body: o.Body}
	err := c.send(ctx, r, out, &meta)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized && c.Tokens().Refresh != "" {
		if rerr := c.refresh(ctx); rerr != nil {
			return nil, rerr
		}
		err = c.send(ctx, r, out, &meta)
	}
	return meta, err
}

func (c *Client) send(ctx context.Context, r req, out any, meta *json.RawMessage) error {
	var rd io.Reader
	if r.body != nil {
		raw, err := json.Marshal(r.body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	u := c.BaseURL + r.path
	if len(r.query) > 0 {
		u += "?" + r.query.Encode()
	}
	hr, err := http.NewRequestWithContext(ctx, r.method, u, rd)
	if err != nil {
		return err
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "application/json")
	for k, v := range r.headers {
		hr.Header.Set(k, v)
	}
	if host := c.hostHeader(); host != "" {
		hr.Host = host
	}
	if !r.noAuth {
		hr.Header.Set("Authorization", "Bearer "+c.Tokens().Access)
	}
	return c.finish(hr, out, meta)
}

func (c *Client) hostHeader() string {
	if c.Host != "" {
		return c.Host
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || c.Tenant == "" {
		return ""
	}
	return c.Tenant + "." + u.Host
}

func (c *Client) finish(hr *http.Request, out any, meta *json.RawMessage) error {
	resp, err := c.HTTP.Do(hr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return err
	}
	var env envelope
	_ = json.Unmarshal(raw, &env)
	if resp.StatusCode >= 400 {
		ae := &APIError{Status: resp.StatusCode, Code: "HTTP_ERROR", Message: http.StatusText(resp.StatusCode)}
		if env.Error != nil {
			ae.Code, ae.Message = env.Error.Code, env.Error.Message
		}
		return ae
	}
	if meta != nil {
		*meta = env.Meta
	}
	if out != nil && len(env.Data) > 0 {
		return json.Unmarshal(env.Data, out)
	}
	return nil
}

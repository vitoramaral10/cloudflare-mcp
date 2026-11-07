// Package cloudflare talks to the Cloudflare API v4 and exposes MCP tools on top of it.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const BaseURL = "https://api.cloudflare.com/client/v4"

// Client is a minimal API v4 client. It resolves zone names, tunnel names and the default account.
type Client struct {
	Base    string
	Token   string
	Account string // CLOUDFLARE_ACCOUNT_ID; empty = discovered via the API if there is only one account
	HTTP    *http.Client

	mu    sync.Mutex
	cache map[string]string
}

func NewClient(token, account string) *Client {
	return &Client{Base: BaseURL, Token: token, Account: account, HTTP: &http.Client{Timeout: 60 * time.Second}, cache: map[string]string{}}
}

// Envelope is the standard API v4 response.
type Envelope struct {
	Success    bool            `json:"success"`
	Errors     []APIError      `json:"errors"`
	Messages   []any           `json:"messages,omitempty"`
	Result     json.RawMessage `json:"result"`
	ResultInfo json.RawMessage `json:"result_info,omitempty"`
}

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Do calls the API. path is relative to /client/v4 (e.g. "/zones").
func (c *Client) Do(ctx context.Context, method, path string, q url.Values, body any) (*Envelope, error) {
	u := c.Base + "/" + strings.TrimPrefix(path, "/")
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		// Some endpoints (e.g. Worker script content) do not return the envelope.
		if resp.StatusCode < 300 {
			s, _ := json.Marshal(string(raw))
			return &Envelope{Success: true, Result: s}, nil
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}
	if !env.Success || resp.StatusCode >= 400 {
		return &env, apiErr(resp.StatusCode, env.Errors)
	}
	return &env, nil
}

func apiErr(status int, errs []APIError) error {
	var parts []string
	for _, e := range errs {
		parts = append(parts, fmt.Sprintf("%d: %s", e.Code, e.Message))
	}
	msg := fmt.Sprintf("Cloudflare HTTP %d: %s", status, strings.Join(parts, "; "))
	if status == 403 || status == 401 {
		msg += " (the token may lack the required permission for this resource)"
	}
	return errors.New(msg)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

var hexID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (c *Client) cached(key string, fetch func() (string, error)) (string, error) {
	c.mu.Lock()
	v, ok := c.cache[key]
	c.mu.Unlock()
	if ok {
		return v, nil
	}
	v, err := fetch()
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.cache[key] = v
	c.mu.Unlock()
	return v, nil
}

// firstID returns the id of the only (or first) item in a listing.
func (c *Client) firstID(ctx context.Context, path string, q url.Values, what string) (string, error) {
	env, err := c.Do(ctx, http.MethodGet, path, q, nil)
	if err != nil {
		return "", err
	}
	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(env.Result, &items); err != nil || len(items) == 0 {
		return "", fmt.Errorf("%s not found", what)
	}
	return items[0].ID, nil
}

// ZoneID accepts the id (32 hex) or the zone name (e.g. "example.com").
func (c *Client) ZoneID(ctx context.Context, zone string) (string, error) {
	zone = strings.TrimSpace(strings.ToLower(zone))
	if zone == "" {
		return "", errors.New("zone parameter is required (zone name or id)")
	}
	if hexID.MatchString(zone) {
		return zone, nil
	}
	return c.cached("zone:"+zone, func() (string, error) {
		return c.firstID(ctx, "/zones", url.Values{"name": {zone}}, "zone "+zone)
	})
}

// ZoneForHost finds the zone of a hostname by trying its suffixes (a.b.example.com → b.example.com → example.com).
func (c *Client) ZoneForHost(ctx context.Context, host string) (string, error) {
	labels := strings.Split(strings.TrimSuffix(strings.ToLower(host), "."), ".")
	for i := 0; i+2 <= len(labels); i++ {
		if id, err := c.ZoneID(ctx, strings.Join(labels[i:], ".")); err == nil {
			return id, nil
		}
	}
	return "", fmt.Errorf("no zone in the account covers %s", host)
}

// AccountID uses the given value, the configured one, or the only account the token can see.
func (c *Client) AccountID(ctx context.Context, given string) (string, error) {
	if given != "" {
		return given, nil
	}
	if c.Account != "" {
		return c.Account, nil
	}
	return c.cached("account", func() (string, error) {
		env, err := c.Do(ctx, http.MethodGet, "/accounts", nil, nil)
		if err != nil {
			return "", err
		}
		var items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(env.Result, &items)
		if len(items) != 1 {
			var opts []string
			for _, a := range items {
				opts = append(opts, a.ID+" ("+a.Name+")")
			}
			return "", fmt.Errorf("the token sees %d accounts, set account_id: %s", len(items), strings.Join(opts, ", "))
		}
		return items[0].ID, nil
	})
}

// TunnelID accepts the tunnel id (UUID) or name.
func (c *Client) TunnelID(ctx context.Context, account, tunnel string) (string, error) {
	if tunnel == "" {
		return "", errors.New("tunnel parameter is required (name or id)")
	}
	if uuidRe.MatchString(tunnel) {
		return tunnel, nil
	}
	return c.cached("tunnel:"+account+":"+tunnel, func() (string, error) {
		return c.firstID(ctx, "/accounts/"+account+"/cfd_tunnel", url.Values{"name": {tunnel}, "is_deleted": {"false"}}, "tunnel "+tunnel)
	})
}

package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// upsertRule puts the rule before the catch-all, or replaces the rule with the same hostname+path.
// It returns true if it replaced an existing one.
func upsertRule(ingress []any, rule map[string]any) ([]any, bool, error) {
	for i, r := range ingress {
		m, _ := r.(map[string]any)
		if m["hostname"] == rule["hostname"] && str(m["path"]) == str(rule["path"]) {
			ingress[i] = rule
			return ingress, true, nil
		}
	}
	n := len(ingress)
	if n == 0 {
		return []any{rule, map[string]any{"service": "http_status:404"}}, false, nil
	}
	last, _ := ingress[n-1].(map[string]any)
	if str(last["hostname"]) != "" {
		return nil, false, errors.New("the last ingress rule is not a catch-all; fix the config first")
	}
	out := append(append(append([]any{}, ingress[:n-1]...), rule), ingress[n-1])
	return out, false, nil
}

// removeRule drops the rule with the same hostname+path.
func removeRule(ingress []any, hostname, path string) ([]any, bool) {
	out := make([]any, 0, len(ingress))
	found := false
	for _, r := range ingress {
		m, _ := r.(map[string]any)
		if m["hostname"] == hostname && str(m["path"]) == path {
			found = true
			continue
		}
		out = append(out, r)
	}
	return out, found
}

// tunnelConfig reads the remote config; it returns the whole config object (keeps warp-routing, originRequest…).
func (c *Client) tunnelConfig(ctx context.Context, account, tunnel string) (map[string]any, error) {
	env, err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/configurations", account, tunnel), nil, nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(env.Result, &res); err != nil {
		return nil, err
	}
	if res.Config == nil {
		return nil, errors.New("the tunnel has no remote config (it is managed by a local file, config_src=local)")
	}
	return res.Config, nil
}

func (c *Client) ensureCNAME(ctx context.Context, hostname, tunnel string) (string, error) {
	zone, err := c.ZoneForHost(ctx, hostname)
	if err != nil {
		return "", err
	}
	target := tunnel + ".cfargotunnel.com"
	env, err := c.Do(ctx, http.MethodGet, "/zones/"+zone+"/dns_records", url.Values{"name": {hostname}}, nil)
	if err != nil {
		return "", err
	}
	var recs []struct{ ID, Type, Content string }
	_ = json.Unmarshal(env.Result, &recs)
	if len(recs) > 0 {
		r := recs[0]
		if r.Type == "CNAME" && r.Content == target {
			return "CNAME already existed", nil
		}
		return "", fmt.Errorf("a %s record already exists for %s pointing to %s: change it with dns_record_update", r.Type, hostname, r.Content)
	}
	_, err = c.Do(ctx, http.MethodPost, "/zones/"+zone+"/dns_records", nil, map[string]any{"type": "CNAME", "name": hostname, "content": target, "proxied": true})
	if err != nil {
		return "", err
	}
	return "CNAME created → " + target, nil
}

func registerIngress(s *mcp.Server, c *Client) {
	s.AddTool(&mcp.Tool{
		Name: "tunnel_route_add",
		Description: "Publishes a hostname on a tunnel: inserts the ingress rule before the catch-all (or replaces the one with the same hostname/path) " +
			"and, with create_dns=true, creates the proxied CNAME to <tunnel>.cfargotunnel.com. Only for tunnels with a remote config.",
		InputSchema: schemaFor([]param{pAccount, pTunnel,
			{Name: "hostname", Type: "string", Desc: "Public hostname (e.g. app.example.com)", Required: true},
			{Name: "service", Type: "string", Desc: "Target (e.g. http://app:8080, http://localhost:3000, ssh://localhost:22, http_status:404)", Required: true},
			{Name: "path", Type: "string", Desc: "Optional path regex"},
			{Name: "origin_request", Type: "object", Desc: "Rule originRequest (e.g. {\"noTLSVerify\":true})"},
			{Name: "create_dns", Type: "boolean", Desc: "Create the hostname CNAME to the tunnel (default true)"},
		}, false),
		Annotations: annotations(false, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := parseArgs(req)
		if err != nil {
			return result(nil, err)
		}
		account, err := c.AccountID(ctx, str(args["account_id"]))
		if err != nil {
			return result(nil, err)
		}
		tunnel, err := c.TunnelID(ctx, account, str(args["tunnel"]))
		if err != nil {
			return result(nil, err)
		}
		host, service := str(args["hostname"]), str(args["service"])
		if host == "" || service == "" {
			return result(nil, errors.New("hostname and service are required"))
		}
		cfg, err := c.tunnelConfig(ctx, account, tunnel)
		if err != nil {
			return result(nil, err)
		}
		rule := map[string]any{"hostname": host, "service": service}
		if p := str(args["path"]); p != "" {
			rule["path"] = p
		}
		if o, ok := args["origin_request"].(map[string]any); ok {
			rule["originRequest"] = o
		}
		ingress, _ := cfg["ingress"].([]any)
		ingress, replaced, err := upsertRule(ingress, rule)
		if err != nil {
			return result(nil, err)
		}
		cfg["ingress"] = ingress
		if _, err := c.Do(ctx, http.MethodPut, fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/configurations", account, tunnel), nil, map[string]any{"config": cfg}); err != nil {
			return result(nil, err)
		}
		msg := "rule added"
		if replaced {
			msg = "existing rule replaced"
		}
		if v, ok := args["create_dns"].(bool); !ok || v {
			dns, err := c.ensureCNAME(ctx, host, tunnel)
			if err != nil {
				dns = "ingress saved, but DNS failed: " + err.Error()
			}
			msg += "; " + dns
		}
		return textResult(msg), nil
	})

	s.AddTool(&mcp.Tool{
		Name:        "tunnel_route_remove",
		Description: "Removes a hostname's ingress rule from the tunnel. Does not delete the CNAME (use dns_records_list + dns_records_delete). Destructive.",
		InputSchema: schemaFor([]param{pAccount, pTunnel,
			{Name: "hostname", Type: "string", Desc: "Rule hostname", Required: true},
			{Name: "path", Type: "string", Desc: "Rule path, if any"},
		}, false),
		Annotations: annotations(false, true),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := parseArgs(req)
		if err != nil {
			return result(nil, err)
		}
		account, err := c.AccountID(ctx, str(args["account_id"]))
		if err != nil {
			return result(nil, err)
		}
		tunnel, err := c.TunnelID(ctx, account, str(args["tunnel"]))
		if err != nil {
			return result(nil, err)
		}
		cfg, err := c.tunnelConfig(ctx, account, tunnel)
		if err != nil {
			return result(nil, err)
		}
		ingress, _ := cfg["ingress"].([]any)
		ingress, found := removeRule(ingress, str(args["hostname"]), str(args["path"]))
		if !found {
			return result(nil, fmt.Errorf("no rule for %s on the tunnel", str(args["hostname"])))
		}
		cfg["ingress"] = ingress
		if _, err := c.Do(ctx, http.MethodPut, fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/configurations", account, tunnel), nil, map[string]any{"config": cfg}); err != nil {
			return result(nil, err)
		}
		return textResult("rule removed"), nil
	})
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

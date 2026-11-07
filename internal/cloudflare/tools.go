package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Instructions is sent on initialize: the map the model reads before choosing a tool.
const Instructions = `Cloudflare API v4 tools: one per operation of the official specification, named after the SDK (dns_records_list, tunnels_config_update, zero_trust_access_applications_create…).
- zone_id accepts the zone name (e.g. "example.com") or the id; tunnel_id and tunnel accept a name or an id; account_id is optional (uses the default account).
- If the right tool is not in view, use find (search by meaning across all operations) and then call with the name it returns.
- For operations with a body, the body fields go as top-level arguments, or inside "body" when the schema says so.
- To publish a service on a tunnel use tunnel_route_add (inserts before the catch-all and can create the CNAME); to remove it, tunnel_route_remove.
- Listings paginate with page/per_page; the response carries result_info.
- Anything without a dedicated tool goes through api_request, with the API v4 path (e.g. "/zones/{zone}/email/routing/rules", with zone="example.com").
- Tools marked as destructive change production: confirm with the user before deleting.`

type param struct {
	Name, Type, Desc string
	Required         bool
	Enum             []string
}

var (
	pAccount = param{Name: "account_id", Type: "string", Desc: "Account id; omit to use the default account"}
	pTunnel  = param{Name: "tunnel", Type: "string", Desc: "Tunnel name or id", Required: true}
)

var placeholder = regexp.MustCompile(`\{(\w+)\}`)

func schemaFor(ps []param, open bool) map[string]any {
	props := map[string]any{}
	req := []string{}
	for _, p := range ps {
		s := map[string]any{"description": p.Desc}
		if p.Type != "" {
			s["type"] = p.Type
		}
		if p.Type == "array" {
			s["items"] = map[string]any{}
		}
		if len(p.Enum) > 0 {
			s["enum"] = p.Enum
		}
		props[p.Name] = s
		if p.Required {
			req = append(req, p.Name)
		}
	}
	out := map[string]any{"type": "object", "properties": props, "additionalProperties": open}
	if len(req) > 0 {
		out["required"] = req
	}
	return out
}

// resolvePath replaces the placeholders and removes the consumed keys from args.
func (c *Client) resolvePath(ctx context.Context, path string, args map[string]any) (string, error) {
	var account string
	if strings.Contains(path, "{account}") || strings.Contains(path, "{tunnel}") {
		a, err := c.AccountID(ctx, str(args["account_id"]))
		if err != nil {
			return "", err
		}
		account = a
		delete(args, "account_id")
	}
	var firstErr error
	out := placeholder.ReplaceAllStringFunc(path, func(m string) string {
		name := m[1 : len(m)-1]
		var v string
		var err error
		switch name {
		case "account":
			v = account
		case "zone":
			v, err = c.ZoneID(ctx, str(args["zone"]))
		case "tunnel":
			v, err = c.TunnelID(ctx, account, str(args["tunnel"]))
		default:
			v = str(args[name])
			if v == "" {
				err = fmt.Errorf("parameter %s is required", name)
			}
		}
		delete(args, name)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		return url.PathEscape(v)
	})
	return out, firstErr
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return fmt.Sprintf("%v", t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func toQuery(args map[string]any) url.Values {
	q := url.Values{}
	for k, v := range args {
		switch t := v.(type) {
		case []any:
			for _, x := range t {
				q.Add(k, str(x))
			}
		case map[string]any: // Cloudflare filters: name={"exact":"x"} → name.exact=x
			for sub, x := range toQuery(t) {
				q[k+"."+sub] = x
			}
		default:
			q.Set(k, str(v))
		}
	}
	return q
}

func result(env *Envelope, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
	}
	out := map[string]any{"result": env.Result}
	if len(env.ResultInfo) > 0 && string(env.ResultInfo) != "null" {
		out["result_info"] = env.ResultInfo
	}
	b, _ := json.MarshalIndent(out, "", " ")
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
}

func parseArgs(req *mcp.CallToolRequest) (map[string]any, error) {
	args := map[string]any{}
	if raw := req.Params.Arguments; len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	return args, nil
}

func annotations(readOnly, destructive bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: readOnly, DestructiveHint: &destructive}
}

// Register adds the operations generated from the spec and the own tools to the server.
func Register(s *mcp.Server, c *Client, ops []Op, ix *Index) {
	registerOps(s, c, ops)
	registerFind(s, c, ix)
	registerIngress(s, c)
	registerRequest(s, c)
}

func registerRequest(s *mcp.Server, c *Client) {
	s.AddTool(&mcp.Tool{
		Name: "api_request",
		Description: "Free-form Cloudflare API v4 call, for what the operation tools cannot do: non-JSON bodies (Worker script uploads), the GraphQL analytics API (POST /graphql) or endpoints newer than the catalog. " +
			"path starts with /, without /client/v4 (e.g. /zones/{zone}/email/routing/rules). {zone}, {account} and {tunnel} in the path are resolved from zone, account_id and tunnel.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"method":     map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE"}},
				"path":       map[string]any{"type": "string", "description": "API v4 path"},
				"query":      map[string]any{"type": "object", "description": "Query parameters"},
				"body":       map[string]any{"description": "JSON body"},
				"zone":       map[string]any{"type": "string", "description": "Used by {zone} in the path"},
				"account_id": map[string]any{"type": "string", "description": "Used by {account} in the path"},
				"tunnel":     map[string]any{"type": "string", "description": "Used by {tunnel} in the path"},
			},
			"required": []string{"method", "path"},
		},
		Annotations: annotations(false, true),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := parseArgs(req)
		if err != nil {
			return result(nil, err)
		}
		method := strings.ToUpper(str(args["method"]))
		ref := map[string]any{"zone": args["zone"], "account_id": args["account_id"], "tunnel": args["tunnel"]}
		path, err := c.resolvePath(ctx, str(args["path"]), ref)
		if err != nil {
			return result(nil, err)
		}
		q := url.Values{}
		if m, ok := args["query"].(map[string]any); ok {
			q = toQuery(m)
		}
		return result(c.Do(ctx, method, path, q, args["body"]))
	})
}

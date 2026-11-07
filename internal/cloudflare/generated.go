package cloudflare

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Op is an API operation, generated from the OpenAPI spec by cmd/cloudflare-mcp-catalog.
type Op struct {
	Name     string         `json:"n"`
	Method   string         `json:"m"`
	Path     string         `json:"p"`
	Desc     string         `json:"d,omitempty"`
	Params   []OpParam      `json:"a,omitempty"`
	Body     map[string]any `json:"b,omitempty"`
	BodyFlat bool           `json:"f,omitempty"`  // body properties become top-level arguments
	Tags     []string       `json:"t,omitempty"`  // spec tags ("Zone Settings", "Lists")
	Keywords []string       `json:"k,omitempty"`  // enum values from the body and parameters, search only
	PT       string         `json:"pt,omitempty"` // Portuguese sentence about the task (scripts/gen_pt_texts.py), search only
	Queries  []string       `json:"q,omitempty"`  // Portuguese user requests; each becomes a search vector
}

type OpParam struct {
	Name     string         `json:"n"`
	In       string         `json:"i"` // path | query
	Required bool           `json:"r,omitempty"`
	Schema   map[string]any `json:"s,omitempty"`
}

//go:embed catalog.json.gz
var catalogGz []byte

func LoadOps() ([]Op, error) {
	zr, err := gzip.NewReader(bytes.NewReader(catalogGz))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var ops []Op
	return ops, json.NewDecoder(zr).Decode(&ops)
}

// Placeholders with special resolution: they accept a name instead of an id, and account_id has a default.
var resolvable = map[string]string{
	"account_id": "Account id; omit to use the default account",
	"zone_id":    "Zone name (e.g. example.com) or id",
	"tunnel_id":  "Tunnel name or id",
}

func opSchema(op Op) map[string]any {
	props := map[string]any{}
	req := []string{}
	for _, p := range op.Params {
		s := map[string]any{}
		for k, v := range p.Schema {
			s[k] = v
		}
		if d, ok := resolvable[p.Name]; ok && p.In == "path" {
			s = map[string]any{"type": "string", "description": d}
			if p.Name == "account_id" {
				props[p.Name] = s
				continue
			}
		}
		props[p.Name] = s
		if p.Required {
			req = append(req, p.Name)
		}
	}
	if op.Body != nil {
		if bp, ok := op.Body["properties"].(map[string]any); ok && op.BodyFlat {
			for k, v := range bp {
				props[k] = v
			}
			if r, ok := op.Body["required"].([]any); ok {
				for _, x := range r {
					req = append(req, str(x))
				}
			}
		} else {
			props["body"] = op.Body
		}
	}
	out := map[string]any{"type": "object", "properties": props}
	if len(req) > 0 {
		out["required"] = req
	}
	return out
}

// callOp builds the path, query and body from the arguments.
func (c *Client) callOp(ctx context.Context, op Op, args map[string]any) (*Envelope, error) {
	pathVals := map[string]string{}
	q := url.Values{}
	account := ""
	needAccount := false
	for _, p := range op.Params {
		if p.In == "path" && (p.Name == "account_id" || p.Name == "tunnel_id") {
			needAccount = true
		}
	}
	if needAccount {
		a, err := c.AccountID(ctx, str(args["account_id"]))
		if err != nil {
			return nil, err
		}
		account = a
	}
	for _, p := range op.Params {
		v, present := args[p.Name]
		delete(args, p.Name)
		if p.In == "query" {
			if present {
				for k, vs := range toQuery(map[string]any{p.Name: v}) {
					q[k] = vs
				}
			}
			continue
		}
		var s string
		var err error
		switch p.Name {
		case "account_id":
			s = account
		case "zone_id":
			s, err = c.ZoneID(ctx, str(v))
		case "tunnel_id":
			s, err = c.TunnelID(ctx, account, str(v))
		default:
			s = str(v)
			if s == "" {
				err = fmt.Errorf("parameter %s is required", p.Name)
			}
		}
		if err != nil {
			return nil, err
		}
		pathVals[p.Name] = url.PathEscape(s)
	}
	path := placeholder.ReplaceAllStringFunc(op.Path, func(m string) string {
		if v, ok := pathVals[m[1:len(m)-1]]; ok {
			return v
		}
		return m
	})
	var body any
	if op.Body != nil {
		if op.BodyFlat {
			if len(args) > 0 {
				body = args
			}
		} else {
			body = args["body"]
		}
	}
	return c.Do(ctx, op.Method, path, q, body)
}

func registerOps(s *mcp.Server, c *Client, ops []Op) {
	for _, op := range ops {
		op := op
		ro := op.Method == http.MethodGet
		s.AddTool(&mcp.Tool{Name: op.Name, Description: op.Desc, InputSchema: opSchema(op), Annotations: annotations(ro, op.Method == http.MethodDelete)},
			func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				args, err := parseArgs(req)
				if err != nil {
					return result(nil, err)
				}
				return result(c.callOp(ctx, op, args))
			})
	}
}

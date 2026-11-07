// cloudflare-mcp-catalog reads the Cloudflare OpenAPI spec and writes the compact catalog
// the server embeds: one entry per operation, with name, path and already-resolved schemas.
//
//	curl -sLo openapi.json https://raw.githubusercontent.com/cloudflare/api-schemas/main/openapi.json
//	go run ./cmd/cloudflare-mcp-catalog openapi.json internal/cloudflare/catalog.json.gz internal/cloudflare/descriptions_pt.json internal/cloudflare/queries_pt.json
package main

import (
	"compress/gzip"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/vitoramaral10/cloudflare-mcp/internal/cloudflare"
)

var spec map[string]any

var reduced, opaque int // bodies that went over the size limit

func main() {
	if len(os.Args) < 3 || len(os.Args) > 5 {
		log.Fatal("usage: cloudflare-mcp-catalog <openapi.json> <output.json.gz> [descriptions_pt.json [queries_pt.json]]")
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		log.Fatal(err)
	}
	ops := build()
	if len(os.Args) >= 4 {
		var pt map[string]string
		b, err := os.ReadFile(os.Args[3])
		if err != nil {
			log.Fatal(err)
		}
		if err := json.Unmarshal(b, &pt); err != nil {
			log.Fatal(err)
		}
		n := 0
		for i := range ops {
			if d, ok := pt[ops[i].Name]; ok {
				ops[i].PT = d
				n++
			}
		}
		log.Printf("Portuguese descriptions: %d of %d", n, len(ops))
	}
	if len(os.Args) == 5 {
		var qs map[string][]string
		b, err := os.ReadFile(os.Args[4])
		if err != nil {
			log.Fatal(err)
		}
		if err := json.Unmarshal(b, &qs); err != nil {
			log.Fatal(err)
		}
		n := 0
		for i := range ops {
			if q, ok := qs[ops[i].Name]; ok {
				ops[i].Queries = q
				n++
			}
		}
		log.Printf("Portuguese queries: %d of %d operations", n, len(ops))
	}
	f, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	zw, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err := json.NewEncoder(zw).Encode(ops); err != nil {
		log.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	log.Printf("%d operations; reduced bodies: %d, opaque: %d", len(ops), reduced, opaque)
}

var methods = []string{"get", "post", "put", "patch", "delete"}

func build() []cloudflare.Op {
	paths := spec["paths"].(map[string]any)
	keys := make([]string, 0, len(paths))
	for p := range paths {
		keys = append(keys, p)
	}
	sort.Strings(keys)

	var ops []cloudflare.Op
	count := map[string]int{}
	for _, p := range keys {
		item := paths[p].(map[string]any)
		shared, _ := item["parameters"].([]any)
		for _, m := range methods {
			o, ok := item[m].(map[string]any)
			if !ok {
				continue
			}
			op := cloudflare.Op{Name: baseName(o), Method: strings.ToUpper(m), Path: p, Desc: scope(p) + describe(o)}
			op.Params = params(append(append([]any{}, shared...), asSlice(o["parameters"])...))
			if op.Method != "GET" && op.Method != "DELETE" {
				op.Body, op.BodyFlat = body(o, op.Params)
			}
			for _, t := range asSlice(o["tags"]) {
				op.Tags = append(op.Tags, str(t))
			}
			op.Keywords = keywords(o, append(append([]any{}, shared...), asSlice(o["parameters"])...))
			count[op.Name]++
			ops = append(ops, op)
		}
	}
	// a duplicate or too-long name gets the operationId or a short hash
	for i := range ops {
		if count[ops[i].Name] > 1 {
			ops[i].Name = slug(opID(paths, ops[i]))
		}
	}
	seen := map[string]bool{}
	for i := range ops {
		n := ops[i].Name
		if len(n) > 60 || seen[n] {
			h := sha1.Sum([]byte(ops[i].Method + ops[i].Path))
			n = strings.TrimRight(n[:min(len(n), 51)], "_") + "_" + hex.EncodeToString(h[:4])
		}
		seen[n] = true
		ops[i].Name = n
	}
	return ops
}

func opID(paths map[string]any, op cloudflare.Op) string {
	o := paths[op.Path].(map[string]any)[strings.ToLower(op.Method)].(map[string]any)
	return str(o["operationId"])
}

func baseName(o map[string]any) string {
	m := str(o["x-fern-sdk-method-name"])
	var g []string
	switch v := o["x-fern-sdk-group-name"].(type) {
	case string:
		g = []string{v}
	case []any:
		for _, x := range v {
			g = append(g, str(x))
		}
	}
	if m == "" || len(g) == 0 {
		return slug(str(o["operationId"]))
	}
	return slug(strings.Join(append(g, m), "_"))
}

var camel = regexp.MustCompile(`([a-z0-9])([A-Z])`)
var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.ToLower(camel.ReplaceAllString(s, "${1}_${2}"))
	return strings.Trim(nonAlnum.ReplaceAllString(s, "_"), "_")
}

// scope marks the operation level: the SDK gives account and zone variants the same base name.
func scope(path string) string {
	switch {
	case strings.HasPrefix(path, "/accounts/{account_id}"):
		return "[account] "
	case strings.HasPrefix(path, "/zones/{zone_id}"):
		return "[zone] "
	case strings.HasPrefix(path, "/user"):
		return "[user] "
	}
	return ""
}

func describe(o map[string]any) string {
	sum := strings.TrimSpace(str(o["summary"]))
	d := strings.TrimSpace(str(o["description"]))
	if d == "" || d == sum {
		return sum
	}
	return trunc(sum+". "+d, 400)
}

func params(list []any) []cloudflare.OpParam {
	var out []cloudflare.OpParam
	seen := map[string]bool{}
	for _, x := range list {
		p := deref(x)
		in := str(p["in"])
		name := str(p["name"])
		if (in != "path" && in != "query") || name == "" || seen[in+name] {
			continue
		}
		seen[in+name] = true
		s := schema(p["schema"], 0)
		if d := str(p["description"]); d != "" {
			s["description"] = trunc(d, 200)
		}
		req, _ := p["required"].(bool)
		out = append(out, cloudflare.OpParam{Name: name, In: in, Required: req || in == "path", Schema: s})
	}
	return out
}

// body returns the JSON body schema and whether it can be "flattened" into the arguments.
func body(o map[string]any, ps []cloudflare.OpParam) (map[string]any, bool) {
	rb := deref(o["requestBody"])
	content, _ := rb["content"].(map[string]any)
	var raw any
	for ct, v := range content {
		if strings.Contains(ct, "json") {
			raw = v.(map[string]any)["schema"]
			break
		}
	}
	if raw == nil {
		if len(content) > 0 {
			return map[string]any{"description": "non-JSON body (multipart/binary): use api_request"}, false
		}
		return nil, false
	}
	s := schema(raw, 0)
	if b, _ := json.Marshal(s); len(b) > 12000 {
		reduced++
		s = schema(raw, 3)
	}
	if b, _ := json.Marshal(s); len(b) > 12000 {
		opaque++
		s = map[string]any{"type": "object", "description": "large object: see the operation documentation"}
	}
	props, ok := s["properties"].(map[string]any)
	if s["type"] != "object" || !ok {
		return s, false
	}
	for _, p := range ps {
		if _, clash := props[p.Name]; clash {
			return s, false
		}
	}
	return s, true
}

const maxDepth = 5

// schema resolves $ref and allOf and compacts: it drops examples, x-*, read-only fields and shortens descriptions.
func schema(x any, depth int) map[string]any {
	s := deref(x)
	if depth >= maxDepth {
		out := map[string]any{}
		if t, ok := s["type"]; ok {
			out["type"] = t
		}
		return out
	}
	if all, ok := s["allOf"].([]any); ok {
		merged := map[string]any{"type": "object"}
		props := map[string]any{}
		var req []any
		for _, part := range all {
			ps := schema(part, depth)
			if pp, ok := ps["properties"].(map[string]any); ok {
				for k, v := range pp {
					props[k] = v
				}
			}
			if r, ok := ps["required"].([]any); ok {
				req = append(req, r...)
			}
			if d, ok := ps["description"]; ok {
				merged["description"] = d
			}
			if t, ok := ps["type"]; ok && t != "object" && len(all) == 1 {
				return ps
			}
		}
		if len(props) > 0 {
			merged["properties"] = props
		}
		if len(req) > 0 {
			merged["required"] = req
		}
		return merged
	}
	out := map[string]any{}
	for _, k := range []string{"type", "format", "default", "minimum", "maximum", "pattern"} {
		if v, ok := s[k]; ok {
			out[k] = v
		}
	}
	if d := str(s["description"]); d != "" {
		out["description"] = trunc(d, 160)
	}
	if e, ok := s["enum"].([]any); ok && len(e) <= 60 {
		out["enum"] = e
	}
	if it, ok := s["items"]; ok {
		out["items"] = schema(it, depth+1)
	}
	if props, ok := s["properties"].(map[string]any); ok {
		pp := map[string]any{}
		for k, v := range props {
			vs := deref(v)
			if ro, _ := vs["readOnly"].(bool); ro {
				continue
			}
			pp[k] = schema(v, depth+1)
		}
		out["properties"] = pp
		if r, ok := s["required"].([]any); ok {
			var keep []any
			for _, k := range r {
				if _, ok := pp[str(k)]; ok {
					keep = append(keep, k)
				}
			}
			if len(keep) > 0 {
				out["required"] = keep
			}
		}
	}
	for _, k := range []string{"oneOf", "anyOf"} {
		alts, ok := s[k].([]any)
		if !ok {
			continue
		}
		var rs []map[string]any
		for _, a := range alts {
			rs = append(rs, schema(a, depth))
		}
		if merged, ok := mergeObjects(rs); ok {
			for key, v := range merged {
				if _, has := out[key]; !has || key == "properties" {
					out[key] = v
				}
			}
			continue
		}
		var list []any
		for i, r := range rs {
			if i == 12 {
				break
			}
			list = append(list, r)
		}
		out[k] = list
	}
	if ap, ok := s["additionalProperties"].(map[string]any); ok {
		out["additionalProperties"] = schema(ap, depth+1)
	}
	return out
}

// mergeObjects merges alternatives that are all objects into a single object: union of the
// fields, enums of the same field combined, required = what all of them require. For the model,
// seeing every possible field (e.g. DNS record types) beats eight variants.
func mergeObjects(alts []map[string]any) (map[string]any, bool) {
	if len(alts) == 0 {
		return nil, false
	}
	props := map[string]any{}
	reqCount := map[string]int{}
	for _, a := range alts {
		pp, ok := a["properties"].(map[string]any)
		if !ok || (a["type"] != nil && a["type"] != "object") {
			return nil, false
		}
		for name, v := range pp {
			vm, _ := v.(map[string]any)
			prev, seen := props[name].(map[string]any)
			if !seen {
				props[name] = vm
				continue
			}
			if e1, ok := prev["enum"].([]any); ok {
				if e2, ok := vm["enum"].([]any); ok {
					cp := map[string]any{}
					for k, x := range prev {
						cp[k] = x
					}
					cp["enum"] = unionAny(e1, e2)
					props[name] = cp
				}
			}
		}
		if r, ok := a["required"].([]any); ok {
			for _, x := range r {
				reqCount[str(x)]++
			}
		}
	}
	out := map[string]any{"type": "object", "properties": props}
	var req []any
	for name, n := range reqCount {
		if n == len(alts) {
			req = append(req, name)
		}
	}
	if len(req) > 0 {
		sort.Slice(req, func(i, j int) bool { return str(req[i]) < str(req[j]) })
		out["required"] = req
	}
	return out, true
}

func unionAny(a, b []any) []any {
	seen := map[string]bool{}
	var out []any
	for _, x := range append(append([]any{}, a...), b...) {
		k := fmt.Sprint(x)
		if !seen[k] {
			seen[k] = true
			out = append(out, x)
		}
	}
	return out
}

var noise = map[string]bool{"on": true, "off": true, "true": true, "false": true, "none": true, "null": true, "all": true, "any": true, "default": true}

// keywords collects enum values (including from read-only fields) of the body and parameters:
// that is where names the description does not mention live, like the zone setting ids.
func keywords(o map[string]any, ps []any) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(x any, depth int)
	walk = func(x any, depth int) {
		if depth > 8 || len(out) >= 120 {
			return
		}
		m := deref(x)
		if e, ok := m["enum"].([]any); ok {
			for _, v := range e {
				w, ok := v.(string)
				if ok && len(w) >= 3 && len(w) <= 40 && !noise[strings.ToLower(w)] && !seen[w] && len(out) < 120 {
					seen[w] = true
					out = append(out, w)
				}
			}
		}
		for _, k := range []string{"properties"} {
			if pp, ok := m[k].(map[string]any); ok {
				keys := make([]string, 0, len(pp))
				for name := range pp {
					keys = append(keys, name)
				}
				sort.Strings(keys)
				for _, name := range keys {
					walk(pp[name], depth+1)
				}
			}
		}
		for _, k := range []string{"oneOf", "anyOf", "allOf"} {
			for _, a := range asSlice(m[k]) {
				walk(a, depth+1)
			}
		}
		if it, ok := m["items"]; ok {
			walk(it, depth+1)
		}
	}
	for _, p := range ps {
		walk(deref(p)["schema"], 0)
	}
	// request body and 2xx responses (the zone setting ids only appear in the response)
	sources := []map[string]any{deref(o["requestBody"])}
	if rs, ok := o["responses"].(map[string]any); ok {
		codes := make([]string, 0, len(rs))
		for code := range rs {
			if strings.HasPrefix(code, "2") {
				codes = append(codes, code)
			}
		}
		sort.Strings(codes)
		for _, code := range codes {
			sources = append(sources, deref(rs[code]))
		}
	}
	for _, rb := range sources {
		content, ok := rb["content"].(map[string]any)
		if !ok {
			continue
		}
		cts := make([]string, 0, len(content))
		for ct := range content {
			cts = append(cts, ct)
		}
		sort.Strings(cts)
		for _, ct := range cts {
			walk(content[ct].(map[string]any)["schema"], 0)
		}
	}
	return out
}

func deref(x any) map[string]any {
	m, _ := x.(map[string]any)
	for i := 0; m != nil && i < 10; i++ {
		ref, ok := m["$ref"].(string)
		if !ok {
			return m
		}
		m = lookup(ref)
	}
	if m == nil {
		return map[string]any{}
	}
	return m
}

func lookup(ref string) map[string]any {
	var cur any = spec
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[part]
	}
	m, _ := cur.(map[string]any)
	return m
}

func asSlice(x any) []any { s, _ := x.([]any); return s }

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

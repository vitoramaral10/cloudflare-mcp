package cloudflare

import (
	"context"
	"regexp"
	"testing"
)

func TestUpsertRule(t *testing.T) {
	catchAll := map[string]any{"service": "http_status:404"}
	tests := []struct {
		name      string
		ingress   []any
		rule      map[string]any
		wantLen   int
		wantAt    int
		replaced  bool
		wantError bool
	}{
		{"empty gets a catch-all", nil, map[string]any{"hostname": "a.x", "service": "http://a"}, 2, 0, false, false},
		{"inserts before the catch-all", []any{map[string]any{"hostname": "b.x", "service": "http://b"}, catchAll}, map[string]any{"hostname": "a.x", "service": "http://a"}, 3, 1, false, false},
		{"replaces the existing one", []any{map[string]any{"hostname": "a.x", "service": "http://old"}, catchAll}, map[string]any{"hostname": "a.x", "service": "http://a"}, 2, 0, true, false},
		{"fails without a catch-all", []any{map[string]any{"hostname": "b.x", "service": "http://b"}}, map[string]any{"hostname": "a.x", "service": "http://a"}, 0, 0, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, replaced, err := upsertRule(tt.ingress, tt.rule)
			if (err != nil) != tt.wantError {
				t.Fatalf("err = %v", err)
			}
			if err != nil {
				return
			}
			if len(got) != tt.wantLen || replaced != tt.replaced {
				t.Fatalf("len=%d replaced=%v, want %d %v", len(got), replaced, tt.wantLen, tt.replaced)
			}
			if got[tt.wantAt].(map[string]any)["service"] != "http://a" {
				t.Fatalf("rule out of place: %v", got)
			}
			if str(got[len(got)-1].(map[string]any)["hostname"]) != "" {
				t.Fatalf("catch-all did not stay at the end: %v", got)
			}
		})
	}
}

func TestRemoveRule(t *testing.T) {
	in := []any{map[string]any{"hostname": "a.x", "service": "http://a"}, map[string]any{"service": "http_status:404"}}
	out, found := removeRule(in, "a.x", "")
	if !found || len(out) != 1 {
		t.Fatalf("found=%v out=%v", found, out)
	}
	if _, found := removeRule(out, "a.x", ""); found {
		t.Fatal("removed what did not exist")
	}
}

var validName = regexp.MustCompile(`^[a-z0-9_]{1,60}$`)

func TestGeneratedCatalog(t *testing.T) {
	ops, err := LoadOps()
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) < 3000 {
		t.Fatalf("only %d operations", len(ops))
	}
	seen := map[string]bool{"tunnel_route_add": true, "tunnel_route_remove": true, "api_request": true, "find": true, "call": true}
	for _, op := range ops {
		if !validName.MatchString(op.Name) || seen[op.Name] {
			t.Errorf("invalid or duplicate name: %q", op.Name)
		}
		seen[op.Name] = true
		declared := map[string]bool{}
		for _, p := range op.Params {
			if p.In == "path" {
				declared[p.Name] = true
			}
		}
		for _, m := range placeholder.FindAllStringSubmatch(op.Path, -1) {
			if !declared[m[1]] {
				t.Errorf("%s: {%s} has no parameter", op.Name, m[1])
			}
		}
	}
}

func TestToQueryDeepObject(t *testing.T) {
	q := toQuery(map[string]any{"name": map[string]any{"exact": "a.x"}, "type": "CNAME", "tag": []any{"a", "b"}})
	if q.Get("name.exact") != "a.x" || q.Get("type") != "CNAME" || len(q["tag"]) != 2 {
		t.Fatalf("query = %v", q.Encode())
	}
}

func TestKeywordSearch(t *testing.T) {
	ops, err := LoadOps()
	if err != nil {
		t.Fatal(err)
	}
	ix := NewIndex(ops, "", "") // no embedding: falls back to keyword
	got, mode, _ := ix.Search(context.Background(), "purge cache", 5)
	if mode != "keyword" || len(got) == 0 {
		t.Fatalf("mode=%s got=%d", mode, len(got))
	}
	found := false
	for _, op := range got {
		found = found || op.Name == "cache_purge"
	}
	if !found {
		t.Fatalf("cache_purge outside the top 5: %v", got)
	}
}

func TestIndexCacheRoundTrip(t *testing.T) {
	ops := []Op{{Name: "a_list", Desc: "x"}, {Name: "b_get", Desc: "y"}}
	ix := NewIndex(ops, "", "m")
	ix.CacheDir = t.TempDir()
	want := [][]float32{{1, 0, -0.5}, {0.25, 2, 3}}
	if err := ix.saveCache(want); err != nil {
		t.Fatal(err)
	}
	got, err := ix.loadCache()
	if err != nil || len(got) != 2 || got[1][2] != 3 || got[0][2] != -0.5 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	other := NewIndex(append(ops, Op{Name: "c"}), "", "m") // catalog changed: different file
	other.CacheDir = ix.CacheDir
	if _, err := other.loadCache(); err == nil {
		t.Fatal("reused the cache of another catalog")
	}
}

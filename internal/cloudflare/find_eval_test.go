package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type evalCase struct {
	Q  string   `json:"q"`
	OK []string `json:"ok"`
}

// eval_tuning.json was used to tune the glossary and guides, so its numbers are optimistic;
// eval_holdout_pt*.json were never used for tuning and measure whether the gains generalize;
// eval_holdout_en.json is the pt1 requests rewritten as a model would send them (English,
// Cloudflare terms).
// eval_holdout_pt*_rewritten.json are the same requests rewritten in English by an LLM (Claude
// Haiku) before the search, as the model calling find is asked to do.
var evalFiles = []string{
	"testdata/eval_tuning.json", "testdata/eval_holdout_en.json",
	"testdata/eval_holdout_pt1.json", "testdata/eval_holdout_pt2.json", "testdata/eval_holdout_pt3.json",
	"testdata/eval_holdout_pt1_rewritten.json", "testdata/eval_holdout_pt2_rewritten.json", "testdata/eval_holdout_pt3_rewritten.json",
}

func loadEval(t *testing.T, file string) []evalCase {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var cs []evalCase
	if err := json.Unmarshal(b, &cs); err != nil {
		t.Fatal(err)
	}
	return cs
}

// The answer key only counts if it points at operations that exist.
func TestEvalNamesExist(t *testing.T) {
	ops, _ := LoadOps()
	names := map[string]bool{"tunnel_route_add": true, "tunnel_route_remove": true}
	for _, op := range ops {
		names[op.Name] = true
	}
	for _, f := range evalFiles {
		for _, c := range loadEval(t, f) {
			for _, n := range c.OK {
				if !names[n] {
					t.Errorf("%q: operation %s does not exist", c.Q, n)
				}
			}
		}
	}
}

// TestFindEval measures the top-5 hit rate. Hybrid search needs an embedding server and the cache:
//
//	EVAL_EMBED_URL=http://127.0.0.1:18090 EVAL_CACHE_DIR=/path/to/cache go test -run FindEval -v ./internal/cloudflare
func TestFindEval(t *testing.T) {
	ops, _ := LoadOps()
	files := evalFiles
	if f := os.Getenv("EVAL_FILES"); f != "" { // other answer keys, e.g. queries rewritten by an LLM
		files = strings.Fields(f)
	}
	run := func(label string, ix *Index) {
		for _, f := range files {
			runFile(t, label+" "+strings.TrimSuffix(filepath.Base(f), ".json"), ix, loadEval(t, f))
		}
	}
	run("keyword", NewIndex(ops, "", ""))

	url, dir := os.Getenv("EVAL_EMBED_URL"), os.Getenv("EVAL_CACHE_DIR")
	if url == "" || dir == "" {
		t.Log("hybrid skipped: set EVAL_EMBED_URL and EVAL_CACHE_DIR")
		return
	}
	ix := NewIndex(ops, url, "bge-m3")
	ix.CacheDir = dir
	vecs, err := ix.loadCache()
	if err != nil {
		t.Fatalf("cache: %v (generate it by running the server with CACHE_DIR=%s)", err, dir)
	}
	ix.setVecs(vecs)
	plain := NewIndex(ops, url, "bge-m3")
	plain.Plain = true
	plain.setVecs(vecs)
	run("hybrid without guides", plain)
	run("hybrid", ix)
	if os.Getenv("EVAL_EXPANDED_SEM") != "" { // query with the glossary in the embedding too
		RawSemantic = false
		run("hybrid, glossary in embedding", ix)
		RawSemantic = true
	}
	if w := os.Getenv("EVAL_SEM_WEIGHTS"); w != "" { // ex.: "0.5 1.5 2 3"
		for _, f := range strings.Fields(w) {
			v, _ := strconv.ParseFloat(f, 64)
			old := semWeight
			semWeight = v
			run("semantic weight "+f, ix)
			semWeight = old
		}
	}
}

func runFile(t *testing.T, label string, ix *Index, cases []evalCase) {
	if ks := os.Getenv("EVAL_K"); ks != "" { // top-K hit rate for several K, totals only
		var parts []string
		for _, f := range strings.Fields(ks) {
			k, _ := strconv.Atoi(f)
			n := 0
			for _, c := range cases {
				got, _, _ := ix.Search(context.Background(), c.Q, k)
				if slices.ContainsFunc(got, func(op Op) bool { return slices.Contains(c.OK, op.Name) }) {
					n++
				}
			}
			parts = append(parts, fmt.Sprintf("top%d=%.0f%%", k, 100*float64(n)/float64(len(cases))))
		}
		t.Logf("[%s] %s", label, strings.Join(parts, " "))
		return
	}
	hits := 0
	for _, c := range cases {
		got, _, _ := ix.Search(context.Background(), c.Q, 5)
		ok := slices.ContainsFunc(got, func(op Op) bool { return slices.Contains(c.OK, op.Name) })
		if ok {
			hits++
		} else {
			var names []string
			for _, op := range got {
				names = append(names, op.Name)
			}
			t.Logf("[%s] miss: %q → %v", label, c.Q, names)
		}
	}
	t.Logf("[%s] top-5 hits: %d/%d (%.0f%%)", label, hits, len(cases), 100*float64(hits)/float64(len(cases)))
}

func TestLooksPortuguese(t *testing.T) {
	for q, want := range map[string]bool{
		"bloquear tráfego de um país": true, "quero ver os buckets": true, "cria um A apontando pro ip": true,
		"WAF custom rules block country": false, "purge cache files": false, "listar workers": true,
	} {
		if got := looksPortuguese(q); got != want {
			t.Errorf("%q: %v, want %v", q, got, want)
		}
	}
}

func TestGuideTriggers(t *testing.T) {
	ops := func(q string) []string {
		if looksPortuguese(q) {
			q = expandQuery(q)
		}
		var out []string
		for _, g := range matchGuides(q) {
			out = append(out, g.ops[0])
		}
		return out
	}
	cases := map[string][]string{
		// English: only the intent's guide, without the ones stray words used to trigger
		"Access application protect subdomain login": {"zero_trust_access_applications_create"},
		"block traffic from a country":               {"rulesets_zone_entrypoint_update"},
		"create DNS record in zone":                  {"dns_records_create"},
		"create Access service token":                nil,
		"rate limit the login form":                  {"rulesets_zone_entrypoint_update"},
		"https redirect for www":                     {"rulesets_zone_entrypoint_update"},
		"Workers KV namespace create":                nil,
		"purge cache files":                          {"cache_purge"},
		"pages project build health":                 nil,
		// Portuguese still goes through the glossary
		"bloquear tráfego de um país":     {"rulesets_zone_entrypoint_update"},
		"ver quantas visitas o site teve": {"analytics_dashboard_get"},
		"proteger um site com login":      {"zero_trust_access_applications_create"},
	}
	for q, want := range cases {
		if got := ops(q); !slices.Equal(got, want) {
			t.Errorf("%q: guides %v, want %v", q, got, want)
		}
	}
}

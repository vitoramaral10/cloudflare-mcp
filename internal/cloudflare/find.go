package cloudflare

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Index finds operations by meaning (embeddings) and, without the embedding service,
// by keyword. Vectors are computed once, in the background, at startup.
type Index struct {
	ops    []Op
	byName map[string]int
	names  [][]string // name tokens, for the fallback (weigh more)
	words  [][]string // description tokens

	embedURL, model string
	CacheDir        string // where to store the vectors; empty = recompute on every start
	Plain           bool   // no glossary or guides (only to measure their gain)
	http            *http.Client

	// Texts with a vector: each operation's docText and each request generated for it (multi-vector).
	// An operation's semantic score is that of its text closest to the query.
	texts []string
	owner []int // owner[j] = index of the operation that owns texts[j]

	mu   sync.RWMutex
	vecs [][]float32 // nil until the semantic index is ready; aligned with texts
}

// RawSemantic sends the original query, without the glossary, to the embedding.
var RawSemantic = true

func NewIndex(ops []Op, embedURL, model string) *Index {
	ix := &Index{ops: ops, byName: map[string]int{}, embedURL: strings.TrimRight(embedURL, "/"), model: model,
		http: &http.Client{Timeout: 60 * time.Second}}
	for i, op := range ops {
		ix.byName[op.Name] = i
		ix.names = append(ix.names, tokens(strings.ReplaceAll(op.Name, "_", " ")))
		ix.words = append(ix.words, tokens(docText(op)+" "+strings.Join(op.Queries, " ")))
		ix.texts = append(ix.texts, docText(op))
		ix.owner = append(ix.owner, i)
		for _, q := range op.Queries {
			ix.texts = append(ix.texts, q)
			ix.owner = append(ix.owner, i)
		}
	}
	return ix
}

// docText is what the search sees of each operation: name, tags, path words,
// enum values (e.g. the zone setting names) and the description.
func docText(op Op) string {
	var path []string
	for _, seg := range strings.Split(op.Path, "/") {
		if seg != "" && !strings.HasPrefix(seg, "{") {
			path = append(path, strings.NewReplacer("_", " ", "-", " ").Replace(seg))
		}
	}
	parts := []string{strings.ReplaceAll(op.Name, "_", " ")}
	if op.PT != "" {
		parts = append(parts, op.PT)
	}
	if len(op.Tags) > 0 {
		parts = append(parts, strings.Join(op.Tags, ", "))
	}
	parts = append(parts, strings.Join(path, " "))
	if len(op.Keywords) > 0 {
		parts = append(parts, strings.ReplaceAll(strings.Join(op.Keywords, " "), "_", " "))
	}
	return strings.Join(append(parts, op.Desc), ". ")
}

// Build computes the vectors of all operations. It retries until it succeeds.
func (ix *Index) Build(ctx context.Context) {
	if ix.embedURL == "" {
		log.Print("search: EMBED_URL empty, keyword only")
		return
	}
	if vecs, err := ix.loadCache(); err == nil {
		ix.setVecs(vecs)
		log.Printf("search: semantic index loaded from cache (%d vectors, %d operations)", len(vecs), len(ix.ops))
		return
	}
	log.Printf("search: computing embeddings for %d texts of %d operations (may take many minutes)", len(ix.texts), len(ix.ops))
	for attempt := 1; ; attempt++ {
		start := time.Now()
		vecs, err := ix.embedAll(ctx)
		if err == nil {
			ix.setVecs(vecs)
			log.Printf("search: semantic index ready (%d vectors, %s)", len(vecs), time.Since(start).Round(time.Second))
			if err := ix.saveCache(vecs); err != nil {
				log.Printf("search: cache not saved: %v", err)
			}
			return
		}
		log.Printf("search: embedding failed (attempt %d), falling back to keyword: %v", attempt, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(min(attempt, 10)) * time.Minute):
		}
	}
}

func (ix *Index) setVecs(v [][]float32) {
	ix.mu.Lock()
	ix.vecs = v
	ix.mu.Unlock()
}

// cachePath changes when the catalog or the model changes: stale vectors are never reused.
func (ix *Index) cachePath() string {
	h := sha256.New()
	h.Write([]byte(ix.model))
	for _, t := range ix.texts {
		h.Write([]byte{0})
		h.Write([]byte(t))
	}
	return filepath.Join(ix.CacheDir, "index-"+hex.EncodeToString(h.Sum(nil))[:16]+".bin")
}

// Format: n and dim (uint32 LE), then n*dim float32 LE.
func (ix *Index) loadCache() ([][]float32, error) {
	if ix.CacheDir == "" {
		return nil, errors.New("no cache")
	}
	b, err := os.ReadFile(ix.cachePath())
	if err != nil {
		return nil, err
	}
	if len(b) < 8 {
		return nil, errors.New("cache too short")
	}
	n, dim := int(binary.LittleEndian.Uint32(b)), int(binary.LittleEndian.Uint32(b[4:]))
	if n != len(ix.texts) || len(b) != 8+n*dim*4 {
		return nil, errors.New("cache does not match the catalog")
	}
	out := make([][]float32, n)
	off := 8
	for i := range out {
		v := make([]float32, dim)
		for j := range v {
			v[j] = math.Float32frombits(binary.LittleEndian.Uint32(b[off:]))
			off += 4
		}
		out[i] = v
	}
	return out, nil
}

func (ix *Index) saveCache(vecs [][]float32) error {
	if ix.CacheDir == "" || len(vecs) == 0 {
		return nil
	}
	dim := len(vecs[0])
	b := make([]byte, 8, 8+len(vecs)*dim*4)
	binary.LittleEndian.PutUint32(b, uint32(len(vecs)))
	binary.LittleEndian.PutUint32(b[4:], uint32(dim))
	for _, v := range vecs {
		if len(v) != dim {
			return errors.New("vectors with different dimensions")
		}
		for _, x := range v {
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(x))
		}
	}
	path := ix.cachePath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (ix *Index) embedAll(ctx context.Context) ([][]float32, error) {
	const batch = 32
	out := make([][]float32, 0, len(ix.texts))
	for i := 0; i < len(ix.texts); i += batch {
		vs, err := ix.embed(ctx, ix.texts[i:min(i+batch, len(ix.texts))])
		if err != nil {
			return nil, err
		}
		out = append(out, vs...)
	}
	return out, nil
}

func (ix *Index) embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": ix.model, "input": texts})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ix.embedURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ix.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed HTTP %d", resp.StatusCode)
	}
	var r struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if len(r.Data) != len(texts) {
		return nil, fmt.Errorf("embed returned %d vectors for %d texts", len(r.Data), len(texts))
	}
	out := make([][]float32, len(texts))
	for _, d := range r.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return nil, errors.New("embed: index outside the batch")
		}
		out[d.Index] = normalize(d.Embedding)
	}
	return out, nil
}

func normalize(v []float32) []float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	n := float32(math.Sqrt(s))
	if n == 0 {
		return v
	}
	for i := range v {
		v[i] /= n
	}
	return v
}

type hit struct {
	i     int
	score float64
}

// Search returns the best operations and the mode used ("hybrid" or "keyword").
// Hybrid = RRF fusion of the semantic and keyword rankings: the embedding finds by
// meaning (including Portuguese) and the keyword holds on to the exact name (cache_purge).
func (ix *Index) Search(ctx context.Context, query string, limit int) ([]Op, string, []string) {
	hits, mode, query := ix.rank(ctx, query)
	// intent guides first (up to 3 operations), then the ranking
	var out []Op
	var tips []string
	seen := map[int]bool{}
	var matched []guide
	if !ix.Plain {
		matched = matchGuides(query)
	}
	for _, g := range matched {
		if g.tip != "" {
			tips = append(tips, g.tip)
		}
	}
	// one operation per guide per round, so one guide does not take every slot
	for round := 0; len(seen) < min(3, limit); round++ {
		added := false
		for _, g := range matched {
			if round >= len(g.ops) || len(seen) >= min(3, limit) {
				continue
			}
			added = true
			if i, ok := ix.byName[g.ops[round]]; ok && !seen[i] {
				seen[i] = true
				out = append(out, ix.ops[i])
			}
		}
		if !added {
			break
		}
	}
	for _, h := range hits {
		if len(out) >= limit {
			break
		}
		if h.score > 0 && !seen[h.i] {
			seen[h.i] = true
			out = append(out, ix.ops[h.i])
		}
	}
	return out, mode, tips
}

// rank orders the operations for a query; it also returns the query expanded by the glossary.
func (ix *Index) rank(ctx context.Context, query string) ([]hit, string, string) {
	raw := query
	if !ix.Plain && looksPortuguese(query) { // the glossary is Portuguese→English: on English queries it only hurts ("login" → access)
		query = expandQuery(query)
	}
	semQuery := query
	if RawSemantic {
		semQuery = raw
	}
	ix.mu.RLock()
	vecs := ix.vecs
	ix.mu.RUnlock()
	hits := ix.keyword(query)
	sortHits(hits)
	mode := "keyword"
	if vecs != nil {
		if q, err := ix.embed(ctx, []string{semQuery}); err == nil {
			mode = "hybrid"
			sem := make([]hit, len(ix.ops))
			for i := range sem {
				sem[i] = hit{i, math.Inf(-1)}
			}
			for j, v := range vecs {
				if s := dot(q[0], v); s > sem[ix.owner[j]].score {
					sem[ix.owner[j]].score = s
				}
			}
			sortHits(sem)
			hits = rrf(sem, hits)
		}
	}
	return hits, mode, query
}

func sortHits(h []hit) {
	sort.SliceStable(h, func(a, b int) bool { return h[a].score > h[b].score })
}

// rrf fuses already-sorted rankings: it sums 1/(k+position). In the keyword list only
// items that matched some term count, so the arbitrary order of zero scores is not a signal.
// semWeight is the weight of the semantic ranking in the fusion (1 = same as keyword).
var semWeight = 1.0

func rrf(sem, kw []hit) []hit {
	const k, depth = 10, 50
	score := map[int]float64{}
	for r, h := range sem[:min(depth, len(sem))] {
		score[h.i] += semWeight / float64(k+r+1)
	}
	for r, h := range kw[:min(depth, len(kw))] {
		if h.score <= 0 {
			break
		}
		score[h.i] += 1 / float64(k+r+1)
	}
	out := make([]hit, 0, len(score))
	for i, s := range score {
		out = append(out, hit{i, s})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].score != out[b].score {
			return out[a].score > out[b].score
		}
		return out[a].i < out[b].i
	})
	return out
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range min(len(a), len(b)) {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// keyword scores each query term: 3 if it is in the name, 1 if only in the description (a
// prefix of 4+ letters counts half). Ties favor the shorter name, usually the base operation.
func (ix *Index) keyword(query string) []hit {
	var q []string
	for _, t := range tokens(query) {
		if !stopwords[t] {
			q = append(q, t)
		}
	}
	var hits []hit
	for i := range ix.ops {
		var s float64
		for _, t := range q {
			s += max(3*match(ix.names[i], t), match(ix.words[i], t))
		}
		hits = append(hits, hit{i, s - float64(len(ix.names[i]))*0.01})
	}
	return hits
}

func match(ws []string, t string) float64 {
	best := 0.0
	for _, w := range ws {
		if w == t || stem(w) == stem(t) {
			return 1
		}
		if len(t) >= 4 && strings.HasPrefix(w, t) {
			best = 0.5
		}
	}
	return best
}

// stopwords do not score in keyword search: they would match almost every Portuguese description.
var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("a o as os um uma uns umas de do da dos das em no na nos nas num numa por pelo pela " +
		"para pra com sem que se e ou ao aos à às meu minha meus minhas seu sua seus suas eu me mim " +
		"quero queria preciso como qual quais é está ser ter tem the of to for in on an and or is") {
		stopwords[w] = true
	}
}

// looksPortuguese detects a Portuguese query: an accent or a word that does not exist in English.
func looksPortuguese(q string) bool {
	if strings.ContainsAny(strings.ToLower(q), "ãõçáéíóúâêôà") {
		return true
	}
	for _, t := range tokens(q) {
		if ptOnly[t] {
			return true
		}
	}
	return false
}

var ptOnly = map[string]bool{}

func init() {
	for _, w := range strings.Fields("o os um uma de do da dos das em no na nos nas para pra com sem que meu minha " +
		"quero preciso como qual quais ver criar apagar listar mudar trocar tirar colocar deixar fazer pelo pela") {
		ptOnly[w] = true
	}
}

// stem strips a simple English/Portuguese plural ("redirects" → "redirect").
func stem(w string) string {
	if len(w) > 4 && strings.HasSuffix(w, "s") {
		return w[:len(w)-1]
	}
	return w
}

func tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func registerFind(s *mcp.Server, c *Client, ix *Index) {
	s.AddTool(&mcp.Tool{
		Name: "find",
		Description: "Searches the ~3,780 Cloudflare API operations for the ones that fit a task. " +
			"ALWAYS write query in English using Cloudflare product terms, even if the user asked in another language " +
			"(ex.: \"purge cache files\", \"Single Redirects rule\", \"WAF custom rules zone entrypoint\", \"IP access rules\", " +
			"\"origin CA certificate\", \"Email Routing catch-all\", \"R2 bucket custom domain\"): accuracy drops a lot with Portuguese. " +
			"If the result does not fit, rephrase with other terms. Returns name, description and the arguments schema; " +
			"run the chosen one with call. Use when no visible tool solves the task.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "The task in English, with Cloudflare terms (translate the user request)"},
				"limit": map[string]any{"type": "integer", "description": "How many operations to return (default 5, max 15)"},
			},
			"required": []string{"query"},
		},
		Annotations: annotations(true, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := parseArgs(req)
		if err != nil {
			return result(nil, err)
		}
		q := strings.TrimSpace(str(args["query"]))
		if q == "" {
			return result(nil, errors.New("query is required"))
		}
		limit := 5
		if f, ok := args["limit"].(float64); ok && f >= 1 {
			limit = min(int(f), 15)
		}
		ops, mode, tips := ix.Search(ctx, q, limit)
		if looksPortuguese(q) {
			// measured: rewriting the request in English with Cloudflare terms raises the top-5
			// hit rate from ~70% to ~95% (testdata/eval_*)
			tips = append([]string{"Search works much better in English with Cloudflare terms: if the operations below do not fit, call find again with the request rewritten (e.g. \"WAF custom rule block country\", \"DNS records update proxied\")."}, tips...)
		}
		type found struct {
			Name        string         `json:"name"`
			Method      string         `json:"method"`
			Path        string         `json:"path"`
			Description string         `json:"description"`
			Arguments   map[string]any `json:"arguments_schema"`
		}
		res := map[string]any{"search": mode, "operations": []found{}}
		if len(tips) > 0 {
			res["tips"] = tips
		}
		var list []found
		for _, op := range ops {
			list = append(list, found{op.Name, op.Method, op.Path, op.Desc, opSchema(op)})
		}
		if list != nil {
			res["operations"] = list
		}
		b, _ := json.MarshalIndent(res, "", " ")
		return textResult(string(b)), nil
	})

	s.AddTool(&mcp.Tool{
		Name: "call",
		Description: "Runs a Cloudflare API operation by the name find returned, with the arguments from its arguments_schema. " +
			"zone_id accepts the domain name, tunnel_id the tunnel name, and account_id is optional. May change production: confirm before deleting.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":      map[string]any{"type": "string", "description": "Operation name (e.g. email_routing_rules_create)"},
				"arguments": map[string]any{"type": "object", "description": "Arguments per the arguments_schema returned by find"},
			},
			"required": []string{"name"},
		},
		Annotations: annotations(false, true),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := parseArgs(req)
		if err != nil {
			return result(nil, err)
		}
		name := str(args["name"])
		i, ok := ix.byName[name]
		if !ok { // the client may send the name with a prefix added by a gateway
			name = strings.TrimPrefix(name, "cloudflare_")
			i, ok = ix.byName[name]
		}
		if !ok {
			return result(nil, fmt.Errorf("operation %q does not exist: use find to look up the name", name))
		}
		opArgs, _ := args["arguments"].(map[string]any)
		if opArgs == nil {
			opArgs = map[string]any{}
		}
		return result(c.callOp(ctx, ix.ops[i], opArgs))
	})
}

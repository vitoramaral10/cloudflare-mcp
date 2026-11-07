// cloudflare-mcp: MCP server (streamable HTTP) exposing the Cloudflare API.
package main

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vitoramaral10/cloudflare-mcp/internal/cloudflare"
)

func main() {
	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	if token == "" {
		log.Fatal("CLOUDFLARE_API_TOKEN not set")
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	client := cloudflare.NewClient(token, os.Getenv("CLOUDFLARE_ACCOUNT_ID"))

	ops, err := cloudflare.LoadOps()
	if err != nil {
		log.Fatalf("embedded catalog: %v", err)
	}
	// High PageSize: some gateways list tools in a single call and do not follow the cursor.
	server := mcp.NewServer(&mcp.Implementation{Name: "cloudflare-mcp", Version: "0.7.0"},
		&mcp.ServerOptions{Instructions: cloudflare.Instructions, PageSize: 100000})
	model := os.Getenv("EMBED_MODEL")
	if model == "" {
		model = "bge-m3"
	}
	index := cloudflare.NewIndex(ops, os.Getenv("EMBED_URL"), model)
	index.CacheDir = os.Getenv("CACHE_DIR")
	go index.Build(context.Background())
	cloudflare.Register(server, client, ops, index)
	log.Printf("%d API operations + 5 own tools", len(ops))

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	mux := http.NewServeMux()
	mux.Handle("/mcp", bearer(os.Getenv("MCP_BEARER"), handler))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	log.Printf("cloudflare-mcp listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// bearer requires "Authorization: Bearer <secret>" when MCP_BEARER is set.
func bearer(secret string, next http.Handler) http.Handler {
	if secret == "" {
		log.Print("warning: MCP_BEARER empty, /mcp is open to anyone who can reach the port")
		return next
	}
	want := []byte("Bearer " + secret)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

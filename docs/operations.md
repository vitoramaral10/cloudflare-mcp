# Running in production

## Image and binary

Images are published to `ghcr.io/vitoramaral10/cloudflare-mcp` for `linux/amd64` and `linux/arm64`, tagged with the release (`v0.7.0`, `0.7`) and `latest`. They are built from `gcr.io/distroless/static-debian12:nonroot`: no shell, no package manager, runs as a non-root user and works with a read-only root filesystem.

The binary is static (`CGO_ENABLED=0`) and has the catalog embedded, so nothing else ships with it. Build it yourself with:

```sh
go build -trimpath -ldflags="-s -w" -o cloudflare-mcp ./cmd/cloudflare-mcp
```

## Configuration

| Variable | Default | |
|---|---|---|
| `CLOUDFLARE_API_TOKEN` | required | API token the server acts with. |
| `CLOUDFLARE_ACCOUNT_ID` | | Default account. Without it, the only account the token sees. |
| `MCP_BEARER` | | If set, `/mcp` requires `Authorization: Bearer <value>` (constant-time comparison). Without it the server logs a warning. |
| `ADDR` | `:8080` | Listen address. |
| `EMBED_URL` | | OpenAI-compatible embedding server, called at `/v1/embeddings`. Empty: keyword search only. |
| `EMBED_MODEL` | `bge-m3` | Model name sent to the embedding server; part of the cache key. |
| `CACHE_DIR` | | Directory for the vector cache. Without it, vectors are recomputed on every start. |
| `GOMEMLIMIT` | | Standard Go soft memory limit; set it a little below the container limit. |

Endpoints: `/mcp` (MCP over streamable HTTP) and `/healthz` (`ok`).

## Memory

Two things dominate memory:

- **`tools/list`**: the answer with all 3,782 tools is about 4.4 MB of JSON. The server sends it in one page, because some gateways read only the first page; building it briefly costs several times its size.
- **The vectors**: 22,662 × 1,024 floats, about 93 MB, kept in memory when semantic search is on.

Without semantic search, give the container about 192 MB (`GOMEMLIMIT=128MiB`); at 64 MB it ran out while answering `tools/list`. With semantic search, give it about 448 MB and set `GOMEMLIMIT=384MiB`; it settles around 300 MB.

## The vector cache

When `EMBED_URL` is set, the server embeds the text of every operation and its five Portuguese requests in the background, in batches of 32, and saves them to `CACHE_DIR/index-<hash>.bin`. Until that finishes, `find` answers by keyword. If the embedding server fails, it retries with backoff and keeps answering by keyword.

The hash covers the model name and every indexed text, so a new catalog or another `EMBED_MODEL` never reuses stale vectors: it writes a new file. Old files can be deleted.

Embedding 22,662 texts takes about 40 minutes on a laptop CPU and hours on a small integrated GPU. Ways to avoid paying that on the server:

- **Download the vectors from the release** you run (`index-<hash>.bin` is attached to each release) into `CACHE_DIR`.
- **Compute them on a faster machine** and copy the file over. The cache only depends on the catalog and on the model name, not on the hardware: vectors computed on CPU and on GPU from the same model file agree to a cosine of 0.998.

The query side always uses your embedding server, so it must serve the same model (bge-m3, any quantization, CLS pooling).

## Behind a gateway

The server speaks plain streamable HTTP and knows nothing about users. To expose it beyond localhost, put it behind an MCP gateway or reverse proxy that authenticates clients, and keep `MCP_BEARER` as the gateway-to-server secret. A gateway that filters tools by pattern is also the easiest place to apply a [starter tool set](/usage#a-starter-tool-set). If the gateway lists tools only once, restart it (or reconnect the upstream) after upgrading cloudflare-mcp, so it sees the new catalog.

## Updating the catalog

Cloudflare adds endpoints often. To pick them up:

```sh
curl -sLo /tmp/openapi.json https://raw.githubusercontent.com/cloudflare/api-schemas/main/openapi.json
go run ./cmd/cloudflare-mcp-catalog /tmp/openapi.json internal/cloudflare/catalog.json.gz
python3 scripts/gen_pt_texts.py --kind descriptions      # Portuguese texts for new operations only
python3 scripts/gen_pt_texts.py --kind queries --batch 40
go run ./cmd/cloudflare-mcp-catalog /tmp/openapi.json internal/cloudflare/catalog.json.gz \
  internal/cloudflare/descriptions_pt.json internal/cloudflare/queries_pt.json
python3 scripts/gen_catalog_doc.py
go test ./...
```

The first run writes a catalog without Portuguese texts so that `gen_pt_texts.py` can see the new operations; it calls the `claude` CLI and only asks for operations that have no text yet. Removed operations are dropped from both files. Any change to the catalog changes the cache key, so the vectors are recomputed (or downloaded) after an upgrade.

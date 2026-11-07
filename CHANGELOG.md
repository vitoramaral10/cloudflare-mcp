# Changelog

Notable changes, newest first. The format follows [Keep a Changelog](https://keepachangelog.com),
and versions follow [Semantic Versioning](https://semver.org).

## [0.7.0] - 2026-11-07

First public release.

### Added
- One MCP tool per Cloudflare API v4 operation (3,777), generated from Cloudflare's OpenAPI schema
  by `cloudflare-mcp-catalog` and embedded in the binary, named like the official SDKs and marked
  `[zone]`, `[account]` or `[user]`.
- Name resolution: `zone_id` accepts a zone name, `tunnel_id` a tunnel name, and `account_id`
  defaults to `CLOUDFLARE_ACCOUNT_ID` or the only account the token sees.
- `find` and `call`: search every operation for a task and run the one picked. Hybrid ranking
  (bge-m3 embeddings from any OpenAI-compatible server, plus keywords), fused with RRF; intent
  guides with usage tips for redirects, WAF, rate limiting, transform and cache rules, analytics,
  SSL, DNS, tunnels, Access and Email Routing; a tip asking for an English query when the query
  looks Portuguese.
- Portuguese search: an LLM-written description and five user-style requests per operation, each
  request with its own vector (22,662 vectors).
- Vector cache in `CACHE_DIR`, keyed by the catalog and the model name; precomputed vectors are
  attached to each release.
- `tunnel_route_add` and `tunnel_route_remove`: publish and remove a hostname on a Cloudflare
  Tunnel in one call, including the proxied CNAME.
- `api_request` for raw API v4 calls.
- Optional bearer authentication (`MCP_BEARER`), read-only and destructive hints on every tool.
- Search evaluation sets and `TestFindEval`.
- Documentation site, Docker images for `linux/amd64` and `linux/arm64`.

[0.7.0]: https://github.com/vitoramaral10/cloudflare-mcp/releases/tag/v0.7.0

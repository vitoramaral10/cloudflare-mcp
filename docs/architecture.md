# Architecture

<img src="/architecture.svg" alt="Architecture: an MCP client calls cloudflare-mcp, optionally through a gateway; the server calls the Cloudflare API v4 and, for find, an embedding server. At build time the OpenAPI spec and Portuguese texts become the embedded catalog.">

## Layout

| Path | |
|---|---|
| `cmd/cloudflare-mcp` | The server: reads the environment, loads the catalog, starts the index and serves `/mcp`. |
| `cmd/cloudflare-mcp-catalog` | Build-time generator: OpenAPI schema (+ Portuguese texts) → `catalog.json.gz`. |
| `internal/cloudflare/client.go` | API v4 client: envelope, errors, and name → id resolution for zones, tunnels and the account. |
| `internal/cloudflare/generated.go` | The embedded catalog and the code that turns an operation into a tool and a call into a request. |
| `internal/cloudflare/tools.go` | Server instructions, `api_request`, tool registration. |
| `internal/cloudflare/ingress.go` | `tunnel_route_add` and `tunnel_route_remove`. |
| `internal/cloudflare/find.go` | The search index, the vector cache, `find` and `call`. |
| `internal/cloudflare/intents.go` | Portuguese glossary and intent guides. |
| `scripts/gen_pt_texts.py` | Writes the Portuguese descriptions and requests with an LLM. |
| `scripts/gen_catalog_doc.py` | Writes the [operation catalog](/catalog) page. |

## From schema to catalog

`cloudflare-mcp-catalog` walks every path and method of [Cloudflare's OpenAPI schema](https://github.com/cloudflare/api-schemas) and writes one compact entry per operation:

- **Name**: the SDK group and method (`x-fern-sdk-group-name` + `x-fern-sdk-method-name`), snake-cased. Names shared by several operations fall back to the `operationId`; names over 60 characters are cut and get a hash of the method and path.
- **Description**: `[zone]`, `[account]` or `[user]` from the path, then the summary and description, cut at 400 characters.
- **Parameters**: path and query parameters with their schemas, descriptions cut at 200 characters.
- **Body**: the JSON request body with `$ref`s resolved (five levels deep), `allOf` merged, read-only fields dropped, examples and vendor keys removed. When every `oneOf`/`anyOf` alternative is an object, they are merged into one object with all fields, so the model sees, for instance, every DNS record type's fields at once. A body over 12 KB is rebuilt shallower; if it is still too big it becomes an opaque object. When its fields don't collide with parameters, the body is flattened into top-level arguments.
- **Search text**: the OpenAPI tags and up to 120 enum values found in the body, parameters and 2xx responses (zone setting names such as `min_tls_version` only appear in responses).
- **Portuguese texts**: one description and five requests per operation, merged in from `descriptions_pt.json` and `queries_pt.json`.

The result, `catalog.json.gz` (under 1 MB), is embedded with `go:embed`.

## A call

When a model calls an operation tool:

1. The arguments are split into path parameters, query parameters and the rest.
2. `account_id` resolves to the given id, `CLOUDFLARE_ACCOUNT_ID`, or the only account the token sees. `zone_id` accepts a name, looked up with `GET /zones?name=`; `tunnel_id` accepts a name, looked up in the account's tunnels. Lookups are cached for the life of the process.
3. Query objects become dotted keys (`name.exact=…`), arrays repeat the key.
4. For methods with a body, the remaining arguments (or `body`) are sent as JSON.
5. The response envelope is unwrapped: `result` and `result_info` on success; on failure, Cloudflare's error codes and messages, with a hint on 401 and 403, returned as a tool error the model can read.

## find

The index holds, for each operation, its text (name, Portuguese description, tags, path words, enum values and description) and its five Portuguese requests, each with its own vector. A query is ranked twice, by meaning and by keyword, and the rankings are fused. [How find works](/search) covers the details and how it is measured.

## Concurrency and state

The server keeps no state besides the lookup caches and the vector index, which is swapped in atomically when the background build finishes. Each MCP session is independent, and the same `mcp.Server` serves all of them.

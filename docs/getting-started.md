# Getting started

Four steps: an API token, the server, a client, and (optional) semantic search for `find`.

## 1. Create an API token

The server acts with one Cloudflare API token, and the model can do whatever that token can. Create it at [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens):

- **To look around first**, start from the *Read all resources* template. Every write fails with a permission error the model can read.
- **For day-to-day work**, create a custom token with only what you need, for example *Zone: DNS Edit*, *Zone: Cache Purge*, *Zone: Zone Settings Edit*, *Account: Cloudflare Tunnel Edit*, *Account: Access: Apps and Policies Edit*.
- Restrict it to the accounts and zones you mean. *All accounts* includes every account your user is a member of, other people's included.

A missing permission is not fatal: the call returns Cloudflare's error and a note that the token may lack the permission.

## 2. Run the server

With Docker:

```sh
docker run -d --name cloudflare-mcp -p 127.0.0.1:8080:8080 \
  -e CLOUDFLARE_API_TOKEN=... \
  -e MCP_BEARER="$(openssl rand -hex 32)" \
  ghcr.io/vitoramaral10/cloudflare-mcp:latest
```

Or with Go 1.27+:

```sh
go install github.com/vitoramaral10/cloudflare-mcp/cmd/cloudflare-mcp@latest
CLOUDFLARE_API_TOKEN=... MCP_BEARER=... cloudflare-mcp
```

The MCP endpoint is `http://127.0.0.1:8080/mcp` (streamable HTTP); `/healthz` answers `ok`. If the token sees more than one account, set `CLOUDFLARE_ACCOUNT_ID` to the default one; tools still take `account_id` per call.

::: warning Keep it private
Whoever reaches `/mcp` acts with your token. Bind to localhost, keep `MCP_BEARER` set, and put an authenticating gateway in front if the server must be reachable from elsewhere. See [SECURITY.md](https://github.com/vitoramaral10/cloudflare-mcp/blob/main/SECURITY.md).
:::

## 3. Connect a client

**Claude Code**

```sh
claude mcp add --transport http cloudflare http://127.0.0.1:8080/mcp \
  --header "Authorization: Bearer <MCP_BEARER>"
```

**VS Code** (`.vscode/mcp.json`)

```json
{
  "servers": {
    "cloudflare": {
      "type": "http",
      "url": "http://127.0.0.1:8080/mcp",
      "headers": { "Authorization": "Bearer <MCP_BEARER>" }
    }
  }
}
```

**Cursor** (`~/.cursor/mcp.json`)

```json
{
  "mcpServers": {
    "cloudflare": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": { "Authorization": "Bearer <MCP_BEARER>" }
    }
  }
}
```

The server lists 3,782 tools. Most clients cope badly with that many; [Using the tools](/usage#a-starter-tool-set) shows how to expose a short list plus `find` and `call`.

## 4. Turn on semantic search (optional)

Without an embedding server, `find` ranks by keyword. That works for English queries that use Cloudflare's terms. For meaning-based ranking, and for Portuguese, give it an OpenAI-compatible embedding server running **bge-m3**:

```sh
mkdir -p models cache
curl -L -o models/bge-m3-Q6_K.gguf \
  https://huggingface.co/gpustack/bge-m3-GGUF/resolve/main/bge-m3-Q6_K.gguf

docker run -d --name embed -p 127.0.0.1:8081:8080 -v "$PWD/models:/models:ro" \
  ghcr.io/ggml-org/llama.cpp:server -m /models/bge-m3-Q6_K.gguf \
  --embedding --pooling cls -c 8192 -np 8 -b 1024 -ub 1024 --alias bge-m3 --host 0.0.0.0 --port 8080
```

On its first start with `EMBED_URL` set, the server embeds 22,662 texts: about 40 minutes on a laptop CPU, hours on a small integrated GPU. `find` answers by keyword in the meantime. Skip the wait by downloading the vectors attached to the release you run:

```sh
gh release download v0.7.0 --repo vitoramaral10/cloudflare-mcp --pattern 'index-*.bin' --dir cache
```

Then start the server with:

```sh
-e EMBED_URL=http://embed:8080 -e CACHE_DIR=/cache -v "$PWD/cache:/cache"
```

The log line `search: semantic index loaded from cache` confirms it. The cache file name is a hash of the catalog and of `EMBED_MODEL`, so it only matches the release it came from and `EMBED_MODEL=bge-m3`. [`docker-compose.example.yml`](https://github.com/vitoramaral10/cloudflare-mcp/blob/main/docker-compose.example.yml) runs both services together.

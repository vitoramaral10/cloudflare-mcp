# Security

## Reporting a vulnerability

Please report vulnerabilities privately, through GitHub's [private vulnerability reporting](https://github.com/vitoramaral10/cloudflare-mcp/security/advisories/new), not in a public issue. Include what an attacker can do and how to reproduce it. You should get an answer within a week.

Only the latest release is supported.

## The security model

**The server acts with your API token, and so does anyone who can call it.** Every tool, including the destructive ones, runs with the permissions of `CLOUDFLARE_API_TOKEN`. Keep the endpoint private:

- bind it to localhost (`ADDR=127.0.0.1:8080`, or publish the container port on `127.0.0.1` only);
- set `MCP_BEARER`, which makes `/mcp` require `Authorization: Bearer <value>`;
- to reach it from elsewhere, put it behind a gateway or reverse proxy that authenticates clients (OAuth, API keys, mTLS).

**Give the token the least it needs.** The model can do anything the token can. Start from the *Read all resources* template, add edit permissions per product, and restrict it to the accounts and zones you mean. *All accounts* covers every account your user belongs to, including other people's.

**Destructive tools are marked, not blocked.** `DELETE` operations, `call`, `api_request` and `tunnel_route_remove` carry `destructiveHint: true`, and the server instructions tell the model to confirm before deleting. Whether a client asks you first is up to the client.

**Secrets can come back in results.** Some operations return secrets, such as `tunnels_token_get` (a connector token) or newly created API tokens. They go to the model like any other result. Exclude those tools if that matters to you.

**What the server does not do.** It keeps no logs of tool arguments or results, stores nothing besides the vector cache, and talks only to `api.cloudflare.com` and, if configured, your embedding server.

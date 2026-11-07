# Using the tools

## How operations are named

Each of the 3,777 operation tools is named after the official Cloudflare SDKs: the resource path, then the method.

| Tool | Operation |
|---|---|
| `dns_records_list` | `GET /zones/{zone_id}/dns_records` |
| `dns_records_create` | `POST /zones/{zone_id}/dns_records` |
| `zones_settings_edit` | `PATCH /zones/{zone_id}/settings/{setting_id}` |
| `tunnels_config_update` | `PUT /accounts/{account_id}/cfd_tunnel/{tunnel_id}/configurations` |
| `rulesets_zone_entrypoint_update` | `PUT /zones/{zone_id}/rulesets/phases/{ruleset_phase}/entrypoint` |

The SDKs give the account and zone variants of some resources the same base name. The description says which one a tool is: `[zone]`, `[account]` or `[user]`. `rulesets_entrypoint_get` works on an account, `rulesets_zone_entrypoint_get` on a zone. When two operations would still collide, one takes its OpenAPI `operationId`; a name over 60 characters is cut and gets a short hash.

## Arguments

- **`zone_id`** takes the zone's name (`example.com`) or its 32-character id.
- **`tunnel_id`** takes the tunnel's name or its UUID.
- **`account_id`** is optional: without it the server uses `CLOUDFLARE_ACCOUNT_ID`, or the only account the token sees. If the token sees several, the error lists them with their names.
- **Body fields** are top-level arguments next to the path and query parameters. When a body field would collide with a parameter, or the body is not an object, it goes inside `body`.
- **Query filters** that are objects become dotted keys: `name: {"exact": "app.example.com"}` is sent as `name.exact=app.example.com`.
- **Lists paginate** with `page` and `per_page`; the response carries `result_info`.

A tool returns Cloudflare's `result` and, when present, `result_info`. On failure it returns Cloudflare's error codes and messages, with a hint when the status is 401 or 403.

## find and call

`find` searches every operation for a task and returns up to 15 (default 5) with their name, method, path, description and argument schema, plus `tips` when the task maps to a mechanism worth explaining. `call` runs the one you pick:

```text
find { "query": "Email Routing catch-all forward" }
→ { "search": "hybrid",
    "operations": [ { "name": "email_routing_rules_catch_all_update", … } ],
    "tips": [ "Email Routing: the destination must be verified …" ] }

call { "name": "email_routing_rules_catch_all_update",
       "arguments": { "zone_id": "example.com", "actions": [ … ], "matchers": [ … ], "enabled": true } }
```

Write the query in English with Cloudflare's product terms, even when the user asked in another language: *"WAF custom rules zone entrypoint"* finds the right operation far more often than *"bloquear um país"*. When the query looks Portuguese, the result starts with a tip saying so. [How find works](/search) has the numbers.

`call` accepts the name with or without a gateway prefix (`cloudflare_dns_records_list` works too).

## Tunnels

`tunnel_route_add` publishes a hostname on a remotely managed tunnel in one call:

```text
tunnel_route_add { "tunnel": "home", "hostname": "app.example.com", "service": "http://app:8080" }
→ rule added; CNAME created → 9ed7….cfargotunnel.com
```

It reads the tunnel's configuration, puts the rule before the catch-all (or replaces the rule with the same hostname and path), writes the configuration back keeping everything else, and with `create_dns` (default true) creates a proxied CNAME in the zone that covers the hostname. It refuses when the last rule is not a catch-all. If another DNS record already holds the hostname, the rule is still written and the result says the DNS step failed and why. `tunnel_route_remove` removes the rule but leaves the DNS record.

## A starter tool set

Showing 3,782 tools to a model costs context on every turn and makes the choice harder. Expose the ones you use daily plus `find` and `call`; the rest stays reachable. A set of about 55 that covers most day-to-day work:

```text
find  call  tunnel_route_add  tunnel_route_remove  api_request
accounts_list  zones_list  zones_get  zones_settings_*
dns_records_*  tunnels_*
zero_trust_access_applications_{list,get,create,update,delete}  zero_trust_access_policies_*
cache_purge  rulesets_zone_{list,get}  rulesets_zone_entrypoint_{get,update}  workers_routes_*
```

How you apply it depends on the client: an allow-list of tool names, or an MCP gateway that filters by pattern (for example rules like `include dns_records_*` followed by `exclude *`).

## Destructive operations

Every `DELETE` operation, `call`, `api_request` and `tunnel_route_remove` carry `destructiveHint: true`; `GET` operations carry `readOnlyHint: true`. Clients that honour the hints ask before running destructive tools. The token is still the real limit: give it only the permissions you are willing to have used.

# Tools

The server lists 3,782 tools: 3,777 generated from Cloudflare's OpenAPI schema and the five below. The [catalog](/catalog) lists every operation by area.

## Server instructions

Clients that show MCP server instructions to the model get this text on `initialize`:

> Cloudflare API v4 tools: one per operation of the official specification, named after the SDK (dns_records_list, tunnels_config_update, zero_trust_access_applications_create…).
> - zone_id accepts the zone name (e.g. "example.com") or the id; tunnel_id and tunnel accept a name or an id; account_id is optional (uses the default account).
> - If the right tool is not in view, use find (search by meaning across all operations) and then call with the name it returns.
> - For operations with a body, the body fields go as top-level arguments, or inside "body" when the schema says so.
> - To publish a service on a tunnel use tunnel_route_add (inserts before the catch-all and can create the CNAME); to remove it, tunnel_route_remove.
> - Listings paginate with page/per_page; the response carries result_info.
> - Anything without a dedicated tool goes through api_request, with the API v4 path (e.g. "/zones/{zone}/email/routing/rules", with zone="example.com").
> - Tools marked as destructive change production: confirm with the user before deleting.

## find

Read-only. Searches all operations for the ones that fit a task.

| Argument | Type | | |
|---|---|---|---|
| `query` | string | required | The task in English, with Cloudflare terms. |
| `limit` | integer | | How many operations to return. Default 5, maximum 15. |

Returns `search` (`hybrid` or `keyword`), `operations` (each with `name`, `method`, `path`, `description` and `arguments_schema`) and, when they apply, `tips`.

## call

Destructive. Runs an operation by name.

| Argument | Type | | |
|---|---|---|---|
| `name` | string | required | Operation name, with or without the `cloudflare_` prefix a gateway may add. |
| `arguments` | object | | Arguments as described by the operation's `arguments_schema`. |

## tunnel_route_add

Publishes a hostname on a tunnel with a remote configuration.

| Argument | Type | | |
|---|---|---|---|
| `tunnel` | string | required | Tunnel name or id. |
| `hostname` | string | required | Public hostname, e.g. `app.example.com`. |
| `service` | string | required | Target: `http://app:8080`, `http://localhost:3000`, `ssh://localhost:22`, `http_status:404`… |
| `path` | string | | Path regex for the rule. |
| `origin_request` | object | | The rule's `originRequest`, e.g. `{"noTLSVerify": true}`. |
| `create_dns` | boolean | | Create the proxied CNAME to `<tunnel id>.cfargotunnel.com`. Default `true`. |
| `account_id` | string | | Defaults to the server's account. |

## tunnel_route_remove

Destructive. Removes a hostname's ingress rule; the DNS record stays (delete it with `dns_records_delete`).

| Argument | Type | | |
|---|---|---|---|
| `tunnel` | string | required | Tunnel name or id. |
| `hostname` | string | required | Hostname of the rule. |
| `path` | string | | Path of the rule, if it has one. |
| `account_id` | string | | Defaults to the server's account. |

## api_request

Destructive. A raw API v4 call for what the operation tools cannot do: non-JSON bodies (Worker script uploads), the GraphQL analytics API (`POST /graphql`) or endpoints newer than the catalog.

| Argument | Type | | |
|---|---|---|---|
| `method` | string | required | `GET`, `POST`, `PUT`, `PATCH` or `DELETE`. |
| `path` | string | required | API v4 path without `/client/v4`, e.g. `/zones/{zone}/email/routing/rules`. |
| `query` | object | | Query parameters. |
| `body` | any | | JSON body. |
| `zone` | string | | Fills `{zone}` in the path; accepts a zone name. |
| `account_id` | string | | Fills `{account}` in the path. |
| `tunnel` | string | | Fills `{tunnel}` in the path; accepts a tunnel name. |

## Operation tools

Every other tool is one Cloudflare API operation. Its description starts with the scope (`[zone]`, `[account]`, `[user]`) followed by Cloudflare's summary and description; its input schema holds the path parameters, the query parameters and the body. `GET` operations are marked read-only, `DELETE` operations destructive. [Using the tools](/usage) explains the naming and the arguments.

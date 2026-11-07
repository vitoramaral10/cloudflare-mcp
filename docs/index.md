---
layout: home
title: cloudflare-mcp
titleTemplate: The whole Cloudflare API as MCP tools

hero:
  name: cloudflare-mcp
  text: The whole Cloudflare API as MCP tools
  tagline: Every Cloudflare API v4 operation as a typed tool, plus find and call, a search that picks the right one among 3,777. An unofficial MCP server in one Go binary.
  image:
    src: /logo.svg
    alt: A cloud outline holding API braces
  actions:
    - theme: brand
      text: Get started
      link: /getting-started
    - theme: alt
      text: Using the tools
      link: /usage
    - theme: alt
      text: View on GitHub
      link: https://github.com/vitoramaral10/cloudflare-mcp

features:
  - icon: 🧰
    title: Every operation, typed
    details: 3,777 tools generated from Cloudflare's OpenAPI schema and named like the official SDKs. Real input schemas, flattened bodies, merged oneOf variants, read-only and destructive hints.
  - icon: 🔎
    title: find + call
    details: Keep the client's tool list short and reach everything else through search. Hybrid ranking, intent guides for redirects, WAF and transform rules, and usage tips in the result.
  - icon: 🪪
    title: Names instead of IDs
    details: Pass example.com as zone_id and a tunnel's name as tunnel_id. The account defaults to the only one the token sees. Errors come back as text the model can act on.
  - icon: 🚇
    title: Tunnel routes in one call
    details: tunnel_route_add inserts the ingress rule before the catch-all, writes the config back and creates the proxied CNAME. tunnel_route_remove undoes it.
  - icon: 🇧🇷
    title: Portuguese, too
    details: Each operation carries an LLM-written description and five user-style requests in Portuguese, so find ranks Portuguese queries by meaning.
  - icon: 📦
    title: One small binary
    details: Go with the catalog embedded. Streamable HTTP, optional bearer auth, Docker images for amd64 and arm64.
---

<div class="home-section">

## How it works

<img src="/architecture.svg" alt="Architecture: an MCP client calls cloudflare-mcp, optionally through a gateway; the server calls the Cloudflare API v4 and, for find, an embedding server. At build time the OpenAPI spec and Portuguese texts become the embedded catalog.">

## Search quality

Share of requests whose operation `find` puts in the top 5, on request sets written before the changes they measure. [How find works](/search) explains the method.

<img src="/search-quality.svg" alt="Bar chart of find accuracy in the top 5 for English queries, Portuguese requests rewritten in English, and raw Portuguese requests.">

::: info Unofficial
This is a community project, not affiliated with or supported by Cloudflare, Inc. Cloudflare's own server is [cloudflare/mcp-server-cloudflare](https://github.com/cloudflare/mcp-server-cloudflare).
:::

</div>

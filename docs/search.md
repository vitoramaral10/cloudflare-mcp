# How find works

`find` answers one question: of 3,777 operations, which few fit this task? It has to work for a model that writes *"WAF custom rules zone entrypoint"* and for a request typed as *"não deixa a china acessar meu site"*.

## What is indexed

Each operation is indexed as several texts, each with its own vector:

- **Its own text**: the tool name in words, an LLM-written Portuguese description of the task, the OpenAPI tags, the words of the path, up to 120 enum values found in the body, parameters and responses (zone setting names such as `min_tls_version` only appear there), and Cloudflare's description.
- **Five requests**: how a person would ask for the operation in Portuguese, two informal, two direct and one with the Cloudflare term, also written by an LLM.

That is 22,662 vectors from bge-m3. An operation's semantic score is the score of its best-matching text, so a query only has to resemble one way of asking.

## Ranking

A query is ranked twice:

1. **By meaning**: the query's embedding against every vector, keeping each operation's best.
2. **By keyword**: each query word scores 3 if it appears in the operation's name, 1 if only in its text, half that for a prefix match of four or more letters. Plurals are folded and common Portuguese and English words ignored. Ties favour shorter names, which are usually the base operation.

The two rankings are fused with reciprocal rank fusion (`k = 10`, top 50 of each). Without an embedding server, the keyword ranking is used alone and the result says `"search": "keyword"`.

## Intent guides

Some tasks are done with a generic mechanism whose description never mentions the task: a redirect is a ruleset rule in phase `http_request_dynamic_redirect`; blocking a country is a WAF custom rule; site traffic lives in the GraphQL API. Seventeen guides recognise these intents from combinations of words (*block* + *country*, *rate* + *limit*, *purge* + *cache*…) and put up to three of their operations first, one guide at a time, with a tip on how to use them:

```text
find { "query": "WAF custom rule block country" }
→ operations: rulesets_zone_entrypoint_update, rulesets_zone_rules_create, rulesets_zone_entrypoint_get, …
  tips: [ "Blocking/challenging traffic: WAF custom rule in the zone's \"http_request_firewall_custom\"
           phase … action \"block\" or \"managed_challenge\" …" ]
```

Guides fire only on combinations, not on single generic words, so *"Access application protect subdomain login"* gets the Access guide and not the WAF or DNS ones. A unit test (`TestGuideTriggers`) pins this down.

## Portuguese

When a query looks Portuguese (accents, or words that don't exist in English), two things happen:

- a small Portuguese → English glossary adds Cloudflare terms to the keyword side (*bloquear* → *block*, *túnel* → *tunnel*);
- the result starts with a tip asking the caller to repeat the search in English with Cloudflare's terms.

The tip is there because of what the numbers below show: matching a raw request against operation texts has a ceiling. Some requests don't contain anything an operation says ("let only the office IP into the panel" is an IP access rule), and the right operation isn't even among the first 30 results for about one request in seven. A model that knows Cloudflare rewrites those requests well, and it is already the one calling `find`.

## Measured

Request sets with known answers live in `internal/cloudflare/testdata`. The holdout sets were written before the changes they measure and never used to tune the glossary or the guides; the rewritten sets are the same requests rewritten in English by Claude Haiku. Share of requests whose operation is among the results, v0.7.0, hybrid search:

| Set | Requests | Top 5 | Top 10 |
|---|---:|---:|---:|
| English, Cloudflare terms (`eval_holdout_en`) | 25 | **100%** | 100% |
| Portuguese set 1, rewritten | 25 | **96%** | 96% |
| Portuguese set 2, rewritten | 30 | **97%** | 97% |
| Portuguese set 3, rewritten | 30 | **93%** | 97% |
| Portuguese set 1, raw | 25 | 76% | 84% |
| Portuguese set 2, raw | 30 | 80% | 90% |
| Portuguese set 3, raw (informal) | 30 | 63% | 80% |
| Tuning set (optimistic) | 47 | 100% | 100% |

What each part contributes: on the rewritten sets, the guides add 13 to 17 points in the top 5 (80% without them, 93–97% with them). Indexing five Portuguese requests per operation took raw Portuguese from 53–68% to 63–80% in the top 5.

## Running the evaluation

Keyword-only scores run with every `go test`. For hybrid scores, start an embedding server with bge-m3 and point the test at it and at a cache directory:

```sh
EVAL_EMBED_URL=http://127.0.0.1:8081 EVAL_CACHE_DIR=$PWD/cache EVAL_K="5 10" \
  go test -count=1 -timeout 60m -run 'TestFindEval$' -v ./internal/cloudflare
```

`EVAL_K` prints scores for several cutoffs; without it the test lists every miss. `EVAL_FILES` points it at other request sets. If you change the search, report before and after on the holdout sets, and write a new holdout set before you look at the misses of an existing one.

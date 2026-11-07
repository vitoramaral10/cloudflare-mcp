# Contributing

Issues and pull requests are welcome. For a change bigger than a bug fix, open an issue first so we can agree on the approach.

## Setup

You need Go 1.27 or newer. The catalog is committed, so a plain build works offline:

```sh
go build ./...
go test ./...
```

To run the server:

```sh
CLOUDFLARE_API_TOKEN=... ADDR=127.0.0.1:8080 go run ./cmd/cloudflare-mcp
```

A read-only token (the *Read all resources* template) is enough to try every `GET` operation without risk.

## Before a pull request

CI runs these and builds the Docker image:

```sh
gofmt -l .        # must print nothing
go vet ./...
go test ./...
```

## Changing the search

`find` is measured, so a change to the glossary, the guides, the ranking or the indexed text should come with numbers. The request sets live in `internal/cloudflare/testdata`:

| File | |
|---|---|
| `eval_tuning.json` | Used to tune the glossary and guides. Its score is optimistic. |
| `eval_holdout_en.json` | Requests as a model writes them: English, Cloudflare terms. |
| `eval_holdout_pt1.json` … `pt3.json` | Raw Portuguese requests, written before the changes they measure. |

Keyword-only scores run with `go test`. For hybrid scores, start an embedding server with bge-m3 and point the test at it and at a cache directory (the first run computes the vectors):

```sh
EVAL_EMBED_URL=http://127.0.0.1:8081 EVAL_CACHE_DIR=$PWD/cache EVAL_K="5 10" \
  go test -count=1 -timeout 60m -run 'TestFindEval$' -v ./internal/cloudflare
```

Please don't tune against the holdout sets. If you need to look at their misses, add a new holdout set written before your change and report on it.

## Updating the catalog

See [Running in production → Updating the catalog](https://vitoramaral10.github.io/cloudflare-mcp/operations#updating-the-catalog). Commit the regenerated `catalog.json.gz`, the Portuguese text files and `docs/catalog.md` together.

## Style

- Standard Go formatting and idioms; keep functions small and the code close to what it does.
- Tool descriptions are read by models: say what the tool does, what the arguments accept, and what to do when it fails.
- Comments say *why*, not *what*.

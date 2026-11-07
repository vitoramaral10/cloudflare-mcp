## What and why

<!-- What changes, and the problem it solves. Link the issue if there is one. -->

## Checks

- [ ] `gofmt -l .` prints nothing, `go vet ./...` and `go test ./...` pass
- [ ] Changes to the search come with scores (`TestFindEval`) before and after, holdout sets untouched
- [ ] A regenerated catalog is committed with the Portuguese texts and `docs/catalog.md`
- [ ] Docs and `CHANGELOG.md` updated

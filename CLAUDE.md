# Vibranium Exchange

Go (Gofr) microservices monorepo. Work follows spec-driven development: specs and plans live in `openspec/`.

PDRs (Product Development Requests) live in `docs/PDR/`, one file per product. They are the source of product requirements: read the relevant one before proposing or designing a change.

## Rules

1. **Always TDD.** No production code without a failing test first: red → green → refactor. Run the test and confirm it fails before implementing.
2. **Languages.** Code, identifiers, comments and commits in English. OpenSpec artifacts in Spanish, keeping technical terms in English.
3. **Layers.** Follow the structure and dependencies in `architecture.md`. If something does not fit a layer, stop and ask; do not invent new layers.
4. **Scope.** If a change alters behaviour and is not covered by an OpenSpec change, propose the change before writing code.
5. **Readability.** Cyclomatic complexity < 10 per function and files of at most 300 lines. Prefer several small files and functions over one large one.
6. **Before finishing.** The module's tests pass, statement coverage is at least **85% per package** with logic (`handler`, `service`, `store`, `shared/*`; `main`, `migrations` and `models` are excluded), every package has a package comment, and `gofmt` and `gocyclo` report nothing.

## Standards

@.claude/standards/architecture.md
@.claude/standards/go.md

## Commands

```bash
go build -o bin/ ./microservices/...   # build every service into bin/ (ignored by git)
go test ./microservices/<service>/...   # tests for one service
go test ./shared/...                    # tests for shared packages
gofmt -l .                              # unformatted Go files (must be empty)
gocyclo -over 9 .                       # functions with complexity >= 10 (must be empty)
go list -f '{{if not .Doc}}{{.ImportPath}}{{end}}' ./... | grep .   # packages without a package comment (must be empty)
```

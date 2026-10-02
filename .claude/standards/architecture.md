# Architecture

## Monorepo

```text
go.mod / go.sum          the only Go module of the repo (github.com/rdzpedraos/order-book)
shared/                  shared, stable libraries
microservices/<name>/    one microservice each (main.go + layer packages)
deploy/                  docker-compose, Helm
docs/                    api.md and documentation
tools/                   tooling (loadgen, etc.)
```

- One Go module at the root, with a single `go.mod` and `go.sum`. No `go.work`, no per-service `go.mod`, no `replace`.
- Import paths are the full path from the module root: `github.com/rdzpedraos/order-book/shared/money`, `github.com/rdzpedraos/order-book/microservices/order-service/service`.
- A service never imports another service's packages. This is a structure rule, not enforced by the compiler.
- `go build ./...`, `go test ./...` and `go mod tidy` run from the root.
- A service image copies `go.mod`, `go.sum`, `shared/` and `microservices/<x>/`, and runs `go build ./microservices/<x>`, so one service is built and deployed without the others.

## shared/

Packages imported by several services. Code goes here only if it meets **both** conditions:

- Two or more services use it.
- It is stable: once defined it rarely changes (money, identity, event contracts).

Rules:

- No service business logic and no access to a service's database.
- Never imports anything from `microservices/`.
- Framework-agnostic: never imports Gofr or any other framework. Only the standard library (`net/http`, `context`, `errors`, ...) and, if justified, small stable libraries. HTTP helpers are plain `func(http.Handler) http.Handler` middlewares that services register with `app.UseMiddleware`.
- Returns sentinel errors, never HTTP responses. Each service's `handler` maps them to status and code with `shared/apierror`, which also writes the error of a middleware such as `identity`, so every error follows the format in `docs/api.md`.
- Changes are backward compatible (add, don't break). A breaking change requires an OpenSpec change.
- When in doubt, the code goes in the service. Promote it to `shared/` when a second service needs it.

`shared/` holds **contracts between services**, not shared domain code. It is not split per service.

| Goes in `shared/` | Does not go in `shared/` (lives in `microservices/<x>/`) |
| --- | --- |
| Value types whose format must match across services (`money`, `books`) | Domain models (`Order`, `Settlement`, `Level`): each service has its own in `models/` |
| Message contracts that a producer and its consumers must agree on (`events`) | Business rules, use cases, validations specific to one service |
| Cross-cutting HTTP conventions every service applies the same way (`identity`, `apierror`) | DB access, SQL, migrations |
| | DB connections and mocks: Gofr already provides `ctx.SQL` and `container.NewMockContainer` |

## Service layout

```text
microservices/order-service/
  main.go           wiring: gofr.New(), migrations, routes. No logic.
  migrations/       Gofr migrations
  handler/          entry points (HTTP routes, Kafka subscribers, gRPC), one file each:
                    parse input, call service, map errors
  service/          business rules
  store/            SQL, pub/sub and other datasources
  models/           the service's domain types and errors
  utils/            the service's pure helpers (no state, no I/O)
```

## Dependencies

```text
handler → service → store
   └──────────┴────────┴──→ models, utils, shared
```

- Never the other way around: `store` does not import `service`, and `service` does not import `handler`.
- Each layer defines the interface it consumes (`service` defines `Store`, `handler` defines `Service`), so every layer can be tested with mocks.
- `*gofr.Context` flows through the layers (it is the Gofr idiom), but only `handler` reads the request (`Bind`, `PathParam`) and only `store` uses the datasources (`ctx.SQL`, pub/sub).
- Database access is plain Gofr: `store` writes SQL by hand with `ctx.SQL` (`ExecContext`, `QueryContext`, `Select`), migrations use Gofr's `app.Migrate`, and `store` tests use `container.NewMockContainer`. No ORM, no query generator (sqlc) and no generic repository.
- A service never imports another service. They communicate over HTTP, gRPC or events, with contracts in `shared/events`.
- `tools/` may import `shared/`; nothing imports `tools/`.

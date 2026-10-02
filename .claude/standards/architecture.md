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
- Declares its errors as `fault.Error` values (`books.ErrUnknownBook`, `money.ErrTooManyDecimals`, `identity.ErrMissingUserID`), each with its status and code, so any service that answers one over HTTP already knows how. A consumer that is not HTTP (the matching engine) uses them as any other `error` and ignores the status and code.
- Changes are backward compatible (add, don't break). A breaking change requires an OpenSpec change.
- When in doubt, the code goes in the service. Promote it to `shared/` when a second service needs it.

`shared/` holds **contracts between services**, not shared domain code. It is not split per service.

| Goes in `shared/` | Does not go in `shared/` (lives in `microservices/<x>/`) |
| --- | --- |
| Value types whose format must match across services (`money`, `books`) | Domain models (`Order`, `Settlement`, `Level`): each service has its own in `models/` |
| Message contracts that a producer and its consumers must agree on (`events`) | Business rules, use cases, validations specific to one service |
| Cross-cutting HTTP conventions every service applies the same way (`identity`, `fault`) | DB access, SQL, migrations |
| The log: one envelope and the catalog of topics, types and payloads (`eventlog/events`), generic publishing (`eventlog/producer`) and reading (`eventlog/consumer`) | |
| | DB connections and mocks: Gofr already provides `ctx.SQL` and `container.NewMockContainer` |

## Service layout

```text
microservices/order-service/
  main.go           wiring: gofr.New(), migrations, routes. No logic.
  migrations/       Gofr migrations
  handlers/         one package per entry point (HTTP route, Kafka subscriber, gRPC method),
    create-order/   in a kebab-case directory with a package name without dashes (createorder):
    list-orders/    parses the input, applies the rules of that use case, calls the store
                    and returns the result or the error
  store/            SQL, pub/sub and other datasources
  models/           the service's domain types, their JSON and their errors
  utils/            the service's pure helpers used by several handlers (no state, no I/O)
```

There is no separate business-rules layer: a use case's rules live in its handler package, next to the input they validate. A rule needed by several handlers goes to `models` (if it is about a domain type) or `utils` (if it is a pure helper).

## Dependencies

```text
handlers/<x> → store
      └─────────┴──→ models, utils, shared
```

- Never the other way around, and a handler package never imports another handler package.
- Handlers call the `store` package functions directly (`store.InsertOrder(ctx, order)`), with no interfaces or constructors. `store` keeps the database behind a package-level variable, and its tests and the handler tests replace it with `store.InitMock(t)`: an in-memory database that records what was written (`mock.Orders`), can be seeded with existing rows, and fails every call when `mock.Err` is set.
- `*gofr.Context` flows to the store (it is the Gofr idiom); only handlers read the request (`Bind`, `PathParam`) and only `store` uses the datasources (`ctx.SQL`, pub/sub).
- Database access is plain Gofr: `store` writes SQL by hand with `ctx.SQL` (`ExecContext`, `QueryContext`, `Select`), migrations use Gofr's `app.Migrate`, and the SQL itself is tested against PostgreSQL with the `integration` build tag; unit tests use Gofr's SQL mock with the same query constants (see `go.md`). No ORM, no query generator (sqlc) and no generic repository.
- A service never imports another service. They communicate over HTTP, gRPC or events, with contracts in `shared/events`.
- `tools/` may import `shared/`; nothing imports `tools/`.

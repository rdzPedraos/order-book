# Architecture

## Monorepo

```text
go.work
modules/            shared, stable libraries (one Go module)
services/<name>/    one Go module per microservice
deploy/             docker-compose, Helm
docs/               api.md and documentation
tools/              tooling (loadgen, etc.)
```

## modules/

Packages imported by several services. Code goes here only if it meets **both** conditions:

- Two or more services use it.
- It is stable: once defined it rarely changes (money, identity, event contracts).

Rules:

- No service business logic and no access to a service's database.
- Never imports anything from `services/`.
- Changes are backward compatible (add, don't break). A breaking change requires an OpenSpec change.
- When in doubt, the code goes in the service's `internal/`. Promote it to `modules/` when a second service needs it.

## Service layout

```text
services/order-service/
  main.go           wiring: gofr.New(), migrations, routes. No logic.
  migrations/       Gofr migrations
  internal/
    handler/        entry points (HTTP routes, Kafka subscribers, gRPC), one file each:
                    parse input, call service, map errors
    service/        business rules
    store/          SQL, pub/sub and other datasources
    models/         the service's domain types and errors
    utils/          the service's pure helpers (no state, no I/O)
```

## Dependencies

```text
handler → service → store
   └──────────┴────────┴──→ models, utils, modules
```

- Never the other way around: `store` does not import `service`, and `service` does not import `handler`.
- Each layer defines the interface it consumes (`service` defines `Store`, `handler` defines `Service`), so every layer can be tested with mocks.
- `*gofr.Context` flows through the layers (it is the Gofr idiom), but only `handler` reads the request (`Bind`, `PathParam`) and only `store` uses the datasources (`ctx.SQL`, pub/sub).
- A service never imports another service. They communicate over HTTP, gRPC or events, with contracts in `modules/events`.
- `tools/` may import `modules/`; nothing imports `tools/`.

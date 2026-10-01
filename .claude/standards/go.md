# Go

## Code

- `gofmt` is mandatory (a hook applies it on every edit).
- Stdlib and Gofr first. A new dependency is justified as a decision in `design.md`.
- Errors: wrap with `fmt.Errorf("...: %w", err)`. Domain errors are sentinels in `models`. Only `handler` maps them to a response (HTTP status in the `docs/api.md` format).
- Money is always `int64` via `modules/money`, never `float64`.
- No mutable globals and no `panic` in a request path. Configuration is read from env through Gofr's config.

## Readability

Code is optimized to be easy to read, not short or clever.

- **Cyclomatic complexity < 10** per function (`gocyclo -over 9`). If it goes over, extract functions or use early returns; don't nest.
- **Files of at most 300 lines** (`_test.go` files don't count). When a file grows, split it by responsibility: one file per resource or use case (`create_order.go`, `cancel_order.go`), not an `orders.go` with everything.
- A function does one thing, and its name says what.
- Early returns instead of chained `else`.
- Explicit names (`reservedAmount`, not `ra`). A comment explains *why*, not *what*.

## Tests

- The `_test.go` file sits next to the code, in the same package.
- Table-driven with `t.Run`. The case name describes the spec scenario (`"limit order is created"`).
- `handler`: `httptest` for HTTP routes, a Gofr test context for subscribers, with `service` mocked. `service`: `store` mocked. `store`: Gofr's mock container (`container.NewMockContainer`).
- Integration tests (real database, Kafka) use the `integration` build tag.
- No `time.Sleep` and no dependence on execution order.

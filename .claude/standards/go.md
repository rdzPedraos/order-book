# Go

## Code

- `gofmt` is mandatory (a hook applies it on every edit).
- Stdlib and Gofr first. A new dependency is justified as a decision in `design.md`.
- Errors: wrap with `fmt.Errorf("...: %w", err)`, also to answer with a `fault.Error` caused by another error (`fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err)`): `fault.From` finds the `fault.Error` in the chain and the client only sees it. A service's domain errors are `fault.Error` values in `models` (status, stable code, message), so the error a client receives is declared next to the rule that produces it. The errors of `shared/` packages are `fault.Error` values too. Codes common to every endpoint (`invalid_body`, `service_unavailable`, `internal_error`) come from `shared/fault`. Every `handler` returns `fault.From(err)`: Gofr only renders a `fault.Error` returned as is, and anything that is not one becomes `internal_error` without revealing its message.
- Money is always `int64` in the currency's minimal unit via `shared/money`, never `float64`. The number of decimals comes from the per-currency registry in `shared/money`, never from a hardcoded factor like `* 100`. Amounts travel as decimal strings in JSON.
- No mutable globals and no `panic` in a request path. Fixed lookup tables (the currencies, the books) are package-level `var` maps that no code writes to after initialization, not functions that rebuild them on each call. Configuration is read from env through Gofr's config.

## Readability

Code is optimized to be easy to read, not short or clever.

- **Cyclomatic complexity < 10** per function (`gocyclo -over 9`). If it goes over, extract functions or use early returns; don't nest.
- **Files of at most 300 lines** (`_test.go` files don't count). When a file grows, split it by responsibility: one file per resource or use case (`create_order.go`, `cancel_order.go`), not an `orders.go` with everything.
- A function does one thing, and its name says what.
- Early returns instead of chained `else`.
- Explicit names (`reservedAmount`, not `ra`).
- **Every package has a package comment** (`// Package x ...`, or `// Command x ...` for a `main`). It is required, not optional: it is the context of the package for whoever opens it. It says what the package is for and, when not obvious, why it exists or where it sits (for a `shared/` package, why it is shared). It goes in the file named after the package (`books.go`, `handler.go`) or, if there is none, in `doc.go`. Checked with `go list` (see `CLAUDE.md`).
- Every other comment only when the code cannot explain itself: a non-obvious rule, constraint or reason (*why*, not *what*). No comment that restates a name or summarizes a body, including doc comments on exported identifiers (Go's doc-comment convention does not apply to them) and comments on test helpers.

## Tests

- The `_test.go` file sits next to the code, in the same package, and is named after the file it mainly tests: `create_order.go` → `create_order_test.go`. Helpers and fakes shared by the whole package go in `<package>_test.go` (`handler_test.go`, `service_test.go`).
- One explicit `t.Run` per case inside the test function, each with its own inputs and assertions written out. No table of cases iterated with a `for` loop: a reader must see each case without mapping a table row back to the loop body.
- Assertions use `github.com/stretchr/testify/require` through `c := require.New(t)`, declared first in each `t.Run` (and in each helper, after `t.Helper()`): `c.Equal`, `c.NoError`, `c.ErrorIs`. A failed assertion stops that case. Not `if` plus a hand-formatted message, and not `testify/assert`, which keeps running after a failure.
- The `t.Run` name describes the spec scenario (`"limit order is created"`).
- Repeated setup or assertions go into small helpers (`t.Helper()`), not into a loop. Helpers are declared at the top of the test file, above the test functions that use them.

  ```go
  func TestNormalize(t *testing.T) {
  	t.Run("lowercase id", func(t *testing.T) {
  		c := require.New(t)

  		got, err := Normalize("brl-vib")
  		c.NoError(err)
  		c.Equal("BRL-VIB", got.ID)
  	})

  	t.Run("tickers in another order", func(t *testing.T) {
  		c := require.New(t)

  		_, err := Normalize("VIB-BRL")
  		c.ErrorIs(err, ErrUnknownBook)
  	})
  }
  ```
- `handler`: `httptest` for HTTP routes, a Gofr test context for subscribers, with `service` mocked. `service`: `store` mocked. `store`: Gofr's mock container (`container.NewMockContainer`).
- Coverage: at least 85% of statements per package with logic (`handler`, `service`, `store`, `shared/*`), measured with the unit tests (`go test -cover`). `main`, `migrations` and `models` hold wiring, schema and types and are excluded. A logic package without any test counts as 0%, not as excluded.
- Integration tests (real database, Kafka) use the `integration` build tag.
- No `time.Sleep` and no dependence on execution order.

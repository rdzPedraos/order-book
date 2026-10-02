# Handlers

A handler is one entry point of a service (an HTTP route, a subscriber): one package in `microservices/<service>/handlers/<kebab-case>/`, with the package name without dashes (`handlers/list-orders` → `package listorders`). It holds everything that use case needs: its input, its rules, its calls to the store and its output. `handlers/modify-order` (body, path and an owned resource), `handlers/list-orders` (query parameters and a page) and `handlers/get-order` are the reference implementations.

## Files

- `handler.go` and `handler_test.go`. A second file only when `handler.go` goes over 300 lines.
- `handler.go` starts with the package comment: the route and what it answers.

  ```go
  // Package getorder handles GET /orders/{id}: it answers one order of the
  // person behind the request.
  package getorder
  ```

## Shape of `handler.go`

Top to bottom, in this order:

1. Constants and the errors only this handler produces (see [Errors](#errors)).
2. The `request` type, and a `response` type only if the answer is not a model.
3. `Handle`.
4. The functions `Handle` calls, in the order it calls them.

`Handle` reads top-down — read the request, act on it, answer — and delegates the details:

```go
func Handle(ctx *gofr.Context) (any, error) {
	req, err := parseRequest(ctx)
	if err != nil {
		return nil, err
	}

	order, err := req.getOwnedOrder(ctx)
	if err != nil {
		return nil, fault.From(err)
	}

	book, err := books.Normalize(order.Book)
	if err != nil {
		return nil, err
	}

	if err := req.validate(book); err != nil {
		return nil, err
	}

	return nil, models.ErrNotImplemented
}
```

## The request

- `request` is the first thing `Handle` builds and everything after it works from it. It holds all the input of the call, already typed: the body fields, the path parameters, the query parameters and the user id.

  ```go
  type request struct {
  	UserID   string    `json:"-"`
  	OrderID  uuid.UUID `json:"-"`
  	Limit    *string   `json:"limit"`
  	Quantity *string   `json:"quantity"`
  }
  ```

  Body fields keep the JSON names and raw types of the API (`*string` for amounts and quantities, which travel as decimal strings). Fields that do not come from the body are tagged `json:"-"`. Query parameters and path values are converted to domain types from `models` (`*models.Side`, `*models.Status`) and parsed values (`uuid.UUID`, `*uuid.UUID`, `int`).
- One function builds it, `parseRequest` (`parseParams` when the input is only the query): `ctx.Bind` for the body, then the user with `identity.GetUserID`, then the path with `ctx.PathParam` and the query with `ctx.Param` (Gofr cannot bind query parameters). Each field is converted and checked once, with one small `parseX` function per query parameter. Nothing is parsed twice.
- A field that was not sent is a nil pointer; an empty query parameter is not a filter.
- Anything that needs the request's data is a method on it, so each call site is one line: `req.getOwnedOrder(ctx)` loads the resource the request points to, `req.validate(book)` checks the rules that need that resource, `req.listOrders(ctx)` runs the query. Their names follow the verbs of `go.md`.
- The rules of a domain value are a method of its model, `order.Validate()`, called once the model is built: a valid side, the allowed combinations of fields, values greater than zero. The handler only binds and parses the request (a value that cannot be parsed is answered with the error of its field) and builds the model.
- Rules that need data from the store (for example, the currencies of the order's book) are checked after loading it, in a `validate` method that receives what it needs.

## Return values

- A function that returns a struct and an error returns `*T`, `nil` on error, never an empty `T{}`.
- On error `Handle` returns a literal `nil` as data: Gofr answers `206 Partial Content` when it gets data and an error together.
- The answer is the model itself when the model's JSON is the API's (`models.Order` has `MarshalJSON`). A list with metadata is a `response.Response{Data, Metadata}` from `gofr.dev/pkg/gofr/http/response`, which Gofr renders as `{"data": ..., "metadata": ...}`. `Handle` returns it as a value (`return *page, nil`): Gofr only recognizes the value, and a `*response.Response` is wrapped again in `{"data": ...}`. A custom struct would be wrapped again in `{"data": ...}`.

## Errors

Every error is a `fault.Error` (status, stable code, message). Where it is declared depends on who uses it:

| Error | Lives in | Example |
| --- | --- | --- |
| Shared by several handlers or returned by the store | `models/errors.go` | `models.ErrOrderNotFound`, `models.ErrInvalidSide` |
| Produced by one handler only | a `var` at the top of its `handler.go` | `errInvalidCursor` in `list-orders` |
| A convention of every service | `shared/` | `identity.ErrMissingUserID`, `books.ErrUnknownBook`, `fault.ErrInvalidBody` |

- Validation errors are returned as they are: they are already `fault.Error` values.
- A store error other than a known domain error becomes `fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err)`, so the cause stays in the chain, and `Handle` passes it through `fault.From(err)`: Gofr only renders a `fault.Error` returned as is, and `From` also turns any unexpected error into `internal_error` without revealing its message.
- A malformed id in the path is answered as the not-found error of that resource, not as a validation error.

## Store

- Handlers call the store package functions directly (`orderdb.GetOrder(ctx, userID, id)`); there is no interface or constructor to inject.
- The store filters by the user id, so someone else's order comes back as `models.ErrOrderNotFound` and its existence is not revealed.
- Tests replace the database with `orderdb.InitMock(t)` (see `go.md`).

## Comments

The package comment is required. Any other comment only states a rule or a reason the code cannot show (for example, why a malformed id is a not-found).

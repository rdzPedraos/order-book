# Event log

Services talk through the log (Redpanda, Kafka API) with `shared/eventlog`: the contract in `events`, publishing in `producer`, reading in `consumer`. A service never calls Kafka directly.

## Messages

- Every message is an `events.Message`: `id` (UUIDv7), `route`, `book`, `schemaVersion`, `createdAt` and a `payload`. Build it with `events.NewMessage(route, book, createdAt, payload)`; a handler reads the payload with `message.ParsePayload(&payload)`.
- A message is named only by its **route**, `<topic>.<type>` (`orders.commands.NewOrder`): it is published to the topic and a handler subscribes to the route. The catalog of `events` declares one `Route…` constant per type and one payload struct per type, in a file per topic family (`orders.go`). Adding a message is adding both there.
- Payloads carry the ids they need (`orderId`, `userId`). Amounts and quantities are `*int64` in minimal units, tagged `json:",string"`, so they travel as decimal strings.
- Changes to a payload are backward compatible (add, don't break), like the rest of `shared/`.

## Routes

| Route | Published by | What it says |
| --- | --- | --- |
| `orders.commands.NewOrder`, `ModifyOrder`, `CancelOrder` | OrderService (`api`) | A person asked to create, change or cancel an order |
| `orders.events.OrderAccepted` | MatchingEngine | The order's funds were frozen; a limit order rests in the book |
| `orders.events.OrderRejected` | MatchingEngine | `reason`: `insufficient_funds` or `invalid_amount` |
| `orders.events.OrderCancelled` | MatchingEngine | `cancelledQuantity` and `released`; `reason` is `no_liquidity` for a market order's remainder, empty when the person cancelled |
| `orders.events.OrderModified` | MatchingEngine | The new `limit` and `quantity` |

The engine publishes only what changes an order: a cancellation or modification that cannot apply (unknown order, someone else's, already final, or short of funds) changes nothing, so it is logged and no event is published.

Every event payload carries an `EventHeader`: the command's `sequence` in its book, the event's `index` among that command's events, the command's `commandId` and `commandOffset`, and the `orderId` and `userId`. An event's `id` is fixed by its command's `id` and its index (`events.NewEventMessage`), which the log keeps unchanged, so a retried batch or a restart publishes the same ids, and the engine resumes after a restart from the last event's `commandOffset` and `index`.

## Publishing

`producer.Publish(ctx, message)` returns only once every in-sync replica has the message (`acks=all`), and gives up after 5 seconds. It goes to the topic of its route, in the partition of its `book` (from `shared/books`), keyed by the book id, so a book's messages are read in one total order. A handler answers a publish error as `fault.ErrServiceUnavailable` (`503`).

`producer.Connect` runs once in `main`; handler tests replace the log with `producer.InitMock(t)` and check `mock.Messages`.

## Consuming

```go
consumer.Start(ctx, brokers, group, ctx.Logger,
	consumer.Subscribe(events.RouteNewOrder, insertneworder.Handle),
)
```

- One `Start` per service role, in `app.OnStart`, with one `Subscribe` per route, like HTTP routes. Each route has its own handler package (`handlers/insert-new-order`) with `func Handle(ctx *gofr.Context, message events.Message) error`.
- Delivery is at least once: a message is committed only after its handler returns `nil`, and an error retries the same message, so return an error only when retrying can succeed (a store failure). A payload that can never be applied is logged and skipped (`return nil`).
- Handlers are idempotent, because a message can arrive twice: write with `ON CONFLICT … DO NOTHING` or check the message `id`.
- A route nobody subscribed to is committed without being handled. There is no order across topics: a handler that needs something another topic creates returns an error until it exists.

## Reading a partition

```go
consumer.StartPartition(ctx, brokers, events.TopicOrderCommands, book.Partition, maxBatch, ctx.Logger, applycommands.Handle)
```

The matching engine is the one exception to `Start`. It reads the partition of its book from the first offset on every start, without a consumer group, and gets batches of what is available, up to `maxBatch`: `func Handle(ctx *gofr.Context, records []consumer.Record) error`.

- **Why:** the book lives in memory, so it is rebuilt by rereading every command, and a group would resume from its last commit and could hand the partition to another pod. A batch lets the engine reserve the funds of all its new orders in one call to WalletService before applying its commands in log order.
- **One handler for every route of the partition** (`handlers/apply-commands`), because the commands of a batch are applied together and in order; it chooses what to do by the message route.
- Nothing is committed. A failed batch is retried until it succeeds, and the engine learns how far it already published from the last event, with `consumer.GetLastMessage`.

## Tests

`producer` and `consumer` are unit tests against `kfake` (`github.com/twmb/franz-go/pkg/kfake`), the in-memory Kafka of franz-go: one cluster per package in `TestMain`, and each test creates its own topic. It speaks the real protocol (acks, idempotent producer, consumer groups and commits) without Docker, so the guarantees above are tested in CI without a broker. Handlers do not touch Kafka: they use `producer.InitMock(t)`.

# Benchmark

## Matching engine

How many commands per second the engine applies to one book, measured with `go test -bench` on the batch handler. The wallet and the event log are the in-memory mocks, so this is the engine's own work: applying each command to the book, crossing orders, and building the events and fund operations of each batch. It does not include the calls to WalletService or Redpanda.

```bash
go test -run '^$' -bench . -benchtime 20x ./microservices/matching-engine/handlers/apply-commands/
```

Each run applies 10,000 random commands (fixed seed, 20 people, prices around R$ 100.00) to an empty book, in batches of 500 like the engine.

| Profile | Commands | commands/s | ns per command | allocs per command |
| --- | --- | --- | --- | --- |
| A, everything crosses | Limit and market orders at one price | 134,785 | 7,419 | 56 |
| B, 90 % rests | Buys below and sells above the mid price; 10 % market orders | 158,159 | 6,323 | 45 |
| C, 50 % cancellations | Half the commands cancel an earlier order | 210,126 | 4,759 | 34 |

Target: at least 25,000 commands/s on one core, five times the 5,000 operations/s of the PDR. All three profiles meet it.

Measured on 2026-10-02 with Go 1.26.0 on a 12th Gen Intel Core i5-1235U.

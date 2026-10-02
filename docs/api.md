# Order Book API

REST API to trade on the order book: hold money in a wallet, create and query buy and sell orders, and see the depth of the market.

## Base URL

All endpoints live under a single base URL. The examples use `$API` for it:

```bash
export API=https://<api-host>
```

## Endpoints

| Method | URL | What it does |
| --- | --- | --- |
| `POST` | `$API/orders` | [Create an order](#create-an-order) |
| `GET` | `$API/orders` | [List your orders](#list-your-orders) |
| `GET` | `$API/orders/{orderId}` | [Get one of your orders](#get-one-of-your-orders) |
| `POST` | `$API/orders/{orderId}/change` | [Request a change to an order](#request-a-change-to-an-order) |
| `POST` | `$API/orders/{orderId}/close` | [Request the cancellation of an order](#request-the-cancellation-of-an-order) |
| `GET` | `$API/wallet` | [Get your wallet](#get-your-wallet) |
| `POST` | `$API/wallet/deposits` | [Deposit](#deposit) |
| `POST` | `$API/wallet/withdrawals` | [Withdraw](#withdraw) |
| `GET` | `$API/wallet/movements` | [List your movements](#list-your-movements) |
| `GET` | `$API/market/orderbook/{book}` | [Get the depth of a book](#get-the-depth-of-a-book) |

## Conventions

### Identity

Every endpoint except the market's requires the `X-User-ID` header, which identifies the person. You only see and operate your own orders and wallet: someone else's order answers `404`, exactly like one that does not exist. Any `X-User-ID` has a wallet: one that never operated holds zero.

### Requests

Bodies are JSON and must be sent with `Content-Type: application/json`.

### Money and quantities

- **Amounts, prices and quantities** (`limit`, `amount`, `avgPrice`, `quantity`, `filledQuantity`, `pendingQuantity`) are **decimal strings** in their currency's scale: `"90.00"` BRL, `"10"` VIB. They are never JSON numbers, so no client reads them as floating point.
- More decimals than the currency allows, negative values or values out of range are rejected with `400`.

| Currency | Decimals | Example |
| --- | --- | --- |
| BRL | 2 | `"90.00"` |
| VIB | 0 | `"10"` |

### Books

A book is a market where one asset is traded for another:

- the **quote** is what you pay or receive; `limit`, `amount` and prices are always in the quote;
- the **base** is what you buy or sell; `quantity` is always in the base.

A price is how much quote one unit of base costs: `"90.00"` in `BRL-VIB` means R$ 90.00 per VIB.

| Book | Quote (what you pay) | Base (what you trade) | Example |
| --- | --- | --- | --- |
| `BRL-VIB` | BRL, 2 decimals | VIB, whole units | `"limit":"90.00","quantity":"10"` buys 10 VIB at up to R$ 90.00 each |

Send the book id exactly as listed above; letter case does not matter (`brl-vib` works). Tickers in another order (`VIB-BRL`) are an unknown book.

### Reads after a write

Creating, changing or cancelling an order is accepted as soon as it is safely recorded, and the orders you read are updated right after, usually within a few milliseconds. In that window a `GET`, `change` or `close` of an order you just created can answer `404 order_not_found`: retry after a moment.

What happens to an order after that comes the same way, a few milliseconds after the book applies it: its `status`, what it executed, the VIB or BRL a trade gives you in your wallet, and the depth of the market.

### Errors

Every error, on every endpoint, has this body:

```json
{ "error": { "code": "invalid_quantity", "message": "invalid quantity" } }
```

`code` is stable and meant for clients; `message` is for humans and may change. Each endpoint lists its own codes. These apply to all of them:

| Status | Code | When |
| --- | --- | --- |
| 401 | `missing_user_id` | `X-User-ID` header missing |
| 503 | `service_unavailable` | The service could not record the request in time; retry later. Rarely, the request was recorded but its confirmation was lost: before creating an order again, check [your orders](#list-your-orders) |
| 500 | `internal_error` | Unexpected error |

## Orders

### The order object

Every orders endpoint returns orders in this shape:

```json
{
  "orderId": "01a0fa6b-28ea-7bbb-9edb-4ad34c008538",
  "book": "BRL-VIB",
  "side": "BUY",
  "type": "LIMIT",
  "limit": "90.00",
  "amount": null,
  "quantity": "10",
  "filledQuantity": "0",
  "pendingQuantity": "10",
  "avgPrice": null,
  "status": "PENDING",
  "createdAt": "2026-10-02T02:22:01.962769264Z",
  "updatedAt": "2026-10-02T02:22:01.962769264Z"
}
```

| Field | Meaning |
| --- | --- |
| `orderId` | Server-generated id |
| `book` | The book id |
| `side` | `BUY` or `SELL` |
| `type` | `LIMIT` when the order has a `limit`, `MARKET` otherwise |
| `limit` | Maximum price per VIB to buy, or minimum to sell, in BRL; `null` in market orders |
| `amount` | BRL to spend in a market buy; `null` otherwise |
| `quantity` | VIB to buy or sell; `null` in a market buy |
| `filledQuantity` | VIB already executed; `"0"` while nothing is executed |
| `pendingQuantity` | `quantity − filledQuantity`; `null` when `quantity` is `null` |
| `avgPrice` | Average executed price in BRL; `null` while nothing is executed |
| `status` | See below |
| `reason` | Why the order was `REJECTED` or `CANCELLED`, when the book gave a reason; `null` otherwise |

| `status` | Meaning |
| --- | --- |
| `PENDING` | Recorded, not yet in the book. A market order stays `PENDING` until it is filled or cancelled |
| `OPEN` | A limit order resting in the book, nothing executed |
| `PARTIALLY_FILLED` | A limit order with part of its `quantity` executed; the rest still rests in the book |
| `FILLED` | All of its `quantity` executed; a market buy, all of its `amount` spent |
| `CANCELLED` | You cancelled it, or nothing was left to execute a market order against (`reason`: `no_liquidity`); what it executed before stays executed |
| `REJECTED` | Never entered the book (`reason`: `insufficient_funds` or `invalid_amount`) |

`FILLED`, `CANCELLED` and `REJECTED` are final: the order never changes again.
| `createdAt`, `updatedAt` | RFC 3339 timestamps in UTC |

### Create an order

`POST $API/orders`

Each call creates a new order: sending the same request twice creates two orders.

**Body**

| Field | Type | Meaning |
| --- | --- | --- |
| `book` | string | The book id, for example `BRL-VIB` |
| `side` | string | `BUY` or `SELL` |
| `limit` | decimal string | Maximum price per VIB when buying, minimum when selling. With it the order is a limit order; without it, a market order |
| `quantity` | decimal string | How many VIB to buy or sell, in whole units: `"10"` |
| `amount` | decimal string | How many BRL to spend, only in a market buy |

There is no `type` field: it is derived from `limit`. An order is expressed either in VIB (`quantity`) or in BRL (`amount`), never both. The allowed combinations are:

| Order | `limit` | `quantity` | `amount` | Reads as |
| --- | --- | --- | --- | --- |
| Limit buy | ✅ | ✅ | ❌ | "Buy 10 VIB paying at most R$ 90 each" |
| Limit sell | ✅ | ✅ | ❌ | "Sell 10 VIB charging at least R$ 90 each" |
| Market buy | ❌ | ❌ | ✅ | "Spend R$ 500 on VIB at the best prices available" |
| Market sell | ❌ | ✅ | ❌ | "Sell 10 VIB at the best prices available" |

**Example**

```bash
curl -X POST $API/orders \
  -H 'X-User-ID: user-a' -H 'Content-Type: application/json' \
  -d '{"book":"BRL-VIB","side":"BUY","limit":"90.00","quantity":"10"}'
```

**Response:** `201 Created`, with the created order in `data`:

```json
{
  "data": {
    "orderId": "01a0fa6b-28ea-7bbb-9edb-4ad34c008538",
    "book": "BRL-VIB",
    "side": "BUY",
    "type": "LIMIT",
    "limit": "90.00",
    "amount": null,
    "quantity": "10",
    "filledQuantity": "0",
    "pendingQuantity": "10",
    "avgPrice": null,
    "status": "PENDING",
    "createdAt": "2026-10-02T02:22:01.962769264Z",
    "updatedAt": "2026-10-02T02:22:01.962769264Z"
  }
}
```

A market buy sends `{"book":"BRL-VIB","side":"BUY","amount":"500.00"}` and gets back `type = MARKET`, `amount = "500.00"`, and `limit`, `quantity` and `pendingQuantity` as `null`.

**Errors**

| Status | Code | When |
| --- | --- | --- |
| 400 | `invalid_body` | The body is not valid JSON |
| 400 | `unknown_book` | The book is not supported |
| 400 | `invalid_side` | `side` is not `BUY` or `SELL` |
| 400 | `invalid_limit_price` | `limit` is ≤ 0 or has too many decimals |
| 400 | `invalid_quantity` | `quantity` is not an integer or is < 1 |
| 400 | `invalid_amount` | `amount` has too many decimals or is ≤ 0 |
| 400 | `unsupported_order` | The combination of `limit`, `quantity` and `amount` is not one of the [allowed combinations](#create-an-order) |

### List your orders

`GET $API/orders`

Your orders, most recent first, one page at a time.

**Query parameters** (all optional, all combinable)

| Parameter | Meaning |
| --- | --- |
| `status` | Only orders in this [status](#the-order-object) |
| `side` | Only `BUY` or only `SELL` orders |
| `book` | Only orders of this book |
| `limit` | Page size, 1 to 100; 20 by default |
| `cursor` | Continue after this `orderId`, without including it. Use the `nextCursor` of the previous page |

**Example**

```bash
curl "$API/orders?status=PENDING&side=BUY&limit=20" -H 'X-User-ID: user-a'
```

**Response:** `200 OK`, with the orders in `data` and the next page's cursor in `metadata`:

```json
{
  "data": [
    { "orderId": "01a0fa6f-6c1d-7f02-9b44-2f0d7a1e5c3b", "book": "BRL-VIB", "side": "BUY", "...": "..." },
    { "orderId": "01a0fa6f-51c5-788b-829d-c57a0db3b581", "book": "BRL-VIB", "side": "BUY", "...": "..." }
  ],
  "metadata": { "nextCursor": "01a0fa6f-51c5-788b-829d-c57a0db3b581" }
}
```

Each item is a full [order object](#the-order-object). To get the next page, repeat the same query adding `&cursor=<nextCursor>`. On the last page `nextCursor` is `null`.

**Errors**

| Status | Code | When |
| --- | --- | --- |
| 400 | `invalid_status` | Unknown `status` |
| 400 | `invalid_side` | `side` is not `BUY` or `SELL` |
| 400 | `unknown_book` | The book is not supported |
| 400 | `invalid_limit` | `limit` is not an integer between 1 and 100 |
| 400 | `invalid_cursor` | `cursor` is not an order id |

### Get one of your orders

`GET $API/orders/{orderId}`

**Example**

```bash
curl $API/orders/01a0fa6b-28ea-7bbb-9edb-4ad34c008538 -H 'X-User-ID: user-a'
```

**Response:** `200 OK`, with the [order object](#the-order-object) in `data`:

```json
{ "data": { "orderId": "01a0fa6b-28ea-7bbb-9edb-4ad34c008538", "book": "BRL-VIB", "...": "..." } }
```

**Errors**

| Status | Code | When |
| --- | --- | --- |
| 404 | `order_not_found` | The order does not exist, is someone else's, or the id is malformed |

### Request a change to an order

`POST $API/orders/{orderId}/change`

Asks to change the `limit` and/or the pending `quantity` of one of your limit orders. The change is applied asynchronously: the response is `201` with the order **as it is now, unchanged**, and the order shows the new values once the change is applied. If the order was filled or cancelled first, the change has no effect.

**Body:** `limit` and/or `quantity`, validated with the same rules as [Create an order](#create-an-order).

```bash
curl -X POST $API/orders/01a0fa6b-28ea-7bbb-9edb-4ad34c008538/change \
  -H 'X-User-ID: user-a' -H 'Content-Type: application/json' -d '{"limit":"92.00"}'
```

**Response:** `201 Created`, with the [order object](#the-order-object), still with its previous values, in `data`.

**Errors**, checked in this order:

| Status | Code | When |
| --- | --- | --- |
| 400 | `invalid_body` | The body is not valid JSON |
| 404 | `order_not_found` | The order does not exist, is someone else's, or the id is malformed |
| 409 | `order_not_modifiable` | The order is a market order; only limit orders can be changed |
| 400 | `invalid_body` | The body has neither `limit` nor `quantity` |
| 400 | `invalid_limit_price` | `limit` is ≤ 0 or has too many decimals |
| 400 | `invalid_quantity` | `quantity` is not an integer or is < 1 |

### Request the cancellation of an order

`POST $API/orders/{orderId}/close`

Asks to cancel one of your orders. The cancellation is applied asynchronously, because it can race with an execution of the same order: the response is `201` with the order **as it is now**, and the order shows the result once it is applied. The result is one of:

- **cancelled**: nothing had been executed;
- **cancelled in part**: what was already executed stays executed, and the rest is cancelled;
- **no effect**: the order was already filled or cancelled, and stays as it was.

Sending the same request twice is safe: the second one has no effect.

```bash
curl -X POST $API/orders/01a0fa6b-28ea-7bbb-9edb-4ad34c008538/close -H 'X-User-ID: user-a'
```

**Response:** `201 Created`, with the [order object](#the-order-object), still `PENDING`, in `data`.

**Errors**

| Status | Code | When |
| --- | --- | --- |
| 404 | `order_not_found` | The order does not exist, is someone else's, or the id is malformed |

## Wallet

Your money, in BRL and VIB. An order only enters the book if what it may need can be frozen first: a limit buy freezes `quantity × limit` in BRL, a market buy its `amount`, and a sell its `quantity` in VIB. Frozen money shows as `reserved` and cannot be withdrawn or used by another order; cancelling the order gives back what it did not use. Without enough `available`, the order is rejected.

### The balance object

```json
{ "currency": "BRL", "available": "100.00", "reserved": "900.00", "total": "1000.00" }
```

| Field | Meaning |
| --- | --- |
| `available` | What you can withdraw or use in a new order |
| `reserved` | What is frozen for your open orders |
| `total` | `available + reserved` |

### Get your wallet

`GET $API/wallet`

```bash
curl $API/wallet -H 'X-User-ID: user-a'
```

**Response:** `200 OK`, with one [balance object](#the-balance-object) per currency, BRL first, in `data`:

```json
{ "data": [
  { "currency": "BRL", "available": "100.00", "reserved": "900.00", "total": "1000.00" },
  { "currency": "VIB", "available": "0", "reserved": "0", "total": "0" }
] }
```

### Deposit

`POST $API/wallet/deposits`

Adds money to your `available` balance. Deposits are simulated; sending the same request twice deposits twice.

| Field | Type | Meaning |
| --- | --- | --- |
| `currency` | string | `BRL` or `VIB` |
| `amount` | decimal string | How much, in the currency's scale: `"150.00"` BRL, `"10"` VIB |

```bash
curl -X POST $API/wallet/deposits \
  -H 'X-User-ID: user-a' -H 'Content-Type: application/json' \
  -d '{"currency":"BRL","amount":"150.00"}'
```

**Response:** `201 Created`, with the movement in `data`:

```json
{ "data": {
  "movementId": "01a0fdb0-b4ca-766a-a93d-d5a50f96d513",
  "type": "DEPOSIT",
  "currency": "BRL",
  "amount": "150.00",
  "orderId": null,
  "createdAt": "2026-10-02T17:36:51.402421Z"
} }
```

**Errors**

| Status | Code | When |
| --- | --- | --- |
| 400 | `invalid_body` | The body is not valid JSON |
| 400 | `invalid_currency` | `currency` is not `BRL` or `VIB` |
| 400 | `invalid_amount` | `amount` is missing, ≤ 0, or has more decimals than the currency allows |

### Withdraw

`POST $API/wallet/withdrawals`

Takes BRL from your `available` balance, never from what is `reserved` for your orders. Only BRL can be withdrawn. Withdrawals are simulated; sending the same request twice withdraws twice.

The body and the response are the same as a [deposit](#deposit), with `"type": "WITHDRAWAL"`.

**Errors**: those of a [deposit](#deposit), and:

| Status | Code | When |
| --- | --- | --- |
| 422 | `insufficient_funds` | `amount` is more than your `available` BRL; nothing is withdrawn |
| 422 | `currency_not_withdrawable` | `currency` is `VIB` |

### List your movements

`GET $API/wallet/movements`

Every deposit, withdrawal, reservation, release and trade of your wallet, newest first, one page at a time.

**Query parameters** (all optional)

| Parameter | Meaning |
| --- | --- |
| `from`, `to` | Only movements from `from` (included) to `to` (excluded), RFC 3339: `2026-10-01T00:00:00Z` |
| `limit` | Page size, 1 to 100; 20 by default |
| `cursor` | Continue after this `movementId`. Use the `nextCursor` of the previous page |

```bash
curl "$API/wallet/movements?from=2026-10-01T00:00:00Z&limit=20" -H 'X-User-ID: user-a'
```

**Response:** `200 OK`, with movements like the one of a [deposit](#deposit) in `data` and the next page's cursor in `metadata`. A reservation (`RESERVE`), a release (`RELEASE`) and each side of a trade carry their `orderId`: in a trade you have a `TRADE_PAID`, what left your `reserved` balance, and a `TRADE_RECEIVED`, what entered your `available` one in the other currency. On the last page `nextCursor` is `null`.

**Errors**

| Status | Code | When |
| --- | --- | --- |
| 400 | `invalid_date` | `from` or `to` is not an RFC 3339 timestamp |
| 400 | `invalid_limit` | `limit` is not an integer between 1 and 100 |
| 400 | `invalid_cursor` | `cursor` is not a movement id |

## Market

The public view of each book: what is offered at each price, without orders or people. It needs no `X-User-ID`.

### Get the depth of a book

`GET $API/market/orderbook/{book}`

The best buys (`bids`, highest price first) and the best sells (`asks`, lowest price first), each price with the VIB resting there and how many orders.

**Query parameters**

| Parameter | Meaning |
| --- | --- |
| `depth` | Prices per side, 1 to 100; 20 by default |

```bash
curl "$API/market/orderbook/BRL-VIB?depth=5"
```

**Response:** `200 OK`, with the book in `data`:

```json
{ "data": {
  "book": "BRL-VIB",
  "bids": [{ "price": "100.00", "volume": "30", "orders": 2 }, { "price": "99.00", "volume": "15", "orders": 1 }],
  "asks": [{ "price": "101.00", "volume": "5", "orders": 1 }]
} }
```

**Errors**

| Status | Code | When |
| --- | --- | --- |
| 404 | `book_not_found` | The book does not exist |
| 400 | `invalid_depth` | `depth` is not an integer between 1 and 100 |


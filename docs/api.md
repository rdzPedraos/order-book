# Order Book API

REST API to trade on the order book: create and query buy and sell orders.

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

## Conventions

### Identity

Every endpoint requires the `X-User-ID` header, which identifies the person. You only see and operate your own orders: someone else's order answers `404`, exactly like one that does not exist.

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
| `status` | Every new order is `PENDING` |
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
| `status` | Only orders in this status (`PENDING`) |
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

Asks to change the `limit` and/or the pending `quantity` of one of your limit orders. The change is applied asynchronously: the response is `201` with the order **as it is now, unchanged**, and the order shows the new values once the change is applied. A change can be rejected if the order was filled first.

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
- **rejected**: the order was already filled, so there is nothing to cancel.

Sending the same request twice is safe: the second one is rejected because the order is already cancelled.

```bash
curl -X POST $API/orders/01a0fa6b-28ea-7bbb-9edb-4ad34c008538/close -H 'X-User-ID: user-a'
```

**Response:** `201 Created`, with the [order object](#the-order-object), still `PENDING`, in `data`.

**Errors**

| Status | Code | When |
| --- | --- | --- |
| 404 | `order_not_found` | The order does not exist, is someone else's, or the id is malformed |

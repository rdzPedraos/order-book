# Proposal

## TL;DR

Fase 5 de 8. Que el resto del sistema se entere de lo que decide el engine: WalletService paga cada trade (mueve BRL y VIB entre comprador y vendedor), OrderService muestra el estado real de cada orden y MarketService, un servicio nuevo, muestra la profundidad pública del book. Todos leen `orders.events` y aplican cada evento una sola vez.

## Why

Hasta la fase 4 el engine cruza órdenes, pero nadie más se entera: la persona no recibe lo que compró, sus órdenes quedan en `PENDING` para siempre y no hay vista de mercado. Con esta fase el MVP se puede usar de punta a punta: ingresar una orden, verla ejecutarse y ver la wallet y el mercado actualizados.

## Goals

- Pagar cada trade una sola vez, conservando la suma total de BRL y VIB.
- Que el estado de una orden llegue al del engine.
- Una vista pública de profundidad, sin exponer órdenes ni personas.

## Non-Goals

- Rutas de trades (`GET /orders/{id}/trades`, `GET /trades`).
- Rechazar con `409` la modificación o cancelación de una orden terminada: el engine ya la ignora.
- Catálogo de books, reconstrucción de la vista de mercado y control de divergencias en las reservas.
- WebSocket o SSE para la vista de mercado.

## What Changes

- Un rol nuevo `trades` en WalletService lee `TradeExecuted` y paga cada trade, con los movimientos `TRADE_PAID` y `TRADE_RECEIVED` en el ledger. Va aparte del rol `funds` para que los pagos no compitan con las reservas del engine.
- `OrderAccepted` y `OrderRejected` traen los datos de la orden, para que OrderService la cree con el primero que lea, el comando o el evento.
- El rol `projector` de OrderService lee también los eventos del engine: el estado de cada orden, su motivo, y lo ejecutado según sus trades.
- MarketService nuevo, con `GET /market/orderbook/{book}?depth=`.

## Capabilities

### New Capabilities

- `market-data`: vista pública agregada de cada book.

### Modified Capabilities

- `wallet`: se agrega el pago de trades.
- `order-management`: se agregan el ciclo de vida desde los eventos y la proyección eventualmente consistente; se modifica el registro de órdenes desde el log.
- `trading-events`: se agregan la regla de consumidores idempotentes y los datos de la orden en su primer evento.

## Assumptions

- Depende de `04-order-matching`. El engine ya libera la mejora de precio de las compras limit en su propio `RELEASE`, así que el pago solo mueve lo que cada trade usa.
- El saldo recibido en un trade queda disponible cuando WalletService lee el evento, milisegundos después del cruce.

## Impact

- **Código:** `microservices/wallet-service`, `microservices/order-service`, `microservices/market-service` (nuevo), y en `microservices/matching-engine` y `shared/eventlog/events` los datos de la orden en `OrderAccepted` y `OrderRejected`.
- **Datos:** tipos `TRADE_PAID` y `TRADE_RECEIVED` y `UNIQUE (type, message_id, currency)` en el `ledger`; `reason` y `filled_amount` en `orders` y la tabla `trades` en OrderService; la base `market_service` con la tabla `levels`.
- **Docs:** `docs/api.md`.

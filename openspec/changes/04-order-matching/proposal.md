# Proposal

## TL;DR

Fase 4 de 8. Activar el matching en el engine: price-time priority, ejecución al precio del maker, mejora de precio, órdenes market que recorren el book, self-trade prevention y las reglas de prioridad al modificar. Cada cruce publica `TradeExecuted`, y cada nivel tocado, `OrderBookLevelChanged`. El matching queda probado como determinístico y con microbenchmark.

## Why

Es la razón de ser del producto: que la mejor compra encuentre a la mejor venta, que nadie pague más que su límite y que quien llega con mejor precio reciba el precio del book (PRD, HU-12). La fase 3 ya garantiza que toda orden del book tiene su dinero reservado, así que el matching puede concentrarse solo en las reglas.

## Goals

- Implementar todas las reglas de matching del PRD con tests derivados de sus ejemplos.
- Determinismo verificable: la misma secuencia de comandos produce los mismos eventos.
- Un book en memoria que procese ≥ 25.000 comandos/s en microbenchmark.

## Non-Goals

- Mover los saldos de los trades (fase 5).
- La vista pública de profundidad (fase 5), aunque sus eventos se publican desde esta fase.

## What Changes

- Matching de órdenes limit y market en el engine.
- Self-trade prevention con cancelación del remanente entrante.
- Reglas de `sequence` en `ModifyOrder` (conserva su lugar al reducir; lo pierde al cambiar el precio o aumentar) y cruce inmediato si el nuevo precio cruza.
- Eventos `TradeExecuted` y `OrderBookLevelChanged`, con `tradeId` determinístico.
- La mejora de precio de una compra limit se libera en la wallet en el mismo lote del trade.
- Test golden de determinismo y `go test -bench`.

## Capabilities

### New Capabilities

Ninguna.

### Modified Capabilities

- `matching-engine`: se agregan determinismo, price-time priority, precio de ejecución, ejecución de market, self-trade prevention y prioridad al modificar.
- `trading-events`: se agregan los eventos de ejecución.

## Assumptions

- Depende de `03-engine-fund-reservation`.
- Una market buy compra solo unidades enteras de VIB con su `amount`.

## Impact

- **Código:** `microservices/matching-engine` (el cruce en `handlers/apply-commands`), `shared/eventlog` y `shared/money`.
- **Docs:** `.claude/standards/eventlog.md` (eventos de ejecución) y `docs/benchmark.md` (nuevo, con el microbenchmark).

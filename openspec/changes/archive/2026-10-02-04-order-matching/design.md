# Design

## Context

La fase 3 dejó el engine de un book: `handlers/apply-commands` aplica cada lote en orden (reserva en un `funds:batch`, aplica los comandos, libera en otro `funds:batch` y publica), `store/orderbook` guarda el book en memoria (niveles por lado, heap de precios, índice por `orderId`) y `shared/eventlog/events` tiene los cuatro eventos de ciclo de vida. Esta fase agrega el cruce dentro de "aplicar un comando". Motivación en `proposal.md`.

## Goals / Non-Goals

**Goals:**

- Las reglas de matching, sin I/O, testeables y medibles sin Redpanda ni PostgreSQL.

**Non-Goals:**

- Cambiar el transporte, la wallet o el flujo del lote de la fase 3.
- Mover los saldos de un trade entre personas (fase 5).

## Decisions

### D1. El cruce vive en el handler

Las reglas de cruce son reglas del caso de uso "aplicar un comando", así que van en `handlers/apply-commands`, junto a `new_order.go` y `modify_order.go`, como pide `architecture.md`. Las operaciones del book (mejor nivel, sacar, reducir) siguen en `store/orderbook`. El cruce no hace I/O: recibe el book y la orden entrante y devuelve los trades, de modo que los tests y el benchmark lo ejercitan sin el lote.

### D2. Algoritmo de cruce

```text
mientras la entrante tenga pendiente y exista bestOpposite:
  si no cruza por precio (limit) → salir
  maker := cabeza FIFO del nivel
  si maker.userId == taker.userId → cancelar el remanente (self_trade_prevented), salir
  qty := min(taker.pending, maker.pending)          (market buy: también ≤ money.QuantityFor(base, amountRemaining, price))
  si qty == 0 → salir                               (a la market buy no le alcanza para 1 unidad mínima de la base)
  emitir TradeExecuted al precio del maker
  actualizar las dos órdenes, sus reservas y el nivel; si el maker se llenó, sacarlo
lo que queda: limit → reposa; market → cancelado no_liquidity y su reserva se libera
```

- **`OrderBookLevelChanged`:** se acumula por nivel tocado y se emite una vez por nivel al final del comando, con valores absolutos.
- **Self-trade prevention:** se corta en el primer maker propio, aunque detrás haya contrapartes ajenas al mismo precio. Es la variante "cancel newest", simple y predecible.
- **Alternativa considerada:** saltear el maker propio y seguir. Cambia la prioridad de los terceros y complica el razonamiento; se descarta.

### D3. Reservas durante el cruce

Cada trade baja la reserva de las dos órdenes en lo que el trade usa: la compra en el monto pagado (`Notional(base, qty, makerPrice)`) y la venta en `qty`. Ese dinero queda reservado hasta que WalletService liquide el trade (fase 5).

- **Mejora de precio:** cuando una compra limit ejecuta por debajo de su límite, la diferencia `Notional(base, qty, limitPrice) − Notional(base, qty, makerPrice)` ya no la necesita ninguna orden. El engine la libera en la llamada `RELEASE` del mismo lote, igual que una cancelación, así queda disponible en cuanto el lote termina. Se resta lo reservado menos lo pagado, ya redondeados, para que lo pagado más lo liberado sea exactamente lo que se reservó. El maker que compra nunca tiene mejora, porque ejecuta a su propio precio.
- **Market buy:** lo que sobra de su `amount` al terminar se libera con su `OrderCancelled` (`no_liquidity`), como en la fase 3.

### D4. `ModifyOrder` con prioridad

- **Solo reducción:** se ajusta la cantidad en el nodo, que sigue en su lugar.
- **Cambio de precio o aumento:** el nodo se saca, recibe un `sequence` nuevo y pasa por el cruce como entrante con su cantidad pendiente.
- La reserva ya la ajusta la fase 3 antes de aplicar.

### D5. Eventos de ejecución

Dos eventos nuevos en `orders.events`, sobre el `EventHeader` de la fase 3:

- **`TradeExecuted`:** `tradeId`, `buyOrderId`, `sellOrderId`, `buyerId`, `sellerId`, `makerSide`, precio, cantidad y monto. Trae las dos órdenes y la cantidad, así que quien proyecta una orden sabe si quedó parcial o completa sumando sus trades; no hacen falta eventos de llenado aparte.
- **`OrderBookLevelChanged`:** el estado absoluto de un nivel (lado, precio, volumen y cantidad de órdenes), para la profundidad de MarketService (fase 5).
- **`tradeId`:** UUIDv5 de `trade:{fillIndex}` con el `id` del comando como namespace, igual que el `id` de los eventos, así un replay produce los mismos ids.

### D6. Pruebas y medición

- **Un `t.Run` por escenario de la spec**, con sus entradas y asserts escritos, como pide `go.md`.
- **Test golden de determinismo:** 10.000 comandos aleatorios con semilla fija se aplican dos veces y se comparan los eventos.
- **`go test -bench`:** sobre el cruce, con perfiles de los escenarios A (todo cruza), B (90 % reposa) y C (50 % cancelaciones).
- **Objetivo:** ≥ 25.000 comandos/s en un core, con margen sobre los 5.000/s. Se reportan `allocs/op` y `ns/op` en `docs/benchmark.md`.

### D7. Archivos

| Capa | Archivo | Responsabilidad |
| --- | --- | --- |
| `handlers/apply-commands` | `match.go` | El cruce de D2 para limit y market, el precio del maker, las reservas de D3 y el self-trade prevention |
| `handlers/apply-commands` | `level_changes.go` | Acumulado por nivel tocado y un `OrderBookLevelChanged` por nivel al final del comando |
| `handlers/apply-commands` | `new_order.go`, `modify_order.go` (cambian) | Pasan la orden por el cruce antes de dejarla en reposo; la prioridad de D4 |
| `store/orderbook` | `book.go` (cambia, si hace falta) | Lo que el cruce necesite del book además de lo que ya tiene |
| `models` | `trade.go` | `Trade` |
| `shared/money` | `arithmetic.go` (cambia) | `QuantityFor` y `AveragePrice` de D8 |
| `shared/eventlog/events` | `trades.go` | `TradeExecuted`, `OrderBookLevelChanged`, sus rutas y el `tradeId` de D5 |

- **Por qué `shared/eventlog`:** los eventos de ejecución los produce el engine y los consumen WalletService, OrderService y MarketService (fase 5); son contratos entre servicios.

### D8. Montos del trade en `shared/money`

Toda la aritmética entre la cantidad (unidades mínimas de la base) y el precio (unidades mínimas de la quote por 1 unidad entera de la base) pasa por `shared/money` y recibe la moneda base, para no suponer que la base es entera:

| Función | Uso | Fórmula | Redondeo |
| --- | --- | --- | --- |
| `Notional(base, qty, price)` (fase 3) | Monto del trade: lo que paga el comprador y recibe el vendedor | `qty × price / 10^dec_base` | Hacia arriba, igual que la reserva, para que la reserva siempre cubra el pago |
| `QuantityFor(base, amount, price)` | Cuánta base alcanza para una market buy con su `amountRemaining` | `amount × 10^dec_base / price` | Hacia abajo: nunca se compra más de lo que cubre el monto |
| `AveragePrice(base, total, qty)` | Precio promedio de una orden, para quien lo muestre | `total × 10^dec_base / qty` | Half-up. Es informativo: ningún saldo se calcula a partir de él |

- Los productos intermedios se calculan con `math/big`, como `Notional`, y un resultado fuera de `int64` devuelve `money.ErrOverflow`.

## Risks / Trade-offs

- **[Una market muy grande recorre muchos niveles en un solo comando]** → Es parte de la latencia de ese comando. Se mide con el benchmark y en la fase 6.
- **[Asignaciones de memoria por evento afectan la latencia]** → Se mide `allocs/op` en el benchmark antes de optimizar nada.

## Migration Plan

Las fases 1–3 solo corren en entornos de desarrollo. Al activar el matching se recrea el entorno: topics vacíos y bases de WalletService y OrderService nuevas.

- **Por qué no reaplicar el log viejo con matching:** el mismo comando produciría eventos distintos (por ejemplo, `TradeExecuted` donde antes hubo `OrderAccepted`) con el mismo `id`. Es la regla general: **un cambio que altera el resultado de un comando ya aplicado requiere un log nuevo**, y queda documentado en `docs/operations.md` (fase 6).
- **Rollback:** volver a la imagen de la fase 3 sobre un entorno limpio.

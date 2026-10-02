# Design

## Context

Desde la fase 4, `orders.events` tiene todo lo que decide el engine: `OrderAccepted`, `OrderRejected`, `OrderCancelled`, `OrderModified`, `TradeExecuted` y `OrderBookLevelChanged`. Un book entero va en una partición, así que sus eventos llegan en orden. Un evento puede llegar dos veces (entrega at-least-once, o el engine que se reinicia). No hay eventos de llenado: una orden está completa cuando sus trades suman su cantidad. La mejora de precio de una compra limit ya la liberó el engine (fase 4, D3), así que `TradeExecuted` solo trae lo que el trade usa. Motivación en `proposal.md`.

## Goals / Non-Goals

**Goals:**

- Tres lectores de `orders.events`, cada uno con su consumer group, que aplican cada evento una sola vez.

**Non-Goals:**

- Garantías exactly-once del broker.

## Decisions

### D1. Lectores con `shared/eventlog`

Cada servicio usa `consumer.Start` con un `consumer.Subscribe` por ruta, y cada ruta tiene su paquete `handlers/<x>` con `func Handle(ctx *gofr.Context, message events.Message) error`, como en `.claude/standards/eventlog.md`. Un error reintenta el mismo mensaje; un payload que no se puede leer se registra y se salta.

| Servicio | Dónde lee | Rutas |
| --- | --- | --- |
| WalletService | Rol nuevo `trades`: no tiene rutas y solo paga los trades, para que un atraso en los pagos no compita con las reservas del rol `funds` | `TradeExecuted` |
| OrderService | Rol `projector`, el que ya guarda las órdenes desde `orders.commands` | `OrderAccepted`, `OrderRejected`, `OrderCancelled`, `OrderModified`, `TradeExecuted` |
| MarketService | Su único proceso, junto a su ruta HTTP | `OrderBookLevelChanged` |

OrderService y MarketService no suman roles.

### D2. WalletService: pagar un trade

Por cada `TradeExecuted`, en una transacción, cada persona paga de lo que tenía reservado y recibe en su disponible, y el ledger guarda cuatro filas con el `id` del evento como `message_id`:

```text
Comprador  BRL  TRADE_PAID      reserved  −= monto
Comprador  VIB  TRADE_RECEIVED  available += cantidad
Vendedor   VIB  TRADE_PAID      reserved  −= cantidad
Vendedor   BRL  TRADE_RECEIVED  available += monto
```

- **Una sola vez:** antes de mover nada, la transacción busca un `TRADE_PAID` con ese `message_id`; si existe, el trade ya se pagó y no hace nada.
- **Ledger:** se agregan los tipos `TRADE_PAID` y `TRADE_RECEIVED`, y `UNIQUE (type, message_id)` pasa a `UNIQUE (type, message_id, currency)`, porque un trade escribe dos filas de cada tipo, una en cada moneda.
- **Reserva:** el engine reservó el trade antes de cruzarlo. Si alguna vez no alcanzara, `CHECK (reserved >= 0)` hace fallar la transacción y no se paga a medias.
- **Orden con la liberación:** el `RELEASE` del engine y el `TRADE_PAID` de una misma orden descuentan montos distintos que calculó el engine, así que el saldo final no depende de cuál llegue primero.

### D3. OrderService: el estado de la orden

**Primer evento de la orden.** `OrderAccepted` y `OrderRejected` traen, además de su header, los datos de la orden que el engine leyó del `NewOrder`: `side`, `type`, `limit`, `quantity` y `amount`; su `createdAt` es el del evento, que es el del comando. Así el `projector`, que lee `orders.commands` y `orders.events` sin orden entre ellos, nunca espera:

| Llega | Si la orden no existe | Si ya existe |
| --- | --- | --- |
| `NewOrder` | La crea en `PENDING` | No hace nada (`ON CONFLICT DO NOTHING`, como en la fase 2) |
| `OrderAccepted` | La crea con sus datos, en su estado (abajo) | Le cambia el estado |
| `OrderRejected` | La crea con sus datos, en `REJECTED` y con su `reason` | Le cambia el estado |

Todo evento posterior de una orden llega en `orders.events` después de su `OrderAccepted`, en la misma partición, así que siempre la encuentra creada.

**Estado de la orden:**

- `OrderAccepted`: `PENDING → OPEN` para una limit. Una market sigue en `PENDING` hasta que sus trades la llenan o llega su `OrderCancelled`.
- `OrderRejected`: `REJECTED`, con su `reason`.
- `OrderCancelled`: `CANCELLED`, con su `reason` si trae uno (`no_liquidity` para el remanente de una market).
- `OrderModified`: el nuevo `limit`, y como cantidad lo ya ejecutado más lo que el engine dice que queda pendiente.
- `TradeExecuted`: aplica a las dos órdenes del trade. Suma la cantidad a `filled_quantity` y el monto a `filled_amount`, y deja una limit en `PARTIALLY_FILLED`, o en `FILLED` cuando lo ejecutado alcanza su cantidad. Una market buy queda en `FILLED` cuando gastó todo su `amount`, y una market sell cuando vendió su cantidad; mientras tanto sigue en `PENDING`.

**Cada evento es una sola sentencia SQL, y la condición del cambio va en su `WHERE`.** El handler no lee la orden para decidir en Go: así dos `projector`, o un comando y un evento leídos a la vez, nunca se pisan, porque PostgreSQL bloquea la fila y evalúa la condición en el mismo paso.

| Evento | Sentencia |
| --- | --- |
| `NewOrder` | `INSERT … ON CONFLICT (id) DO NOTHING` |
| `OrderAccepted`, `OrderRejected` | `INSERT … ON CONFLICT (id) DO UPDATE SET status, reason … WHERE orders.status = 'PENDING'`: crea la orden o la saca de `PENDING`, llegue antes o después que su `NewOrder` |
| `OrderCancelled` | `UPDATE … SET status = 'CANCELLED', reason WHERE id = $1 AND status NOT IN` (finales) |
| `OrderModified` | `UPDATE … SET limit_price = $2, quantity = filled_quantity + $3 WHERE id = $1 AND status NOT IN` (finales) |
| `TradeExecuted` | En una transacción: `INSERT` del trade en `trades` con `ON CONFLICT (trade_id) DO NOTHING` y, solo si entró, un `UPDATE` de cada orden que suma a `filled_quantity` y `filled_amount` y calcula su estado en la misma sentencia |

Eso también hace que un evento repetido no se aplique dos veces, sin llevar la cuenta de eventos aplicados: un `OrderAccepted` o `OrderRejected` repetido ya no encuentra la orden en `PENDING`; un `OrderCancelled` repetido la encuentra final; un `OrderModified` repetido pone los mismos valores; y un `TradeExecuted` repetido choca con la clave de `trades` y no suma. La tabla `trades` existe solo para eso y no tiene rutas en esta fase. El `avgPrice` de una orden se calcula al leerla, con `money.AveragePrice` sobre `filled_amount` y `filled_quantity`.

### D4. MarketService: los niveles del book

MarketService es un servicio nuevo con su base `market_service` y una tabla:

```text
levels (book, side, price, volume, orders)   PRIMARY KEY (book, side, price)
```

- **`OrderBookLevelChanged`** trae el estado completo del nivel, así que se escribe tal cual: `INSERT … ON CONFLICT (book, side, price) DO UPDATE`, o `DELETE` si `volume = 0`. Un evento repetido escribe lo mismo, y como llegan en orden el último siempre gana.
- **`GET /market/orderbook/{book}?depth=`** lee los mejores niveles de cada lado con `ORDER BY price DESC` (bids) o `ASC` (asks) y `LIMIT depth`, sobre la clave primaria. Es pública: no lleva `X-User-ID`. El book pasa por `books.Normalize`; `depth` va de 1 a 100 y por defecto es 20.
- **Reconstruir la vista:** basta con borrar la base y arrancar con un consumer group nuevo, que relee `orders.events` desde el principio. No hay un comando para eso.

### D5. Paquetes

Con la estructura de `.claude/standards/architecture.md` y `handlers.md`: un paquete por entry point, sin capa de servicio, y el store como funciones de paquete que los tests reemplazan con `InitMock(t)`.

**WalletService**

| Paquete | Qué agrega |
| --- | --- |
| `handlers/apply-trade` | Lee `TradeExecuted`, arma el `Trade` y llama a `walletdb.ApplyTrade` |
| `store/walletdb` | `ApplyTrade`: la transacción de D2 |
| `models` | `Trade` y los tipos `TRADE_PAID` y `TRADE_RECEIVED` |
| `migrations` | Los tipos nuevos y `UNIQUE (type, message_id, currency)` en `ledger` |

`main.go` suma el rol `trades` y `configs/.env.example` su consumer group.

**OrderService**

| Paquete | Qué agrega |
| --- | --- |
| `handlers/apply-order-accepted`, `apply-order-rejected`, `apply-order-cancelled`, `apply-order-modified` | Leen su evento y llaman a una función del store |
| `handlers/apply-trade-executed` | Arma el trade y llama a `orderdb.InsertTrade` |
| `store/orderdb` | Las sentencias de D3: `InsertOrUpdateOrder`, `UpdateCancelledOrder`, `UpdateModifiedOrder` e `InsertTrade` |
| `models` | Los estados de una orden, `NewOrderFromDetails` y `Trade` |
| `migrations` | `reason` y `filled_amount` en `orders`, y la tabla `trades` |

**MarketService**

| Paquete | Qué agrega |
| --- | --- |
| `handlers/apply-level-changed` | Lee `OrderBookLevelChanged` y escribe el nivel |
| `handlers/get-orderbook` | `GET /market/orderbook/{book}` |
| `store/marketdb` | `UpdateLevel` (upsert o delete) y `ListLevels` |
| `models` | `Level`, `OrderBook`, `ErrInvalidDepth` y `ErrBookNotFound` |
| `migrations` | La tabla `levels` |

## Risks / Trade-offs

- **[El saldo recibido en un trade llega con retraso]** → La nota en `docs/api.md`. Las reservas nunca dependen de ese saldo: como mucho, se rechaza una orden que habría pasado unos milisegundos después.
- **[La proyección muestra un estado viejo un momento]** → Es eventualmente consistente. `change` y `close` pueden aceptar algo que el engine luego ignora; la orden muestra su estado real cuando la proyección se pone al día.
- **[Un `OrderModified` viejo releído después de uno nuevo]** → Pasa solo si el `projector` relee eventos tras reiniciarse, y como los relee en orden, el último vuelve a quedar.

## Migration Plan

Se despliega el código; los consumer groups nuevos leen `orders.events` desde el inicio. MarketService se despliega por primera vez. Rollback: detener los lectores; su progreso queda en sus consumer groups.

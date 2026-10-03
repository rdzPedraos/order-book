# Design

## Context

Motivación en `proposal.md` (Why). Estado actual:

- `consumer.Start` (`shared/eventlog/consumer/consumer.go`) lee como consumer group. Por cada registro busca el handler de su ruta, lo aplica y lo confirma en Kafka con `CommitRecords`, uno por uno, incluso los que ningún handler pidió.
- Lo usan tres lectores, cada uno con un handler por ruta que hace una escritura por mensaje:

  | Lector | Rutas | Escritura por mensaje |
  | --- | --- | --- |
  | OrderService `projector` | `NewOrder` y los 5 eventos de una orden (6 handlers) | una sentencia; un trade, una transacción con el trade y sus dos órdenes |
  | MarketService | `OrderBookLevelChanged` | un upsert o un delete del nivel |
  | WalletService `trades` | `TradeExecuted` | una transacción: ¿ya se pagó?, y 4 cambios de saldo con sus 4 movimientos |

- Las proyecciones ya son idempotentes y toleran el desorden entre topics: cada sentencia lleva su condición en el `WHERE`, los trades se deduplican por `trade_id`, y el ledger tiene `UNIQUE (type, message_id, currency)`.
- `consumer.StartPartition`, el lector del engine, ya entrega lotes: lo que haya disponible, hasta `maxBatch`, reintentando el lote completo si falla.

## Goals / Non-Goals

**Goals:**

- Mismo resultado en cada proyección que aplicando los mensajes uno por uno.

**Non-Goals:**

- Cambiar `consumer.StartPartition` o el engine.

## Decisions

### D1. `consumer.StartGroup`: un consumer group por lotes

```go
func StartGroup[C context.Context](ctx C, brokers []string, group string, routes []string, maxBatch int,
	logger Logger, applyBatch func(C, []events.Message) error) error
```

Reemplaza a `consumer.Start` y queda simétrico con `StartPartition`: uno lee un consumer group, el otro una partición fija, y los dos entregan lotes. Lleva un nombre nuevo para que cada servicio pase al lote en su propia tarea mientras el resto sigue compilando, y `Start` sale cuando ya nadie lo usa.

- **Cómo lee:** toma lo que haya disponible, hasta `maxBatch` registros, sin esperar a juntar más, igual que `StartPartition`. Con poco tráfico los lotes son chicos y no agregan demora.
- **Qué entrega:** solo los mensajes de `routes`, en el orden en que los leyó. Un registro que no es un mensaje se registra en el log y queda fuera, como hoy.
- **Cuándo confirma:** confirma en Kafka todos los registros leídos, incluidos los que no eran de sus rutas, cuando `applyBatch` termina bien. Si falla, reintenta el mismo lote hasta que funcione o termine `ctx`, y no confirma nada.
- **Entrega al menos una vez:** si el lector cae entre la base y la confirmación, el lote completo llega de nuevo, hasta `maxBatch` mensajes. Los handlers ya son idempotentes.
- **`maxBatch`:** 500, como el engine. Lo pasa cada `main`.
- **Salen** `consumer.Start`, `consumer.Subscribe`, `Subscription` y el handler por mensaje. Es un cambio que rompe la API de `shared/`, permitido porque es un change de OpenSpec, y sus únicos tres usuarios cambian en este change.

**Alternativas descartadas:**

| Alternativa | Por qué no |
| --- | --- |
| Agrupar solo la confirmación en Kafka y dejar un handler por mensaje | Cada mensaje sigue siendo un viaje y un commit en la base: la ganancia sería chica |
| Un lote por ruta | Partiría el lote en varias llamadas a la base y perdería el orden entre rutas |

### D2. OrderService: una función aplica el lote

- **Handler:** `handlers/apply-order-messages` reemplaza a los 6 handlers por ruta.
  - Arma un `orderdb.Change` por mensaje, con el mismo parseo de hoy para cada ruta. Un mensaje que no se puede parsear se registra en el log y queda fuera.
  - Llama una vez a `orderdb.ApplyChanges(ctx, changes)`.
  - Actualiza el gauge `order_projection_lag_seconds` con el último `NewOrder` del lote.
- **`orderdb.Change`:** el tipo de cambio (`insert`, `first_event`, `cancel`, `modify` o `trade`) y sus datos: la orden, el id, el motivo, el límite y lo pendiente, o el trade, y la fecha del mensaje.
- **Función:** `apply_order_changes(changes jsonb)` recorre los cambios en orden y aplica a cada uno la sentencia de hoy:

  | Cambio | Sentencia |
  | --- | --- |
  | `insert` | `insertOrder` |
  | `first_event` | `insertOrUpdateOrder` |
  | `cancel` | `updateCancelledOrder` |
  | `modify` | `updateModifiedOrder` |
  | `trade` | `insertTrade` y, solo si entró, `addTradeToOrder` para las dos órdenes |

  Va en un loop y no en sentencias sobre todo el conjunto, porque la misma orden puede tener varios cambios en un lote y cada uno tiene que ver lo que dejó el anterior. Por ejemplo, dos trades suman su cantidad. Una sola sentencia aplica todo el lote o nada.
- **Salen del store** `InsertOrder`, `InsertOrUpdateOrder`, `UpdateCancelledOrder`, `UpdateModifiedOrder` e `InsertTrade`, con sus constantes: su SQL pasa a la función. El mock implementa `ApplyChanges` recorriendo la lógica en memoria que ya tiene.

### D3. MarketService: el último estado de cada nivel, en una sentencia

- **Handler:** `handlers/apply-level-changes` arma un `models.Level` por mensaje y llama una vez a `marketdb.UpdateLevels(ctx, levels)`.
- **Store:** guarda solo el último estado de cada nivel (`book`, `side`, `price`) del lote. Cada evento trae el estado completo de su nivel, así que los anteriores no importan. Después aplica todo con una sentencia: borra los niveles con `volume = 0` y hace upsert del resto.

  ```sql
  WITH changes AS (SELECT * FROM jsonb_to_recordset($1) AS c(book text, side text, price bigint, volume bigint, orders int)),
  emptied AS (DELETE FROM levels USING changes WHERE changes.volume = 0 AND levels.book = changes.book
                AND levels.side = changes.side AND levels.price = changes.price)
  INSERT INTO levels (book, side, price, volume, orders)
  SELECT book, side, price, volume, orders FROM changes WHERE volume > 0
  ON CONFLICT (book, side, price) DO UPDATE SET volume = EXCLUDED.volume, orders = EXCLUDED.orders
  ```

  Quedarse con el último es además lo que hace válida la sentencia: un upsert no puede tocar la misma fila dos veces. No hace falta una función.
- **Sale** `UpdateLevel`.

### D4. WalletService `trades`: una función paga el lote

- **Handler:** `handlers/apply-trades` arma un `models.Trade` por mensaje, como hoy, y llama una vez a `walletdb.ApplyTrades(ctx, trades)`.
- **Store:** arma en Go los 4 movimientos de cada trade (`BuildMovements`, con sus ids y fechas) y los manda a la función.
- **Función:** `apply_trade_movements(movements jsonb)` recorre los movimientos en orden. Para cada uno inserta su fila en el ledger con `ON CONFLICT (type, message_id, currency) DO NOTHING`. Solo si entró, mueve el saldo:
  - `TRADE_PAID`: `takeReserved`;
  - `TRADE_RECEIVED`: `addAvailable`.
- **Sin la consulta previa del trade:** hoy se pregunta "¿ya se pagó este trade?" antes de aplicarlo. Ya no hace falta: los 4 movimientos de un trade tienen cada uno un `(type, currency)` distinto, y el `UNIQUE` del ledger ya los aplica una sola vez. Como la función es atómica, un trade no puede quedar pagado a medias.
- **Si un trade toma más de lo reservado,** el `CHECK (reserved >= 0)` hace fallar el lote y se reintenta, igual que hoy pasa con ese trade solo.
- **Sale** `ApplyTrade` con `isTradeApplied`.

### D4b. La wallet bloquea los saldos siempre en el mismo orden

La medición de D8 mostró deadlocks. WalletService tiene dos roles que escriben las mismas filas de `balances`: `funds`, con las reservas y liberaciones del engine, y `trades`, con el pago de los trades.

- **Cómo se producía:** cada lote bloqueaba las filas en el orden de sus operaciones. Si `funds` bloquea a Ana y pide a Beto mientras `trades` bloqueó a Beto y pide a Ana, se esperan en círculo.
- **Cuánto costaba:** PostgreSQL lo detecta pasado el `deadlock_timeout` (1 s), cancela un lote y ese lote se reintenta. En una corrida a 5.000/s hubo 87 deadlocks, `funds:batch` subió a p90 de ~1.040 ms y el engine bajó a ~1.277 órdenes/s.

**La solución:** al empezar, `apply_funds_batch` y `apply_trade_movements` bloquean todas las filas de saldo de su lote con una sola sentencia, ordenadas por `user_id` y `currency`:

```sql
PERFORM 1 FROM balances WHERE (user_id, currency) IN (<los saldos del lote>)
ORDER BY user_id, currency FOR UPDATE;
```

- **Por qué no hay círculo:** PostgreSQL toma los bloqueos en el orden del `ORDER BY`. Los dos piden en el mismo orden, así que el que llega segundo espera sin tener nada tomado, y nunca se forma un círculo.
- **Cuánto espera el segundo:** lo que tarda en correr el lote del primero, unos milisegundos, porque ya no hay viajes de red dentro del lote. PostgreSQL atiende a los que esperan una fila en orden de llegada.
- **Filas que faltan:** en un trade, quien recibe puede no tener fila en esa moneda. `apply_trade_movements` las crea antes, en el mismo orden (`INSERT … ON CONFLICT DO NOTHING`), así que el bloqueo las incluye y su loop solo hace `UPDATE`.
- **Dónde:** una migración nueva de WalletService, `20261008000000_lock_balances_in_order.go`, con las dos funciones completas (`CREATE OR REPLACE FUNCTION`).
- **Alternativas descartadas:**

  | Alternativa | Por qué no |
  | --- | --- |
  | Lotes de trades más chicos | Bajan los deadlocks pero no los eliminan |
  | Un trade por transacción | Vuelve a un viaje y un commit por trade |
  | Que el engine pague los trades en `funds:batch` (un solo escritor) | Es un cambio de diseño más grande, que queda para otro change si hiciera falta |

### D5. Dónde vive cada componente

| Componente | Capa y path |
| --- | --- |
| `consumer.StartGroup` | `shared/eventlog/consumer/consumer.go` (salen `Start` y `subscription.go`) |
| Handler del projector | `microservices/order-service/handlers/apply-order-messages` (salen los 6 handlers por ruta) |
| `orderdb.Change` y `orderdb.ApplyChanges` | `microservices/order-service/store/orderdb` |
| `apply_order_changes` | `microservices/order-service/migrations/20261007000000_create_apply_order_changes.go` |
| Handler de niveles | `microservices/market-service/handlers/apply-level-changes` (sale `apply-level-changed`) |
| `marketdb.UpdateLevels` | `microservices/market-service/store/marketdb` |
| Handler de trades | `microservices/wallet-service/handlers/apply-trades` (sale `apply-trade`) |
| `walletdb.ApplyTrades` | `microservices/wallet-service/store/walletdb` |
| `apply_trade_movements` | `microservices/wallet-service/migrations/20261007000000_create_apply_trade_movements.go` |

- `consumer` ya está en `shared/` porque lo usan los tres servicios. No hay dependencias nuevas.
- Las funciones siguen la decisión de `optimize-wallet-funds-batch`: el store escribe el SQL a mano, la migración crea la función, y las reglas se prueban contra PostgreSQL con el tag `integration`.

### D6. Standards

- **`eventlog.md` (Consuming):** cada rol tiene un `StartGroup` con sus rutas y un handler de lote, `func Handle(ctx *gofr.Context, messages []events.Message) error`, que aplica el lote con una llamada a su store. El lote se confirma en Kafka cuando el handler termina bien. Lo demás queda igual: idempotencia, saltar lo que nunca se podrá aplicar y devolver error solo cuando reintentar puede funcionar. Cambia una regla: un handler ya no puede devolver error hasta que llegue un mensaje de otro topic. Su lote se reintentaría para siempre sin leer el siguiente, que es justo donde viene ese mensaje. Aplica cada mensaje en el orden en que llegue, como ya hace el projector con el `NewOrder` y el primer evento de una orden.
- **`handlers.md`:** cada lector del log es un handler que recibe los lotes de sus rutas. Deja de ser una excepción del lector del engine.

### D7. Tests

- **`consumer`** (contra `kfake`):
  - un lote llega con solo los mensajes de sus rutas;
  - un lote aplicado queda confirmado: el mismo grupo no lo recibe otra vez;
  - un lote que falla se reintenta sin confirmarse;
  - un registro que no es un mensaje queda fuera.
- **Handlers:** con el mock del store. Los escenarios de los handlers que se juntan pasan al handler de lote con su mismo nombre. Se agregan los casos de lote: varias rutas en un lote y un mensaje inválido que queda fuera.
- **Stores:**
  - unit tests con el SQL mock: una sola consulta con el JSON del lote, y un error de la base devuelto;
  - tests de integración contra PostgreSQL con los escenarios de hoy, ahora a través de la función de lote.
  - Casos nuevos de integración:

    | Store | Caso |
    | --- | --- |
    | Orders | dos trades de la misma orden en un lote suman; el `NewOrder` y el `OrderAccepted` de una orden en un lote, en los dos órdenes; un lote que falla no aplica nada |
    | Market | un nivel que cambia dos veces en un lote queda con el último estado; un nivel vaciado en el lote se borra |
    | Wallet | dos trades de la misma persona en un lote; un trade repetido dentro del lote se paga una vez |

### D8. Medición

1. Reconstruir las imágenes de los tres servicios en minikube y reiniciar sus pods.
2. Correr `tools/loadgen` con el escenario B a 5.000/s durante 30 s.
3. Seguir con `rpk group describe` el lag de `order-service`, `market-service` y `wallet-trades`.

Cumple si los tres llegan a lag 0 en menos de 10 s después de que el engine termina de procesar la corrida. Si se forma lag, se mide a qué ritmo drena el projector, contra los ~780 mensajes/s de la línea base.

## Risks / Trade-offs

- **[Las reglas de la proyección de órdenes y del pago de trades quedan en PL/pgSQL, fuera de los unit tests y de CI]** → Los tests de integración cubren cada escenario, y se corren antes de cerrar cada tarea.
- **[Un mensaje que siempre falla en la base bloquea todo su lote, no solo a él]** → Hoy ese mensaje ya bloquea a su lector, porque se confirma en orden. Lo que no se puede parsear se salta antes de llamar a la base.
- **[Un lote grande de trades hace esperar a `funds:batch` mientras corre]** → Son milisegundos con el tope de 500 (D4b). Si la medición muestra esperas largas, se baja el tope de los lotes de trades.
- **[Una caída repite hasta 500 mensajes]** → Aceptado: las proyecciones son idempotentes.
- **[`consumer.Start` desaparece]** → Sus tres usuarios pasan a `StartGroup` en este change, y nada fuera del repo lo usa.

## Migration Plan

1. Se despliegan las imágenes nuevas de OrderService, MarketService y WalletService. Cada pod crea su función al migrar, al arrancar.
2. Los consumer groups no cambian, así que cada lector retoma desde su último offset confirmado.
3. **Rollback:** las imágenes anteriores. Las funciones quedan en la base, sin uso.

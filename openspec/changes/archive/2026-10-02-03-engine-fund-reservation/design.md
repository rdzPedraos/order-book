# Design

## Context

Las fases 1 y 2 dejaron los comandos de cada book ordenados en `orders.commands`, partición 0 para `BRL-VIB`. Esta fase introduce las dos autoridades del exchange: WalletService, la del dinero, un servicio stateless cuyo estado vive en su PostgreSQL, así que cualquier réplica atiende cualquier petición; y el MatchingEngine, la del book, el único componente con estado: lo mantiene en memoria y por eso corre como single writer por book. Motivación en `proposal.md`.

La restricción que domina el diseño: con 5.000 órdenes/s en un solo book, el engine tiene unos **200 µs por comando** de punta a punta, incluida la reserva.

## Goals / Non-Goals

**Goals:**

- Reserva y liberación correctas bajo concurrencia, e idempotentes: un mensaje repetido no mueve fondos dos veces.
- Un loop single writer cuyo diseño ya soporte el throughput de la fase 4.

**Non-Goals:**

- Matching (fase 4) y liquidación de trades (fase 5).
- Snapshots (fase 7): por ahora el reinicio relee el log desde el principio (D6).

## Decisions

### D1. Modelo de datos de WalletService

Dos tablas: los saldos y su log.

- `balances(user_id, currency, available, reserved)`, con PK `(user_id, currency)` y `CHECK (available >= 0 AND reserved >= 0)`. No hay tabla de personas: la wallet de un `user_id` sin filas tiene saldo cero en BRL y VIB, y el primer depósito crea su fila con `INSERT … ON CONFLICT DO UPDATE`.
- `ledger(id, user_id, currency, type, amount, order_id, message_id, result, created_at)`, con `UNIQUE (type, message_id)`: el histórico inmutable de lo que entra y sale, del que sale `GET /wallet/movements`. Solo se le agregan filas.
  - `type`: `DEPOSIT`, `WITHDRAWAL`, `RESERVE` o `RELEASE`.
  - `message_id`: el `id` del mensaje del log que causó el movimiento (el `NewOrder`, `ModifyOrder` o `CancelOrder`; en la fase 5, el evento del trade). Si el engine reintenta un lote, manda los mismos mensajes con los mismos `id`, así que el `UNIQUE` descarta lo ya aplicado. En depósitos y retiros, que no vienen del log, es `NULL`, y un `NULL` no choca con el `UNIQUE`.
  - `order_id`: la orden del movimiento, para saber qué queda reservado de cada una; `NULL` en depósitos y retiros.
  - `result`: `OK` o `insufficient_funds`. Una reserva rechazada también queda registrada, para que repetir el mismo mensaje dé el mismo resultado aunque la persona haya depositado después.

  Por ejemplo:

  ```text
  type      amount   order_id  message_id  result
  DEPOSIT   1000.00  —         —           OK                   Ana deposita R$ 1.000
  WITHDRAWAL 100.00  —         —           OK                   Ana retira R$ 100
  RESERVE    800.00  orden-A   msg-1       OK                   el NewOrder de A reserva R$ 800
  RESERVE    500.00  orden-B   msg-2       insufficient_funds   el NewOrder de B no alcanza (quedan R$ 100)
  RELEASE    800.00  orden-A   msg-3       OK                   el CancelOrder de A libera R$ 800
  ```

  Si el engine vuelve a pedir la reserva de `msg-1` (por ejemplo, al reintentar un lote), encuentra la fila y devuelve `OK` sin congelar otra vez; la de `msg-2` vuelve a dar `insufficient_funds` aunque Ana tenga saldo ahora.

Cada operación es un `UPDATE … WHERE available >= x` (o `reserved >= x` al liberar) y su fila en el ledger, en la misma transacción, sin read-check-write:

- **Concurrencia:** el `UPDATE` bloquea la fila de esa persona y moneda; una operación concurrente sobre la misma fila espera, y al seguir PostgreSQL reevalúa el `WHERE` sobre la fila ya actualizada, así que dos operaciones nunca gastan el mismo saldo. El `CHECK` es la red de seguridad. No hace falta un lock externo (Redis): la base ya es la que decide, y un lock aparte solo agregaría otra forma de fallar.
- **Deadlocks:** un `funds:batch` bloquea varias filas en una transacción; si los engines de dos books bloquean filas de las mismas personas en orden opuesto, PostgreSQL aborta una transacción y el engine reintenta su lote (D3), sin efectos.
- **Fuera del MVP:** el `Idempotency-Key` de depósitos y retiros (un reintento del cliente acredita dos veces; son simulados) y el tope de una liberación contra lo reservado por cada orden (una liberación no puede dejar `reserved` negativo, pero no se controla por orden).

### D2. `POST /wallet/internal/funds:batch`

- Recibe una lista ordenada de operaciones `RESERVE` y `RELEASE`, cada una con `messageId`, `orderId`, `userId`, `currency` y `amount`, y las aplica en una sola transacción, en orden.
- **`RESERVE`:**
  1. si el ledger ya tiene un `RESERVE` con ese `message_id`, devuelve su `result`;
  2. si no, intenta `UPDATE balances SET available = available - x, reserved = reserved + x WHERE … AND available >= x`;
  3. inserta el `RESERVE` en el ledger con su `result`: `OK`, o `insufficient_funds` si el `UPDATE` no afectó filas (un `user_id` sin saldo también es `insufficient_funds`).
- **`RELEASE`:** si el ledger ya tiene un `RELEASE` con ese `message_id`, no hace nada. Si no, mueve el monto de `reserved` a `available` con `WHERE reserved >= x` (falla con `release_exceeds_reservation` si no alcanza) e inserta el `RELEASE` en el ledger.
- **Por qué en lote:** una llamada por comando, con un round-trip a PostgreSQL de unos 0,5–1 ms, limita el book a ~1.000–2.000 órdenes/s. En lote se amortiza el round-trip sin cambiar la semántica: la reserva sigue siendo síncrona y ninguna orden entra sin fondos.
- **Alternativa descartada:** que el engine lleve una copia de los saldos en memoria. Duplica la autoridad sobre el dinero y choca con los depósitos y retiros concurrentes.
- **Transporte:** HTTP interno con el cliente de servicios de Gofr, hacia el rol `funds` (D9). gRPC queda evaluado en D9 y se decide en la fase 6.

### D3. Loop del engine

```text
poll de la partición (lo que haya, hasta 500 comandos)
  → descartar duplicados (id del mensaje / orderId ya vistos)
  → funds:batch(RESERVE de cada NewOrder y de cada ModifyOrder al alza, en orden)
  → por cada comando, en orden: sequence++, aplicar al book, generar eventos, acumular RELEASE
  → funds:batch(RELEASE acumulados)
  → producir los eventos del lote en orders.events (acks=all) y esperar el ack
```

- **Lectura:** un `consumer.StartPartition` por book (D7) lee su partición fija sin consumer group, en lotes. El lote es lo que haya disponible, hasta 500 comandos, sin esperar a juntar más: con poco tráfico sale de a uno, y con mucho crece solo, que es cuando más rinde un solo viaje a la wallet. El tope se calibra con el benchmark de la fase 6. Gofr se usa solo para health; el rendimiento se mide con el benchmark, sin métricas propias del engine.
- **Las reservas van antes de aplicar el lote**, porque deciden si la orden entra. Las liberaciones van antes de publicar, para que ningún `OrderCancelled` salga antes de que el dinero esté disponible.
- **Reserva calculada sobre la orden ya conocida:** en un `ModifyOrder` la reserva adicional se calcula contra el estado de la orden. Si dentro del lote hay un `ModifyOrder` después del `NewOrder` de la misma orden, se resuelve con una tercera llamada `funds:batch` intermedia. Es un caso raro y se mide.
- **Wallet caída:** reintento con backoff y sin avanzar. Se pierde throughput, pero nunca orden ni correctitud.
- **Determinismo:** el engine no lee el reloj (los timestamps vienen en el comando). Los resultados de las reservas son idempotentes, así que reintentar un lote obtiene exactamente las mismas respuestas.

### D4. Estructuras del Order Book

Por lado:

- `map[price]*PriceLevel` (precio en unidades mínimas de la quote);
- un heap de precios (max-heap para bids, min-heap para asks) con borrado perezoso: cuando un nivel se vacía se borra del map, y el heap lo descarta al llegar al tope;
- `PriceLevel`, con una lista doblemente enlazada de `OrderNode` en orden FIFO por `sequence` y su volumen agregado.

Global: `map[orderID]*OrderNode`.

Todo vive en el tipo `orderbook.Book` (mejor nivel, get, put, remove e iteración ordenada). La fase 8 agrega otra implementación, y recién entonces se extrae la interfaz `LevelStore`: hoy tendría una sola.

- **Alternativa considerada:** un árbol ordenado (B-tree). Da iteración ordenada sin borrado perezoso. Se elige el heap porque el matching solo necesita el mejor precio, que el heap da en O(1), y es más simple de implementar con `container/heap` de la stdlib. La interfaz permite cambiarlo.

### D5. IDs determinísticos y eventos

- `sequence` es un contador por book.
- El payload de cada evento lleva el offset del comando que lo causó y su `index` dentro de ese comando, que marcan hasta dónde publicó el engine (D6).
- El `id` del `Message` de cada evento es el UUIDv5 del `index` con el `id` del comando como namespace. Sale solo de lo que el log guarda sin cambios, así un lote reintentado o un reinicio reemiten los mismos ids sin depender de que el engine vuelva a contar el `sequence` igual. El `id` del comando ya es único en todo el sistema, así que no hace falta el `book`.
- Una liberación que decide el engine (por ejemplo, el remanente de una market) lleva el `id` del mensaje que la causó, el mismo `NewOrder` que reservó: `RESERVE` y `RELEASE` no chocan en el `UNIQUE (type, message_id)` porque son de tipos distintos, y repetirla no tiene efecto.
- Los payloads de los eventos de esta fase se agregan al catálogo de `shared/eventlog/events`, con sus rutas `orders.events.<tipo>`, y se publican con `producer.Publish`, que toma el topic de la ruta y la partición del book.

### D6. Reinicio sin eventos repetidos

El book vive en memoria, así que al arrancar el engine lo reconstruye releyendo su partición de `orders.commands` desde el principio. Releer no debe repetir nada hacia afuera:

1. **Antes de leer**, el engine busca el último evento que publicó en `orders.events` para su book (`consumer.GetLastMessage`). Cada evento lleva el offset del comando que lo causó y su índice dentro de ese comando, así que ese último evento marca el punto exacto hasta donde ya publicó. Si el topic está vacío, no publicó nada.
2. **Relee desde el principio** y aplica cada comando al book como siempre. La wallet responde lo ya anotado sin congelar otra vez, porque el ledger reconoce el `message_id` (D1).
3. **Publica solo los eventos posteriores a ese punto.** Los anteriores ya salieron y se descartan sin publicarse.

Si el engine se cayó a mitad de un lote, después de la wallet pero antes de publicar todos sus eventos, al arrancar el último publicado queda antes de esos eventos y el engine publica los que faltaron. Nadie recibe un evento dos veces y ninguno se pierde.

- **Fuera del MVP:** una señal de "listo" que frene el tráfico mientras relee (el engine no recibe tráfico directo, solo lee el log) y los snapshots, que hacen que no haya que releer desde el principio (fase 7).

### D7. Estructura de WalletService y MatchingEngine

Siguen `.claude/standards/architecture.md` y `handlers.md`: un paquete por entry point en `handlers/<kebab>`, sin capa de servicio, y el store con funciones de paquete sobre una variable que los tests reemplazan con `InitMock(t)`, como `orderdb`.

**WalletService** (nuevo):

| Paquete | Responsabilidad |
| --- | --- |
| `handlers/get-wallet` | `GET /wallet`: saldo de BRL y VIB, en cero si la wallet no tiene filas |
| `handlers/create-deposit` | `POST /wallet/deposits` (BRL y VIB): parseo con `shared/money` y depósito |
| `handlers/create-withdrawal` | `POST /wallet/withdrawals`: solo BRL (`ErrCurrencyNotWithdrawable`) |
| `handlers/list-movements` | `GET /wallet/movements`: cursor y rango de fechas |
| `handlers/apply-funds-batch` | `POST /wallet/internal/funds:batch` (D2), solo en el rol `funds` (D9): bind de la lista de operaciones y resultado de cada una |
| `store/walletdb` | `GetBalances`, `Deposit`, `Withdraw`, `ListMovements` y `ApplyFundsBatch`. Cada función es un caso de uso atómico y abre su propia transacción: el `UPDATE` condicional de `balances` y su fila en `ledger` van juntos. `InitMock(t)` para los tests de los handlers |
| `models` | `Balance`, `Movement`, las operaciones y resultados de `funds:batch` con sus reglas (`Validate`: monto positivo, moneda conocida) y los errores de dominio (`ErrInsufficientFunds`, `ErrCurrencyNotWithdrawable`, `ErrReleaseExceedsReservation`, `ErrInvalidAmount`) |
| `migrations` | Tablas y `CHECK` de D1 |

- `main.go` elige el rol con `ROLE` (D9): `api` registra las rutas públicas con `identity.Middleware` y `funds` solo `funds:batch`.
- La transacción la delimita `walletdb`, porque cada caso de uso es una sola operación atómica sobre la base; los handlers no la ven.
- La idempotencia de `funds:batch` vive en el `UNIQUE (type, message_id)` del ledger, y no en `shared/`: solo la usa este servicio.

**MatchingEngine** (nuevo):

| Paquete | Responsabilidad |
| --- | --- |
| `shared/money` | `arithmetic.go`: `Notional` de D8, con redondeo hacia arriba y el producto en `math/big` |
| `shared/eventlog/events` | `lifecycle.go`: las rutas `orders.events.<tipo>` de los cuatro eventos de ciclo de vida y sus payloads, sobre el envelope común `Message` con `id` determinístico (D5) |
| `shared/eventlog/consumer` | `partition.go` (nuevo): `StartPartition` lee una partición fija desde el principio, sin consumer group, en lotes de lo disponible hasta un tope, entrega cada lote al handler y lo reintenta si el handler falla (D3); `GetLastMessage` devuelve el último mensaje de una partición, o ninguno si está vacía (D6) |
| `handlers/apply-commands` | El handler del lote (D3): dedupe, `RESERVE` previas, `sequence++`, aplicación de cada comando con sus reglas (`new_order.go`, `cancel_order.go`, `modify_order.go`, `reservation.go`), `RELEASE` acumulados y publicación con `producer.Publish` de los eventos posteriores al último publicado (D6) |
| `store/orderbook` | El book en memoria de D4 (`Book`); la fase 8 agrega otra implementación y la interfaz común |
| `store/walletclient` | `ApplyFundsBatch(ctx, operations)`: cliente HTTP de servicios de Gofr hacia `funds:batch`, con `InitMock(t)` |
| `models` | Estado de la orden en el engine, resultado de cada comando y los motivos de rechazo (`insufficient_funds`, `order_not_found`, …) |

- `main.go`, por cada book: busca su último evento publicado con `consumer.GetLastMessage` y arranca un `consumer.StartPartition` sobre la partición que da `shared/books`, con `applycommands.Handle`.
- **Por qué en `shared/`:** los eventos de ciclo de vida son el contrato entre el engine (productor) y sus consumidores (OrderService y MarketService desde la fase 5); `StartPartition` porque un servicio no habla con Kafka directo y el engine necesita leer su partición en orden y en lotes, sin consumer group.

### D8. Monto a reservar en `shared/money`

La reserva de una compra limit es el monto en la quote de `quantity` a `limitPrice`. Se calcula con `money.Notional(base, quantity, price)` en `shared/money`, porque WalletService, el engine y los consumidores de la fase 5 deben obtener exactamente el mismo número.

- **Fórmula:** el precio es por 1 unidad entera de la base y la cantidad viene en unidades mínimas de la base, así que `Notional = quantity × price / 10^base.GetDecimals()`. Con VIB (0 decimales) el divisor es 1, pero la función no lo supone: si mañana VIB tiene decimales, solo cambia su entrada en el registro de monedas.
- **Redondeo hacia arriba:** si la división no es exacta (0,01 VIB a R$ 0,01 da R$ 0,0001), se redondea al centavo siguiente. Una reserva redondeada hacia abajo dejaría la orden sin fondos para pagar su último trade.
- **Overflow:** el producto intermedio puede superar `int64` aunque el resultado quepa, así que se calcula con `math/big`, que no tiene límite de tamaño. Si el resultado no cabe en `int64`, devuelve `money.ErrOverflow` y la orden se rechaza.
- **Moneda desconocida:** propaga `money.ErrUnknownCurrency` de `base.GetDecimals()`.
- Market buy reserva su `amount` y una venta reserva su `quantity` en la base, sin aritmética.

### D9. Roles `api` y `funds` de WalletService

`funds:batch` congela y libera el dinero de cualquier persona, así que no puede quedar al alcance de quien llega a las rutas públicas. WalletService arranca en uno de dos roles, con `ROLE`, como OrderService:

- **`api`:** registra solo las rutas públicas (`/wallet`, `/wallet/deposits`, `/wallet/withdrawals`, `/wallet/movements`) con `identity.Middleware`.
- **`funds`:** registra solo `POST /wallet/internal/funds:batch`, sin `identity.Middleware`: no tiene `X-User-ID`, cada operación trae su `userId`.

Un rol no registra las rutas del otro, así que en los pods de `api` la ruta interna responde `404`: no existe. Los dos roles usan la misma base y el mismo código (`store/walletdb`, `models`), y escalan por separado: `funds` está en el camino crítico de cada lote del engine y `api` atiende a las personas. En local corren como dos procesos; en Kubernetes (fase 6) cada rol tiene su Deployment y su Service, el ingress apunta solo a `api`, y una NetworkPolicy deja conectarse a `funds` solo desde el engine.

- **Por qué no dos servicios:** depósitos, retiros y reservas escriben la misma fila de `balances`, y solo el mismo `UPDATE … WHERE available >= x` en la misma base impide gastar dos veces el mismo dinero. Dos servicios tendrían que compartir la base o hacer del segundo un proxy del primero; el dinero tiene una sola autoridad.
- **Alternativa descartada: un token de servicio** (`X-Internal-Token` en las rutas internas). Con la ruta fuera del rol público y la NetworkPolicy, es una segunda capa que el MVP no necesita.
- **Alternativa evaluada: gRPC para `funds:batch`.** Gofr lo soporta (servidor en `GRPC_PORT` con `app.RegisterService`, cliente con trazas y métricas). A favor: un puerto propio que solo expone el Service interno, contrato tipado en `.proto` y menos costo por llamada (HTTP/2 persistente, protobuf). En contra: `protoc`/`buf` y código generado en local y en CI, tests con servidores en memoria (`bufconn`) en lugar de `httptest`, y el balanceo en Kubernetes: un Service normal balancea por conexión, y como gRPC mantiene una conexión HTTP/2 larga, el engine quedaría pegado a una sola réplica de `funds` sin un Service headless y balanceo `round_robin` en el cliente. Como el costo dominante de cada lote es el round-trip a PostgreSQL, se queda en HTTP y se decide en la fase 6 con el benchmark; si se adopta, va solo en el rol `funds`, y el cambio queda en `store/walletclient` y en `handlers/apply-funds-batch`.

## Risks / Trade-offs

- **[El round-trip a WalletService limita el throughput]** → Lote por poll. El benchmark de la fase 6 es el criterio de salida.
- **[El reinicio tarda más a medida que crece el log]** → Aceptado hasta los snapshots de la fase 7.
- **[Reservas huérfanas si una orden nunca llega a tener evento]** → No ocurre: la reserva y el evento salen del mismo comando, y si el engine se cae antes de publicar, al reiniciar publica los eventos que faltaron (D6).

## Migration Plan

Se despliega WalletService y luego el engine, que consume desde el offset 0 todos los comandos acumulados en las fases 1–2. Es un entorno de desarrollo: al pasar a la fase 4 se recrea desde cero (ver el Migration Plan de `04-order-matching`). Las órdenes de personas sin saldo terminan en `OrderRejected/insufficient_funds`. Rollback: detener el engine; los comandos quedan en el log y las reservas son idempotentes.

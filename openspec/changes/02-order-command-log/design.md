# Design

## Context

La fase 1 dejó OrderService creando y consultando órdenes en la tabla `orders`, con un paquete por ruta en `handlers/` que llama a `store/orderdb`. Esta fase introduce los comandos, agrega modificar y cancelar como solicitudes, y hace del log durable y ordenado por book el registro principal. Motivación en `proposal.md`.

## Goals / Non-Goals

**Goals:**

- Que cada operación aceptada sobre una orden quede como un comando durable en el log, en orden, antes de responder.
- Que la tabla `orders` sea una proyección del log: se llena consumiendo `orders.commands`.

**Non-Goals:**

- Exactly-once con transacciones de Kafka. Cada comando lleva un `commandId` único: una entrega repetida del mismo comando (un reintento del producer, una relectura del consumidor) conserva su `commandId` y se descarta. Dos peticiones sobre la misma orden, como dos `DELETE` o dos `PATCH`, son comandos distintos: comparten el `orderId` pero cada uno tiene su `commandId`, y quien los procesa decide cada uno.

## Decisions

### D1. Comandos y envelope

`shared/events` define `NewOrder`, `ModifyOrder` y `CancelOrder` con un envelope común:

- `commandId` (UUIDv7) y `type`;
- `book`, `userId` y `orderId`;
- `schemaVersion` y `acceptedAt` (el timestamp en que OrderService aceptó la operación);
- el payload propio de cada tipo. El de `NewOrder` lleva todos los campos de la orden, para que el consumidor la guarde sin otra fuente.

El `commandId` se genera una sola vez, antes de publicar. Los reintentos del producer reenvían el mismo record, así que conservan el mismo `commandId`.

### D2. Publicación directa al log

Los handlers publican el comando en `orders.commands` y esperan la confirmación del log (`acks=all`) antes de responder. No escriben en la base.

- **`POST`:** valida, arma la orden en `PENDING` (id UUIDv7, `createdAt`) y publica su `NewOrder`. Con el ack responde `201` con la orden, como en la fase 1. Si el log no confirma, responde `503` y no queda nada: ni orden ni comando.
- **`PATCH`:** lee la orden de `orders` para validar que es propia, limit y que el body aplica, publica el `ModifyOrder` y responde `202` con `orderId` y `commandId`. La orden no cambia.
- **`DELETE`:** lee la orden para validar que es propia, publica el `CancelOrder` y responde `202` con `orderId` y `commandId`. La orden no cambia. Un `DELETE` repetido publica otro `CancelOrder`: el engine (fase 3) cancela con el primero y rechaza los siguientes con `OrderCancelRejected`, porque la orden ya es final. Un duplicado no libera fondos dos veces: la orden ya no está en el book y la wallet rechaza liberar más que lo reservado.
- **Por qué OrderService no decide el resultado:** una modificación o cancelación puede competir con un cruce de la misma orden. Solo quien aplica los comandos de un book en orden sabe cuál ocurrió primero, así que OrderService registra la intención y el resultado llega después. `202` significa "registrada en el log", no "cancelada".
- **Por qué una sola escritura:** "aceptado" significa "durable en el log". Si además se escribiera la base en la misma petición, una caída entre las dos escrituras dejaría órdenes sin comando o comandos sin orden. Escribiendo solo el log, el ack es la única fuente de verdad, y `orders` se reconstruye desde él (D4).
- **Alternativa descartada:** transactional outbox (orden y comando en una transacción de PostgreSQL y un relay que publica después). Da consistencia inmediata en la lectura, pero son dos escrituras más un proceso de relay, cuando el log ya es durable y ordenado.
- **Alternativa descartada:** un bloqueo en Redis para no publicar dos `CancelOrder`. Vuelve a ser una doble escritura (si la publicación falla después del bloqueo, la cancelación se pierde) y suma un servicio, para evitar un duplicado que el engine ya descarta sin efectos.

### D3. Redpanda con dos topics y partición explícita por book

- **`orders.commands`:** key = book. Lo escriben solo los handlers de OrderService.
- **`orders.events`:** se crea ahora, pero se usa desde la fase 3.
- **Partición explícita:** `shared/topics` tiene los nombres de los topics y un registro estático `book → partition` (`BRL-VIB → 0`), y el producer usa un partitioner manual que la toma de ahí. La partición es un detalle del log, no del book: `shared/books` define los mercados y `shared/topics` lo usa, solo con la librería estándar. El partitioner de `franz-go` vive en `store/commandlog`. Así quien lea un book sabe exactamente qué partición leer, sin depender del algoritmo de hash del cliente.
- **Producer:** `franz-go`, con `acks=all`, idempotent producer (el broker descarta los reintentos repetidos y conserva el orden por partición) y reintentos con backoff dentro del timeout de la petición. Se elige por su partitioner manual y su idempotent producer; el publisher de Gofr no permite elegir la partición.
- **Retención:** infinita en el MVP.
- **Alternativa descartada:** NATS JetStream o Redis Streams. Redpanda da la API de Kafka y un log particionado, con poca operación local.

### D4. Consumidor de `orders.commands`

- **Qué es:** un subscriber de Gofr (`app.Subscribe`) en `handlers/order-commands` (el handler del topic `orders.commands`, como `create-order` lo es de `POST /orders`), con su propio consumer group. Gofr registra un subscriber por topic, así que un solo handler recibe los tres tipos de comando. Es otra entrada del servicio, así que sigue `.claude/standards/handlers.md`.
- **`NewOrder`:** guarda la orden con `INSERT … ON CONFLICT (id) DO NOTHING` en `store/orderdb`. Una entrega repetida no tiene efecto.
- **`ModifyOrder` y `CancelOrder`:** no cambian la orden en esta fase; sus resultados llegan con los eventos del engine (fases 3 a 5).
- **Commit del offset:** después de guardar. Si la base falla, el subscriber devuelve error, no avanza el offset y reintenta: at-least-once, que el insert idempotente vuelve efectivamente una sola vez.
- **Por qué el mismo servicio publica y consume:** OrderService cumple dos roles. Los handlers HTTP escriben (validan y publican el comando; su única fuente de verdad es el log) y el consumidor lee (arma `orders` desde el log, como cualquier otro consumidor). `orders` es una proyección: se puede borrar y reconstruir leyendo `orders.commands` desde el offset 0. Que los dos roles vivan en el mismo binario es una decisión de despliegue; `order-commands` puede correr como un proceso aparte con el mismo código.
- **Alternativa descartada:** construir `orders` desde `orders.events`, creando la fila con el `OrderAccepted` u `OrderRejected` del engine. Dejaría un solo consumidor, pero en esta fase todavía no hay engine, y desde la fase 3 una orden aceptada con `201` solo aparecería cuando el engine y la wallet la procesen, y no aparecería si el engine está caído.
- **Lectura eventual:** una orden aceptada aparece en `GET /orders` cuando el consumidor la guarda, normalmente milisegundos después del `201`. Un `GET`, `PATCH` o `DELETE` inmediato puede responder `404` mientras tanto. Se documenta en `docs/api.md`.

### D5. Observabilidad

`order_projection_lag_seconds` = ahora menos `acceptedAt` del último comando guardado por el consumidor, expuesta en Prometheus vía Gofr. Crece si el consumidor se detiene o se atrasa.

### D6. Estructura de OrderService

| Paquete | Contenido | Responsabilidad |
| --- | --- | --- |
| `shared/events` | `commands.go` | Tipos `NewOrder`, `ModifyOrder`, `CancelOrder` y su envelope (D1), con serialización JSON |
| `shared/topics` | `topics.go` | Nombres de los topics y `Partition(book)` desde el registro `book → partition` (D3) |
| `handlers/create-order` | `handler.go` (cambia) | Arma la orden y su `NewOrder`, lo publica y responde `201`, o `503` si el log no confirma (D2) |
| `handlers/modify-order`, `handlers/cancel-order` | `handler.go` (cambian) | Mismo `request` y validaciones que en la fase 1; publican `ModifyOrder` / `CancelOrder` y responden `202` con `orderId` y `commandId` |
| `handlers/order-commands` | `handler.go` (nuevo) | Subscriber de `orders.commands`: decodifica el comando y guarda cada `NewOrder` con `orderdb`; actualiza `order_projection_lag_seconds` (D4, D5) |
| `store/commandlog` | `producer.go` (nuevo), mock | Producer `franz-go` hacia `orders.commands` con `acks=all`, idempotent producer y un partitioner que usa `topics.Partition`; un `InitMock(t)` que registra lo publicado y puede fallar, como el de `orderdb` |
| `store/orderdb` | `postgres.go` (cambia) | `InsertOrder` idempotente con `ON CONFLICT (id) DO NOTHING` |
| `models` | `errors.go` (cambia) | `ErrOrderNotModifiable` para modificar una orden market |

- No hay migraciones nuevas: `orders` no cambia.
- El producer se crea al arrancar en `main.go`, que también registra el subscriber.
- **Por qué en `shared/`:** el envelope de comandos y el mapa de particiones son el contrato entre OrderService (productor y consumidor) y el engine (consumidor desde la fase 3), y una vez definidos casi no cambian.

## Risks / Trade-offs

- **[Lectura eventual tras el `POST`]** → El `201` ya devuelve la orden completa. Un `GET`, `PATCH` o `DELETE` inmediato puede responder `404` unos milisegundos; se documenta y la métrica de D5 muestra el atraso.
- **[El consumidor se detiene]** → Las órdenes aceptadas siguen en el log y aparecen cuando el consumidor vuelve, desde su último offset. `order_projection_lag_seconds` lo alerta.
- **[Un `POST` cuyo ack se pierde]** → El cliente recibe un error aunque el comando haya quedado en el log, y la orden aparece después. Un reintento del cliente crea otra orden, igual que en la fase 1 (sin idempotencia en la API de órdenes).
- **[`CancelOrder` duplicados]** → Sin efecto en fondos ni estado (D2). Nota para la fase 5: la proyección no debería registrar como "último rechazo" un `OrderCancelRejected` sobre una orden ya `CANCELLED`, para que un `DELETE` repetido no deje un rechazo visible.

## Migration Plan

Se agregan Redpanda y Redpanda Console (`http://localhost:8081`) a compose y se corre el job de init. Antes de desplegar OrderService con la publicación y el consumidor, se vacía la tabla `orders` de desarrollo, porque las órdenes de la fase 1 no tienen comando y no se podrían reconstruir desde el log. Rollback: volver a la versión de la fase 1; los comandos quedan en el log.

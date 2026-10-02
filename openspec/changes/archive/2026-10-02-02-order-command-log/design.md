# Design

## Context

La fase 1 dejó OrderService creando y consultando órdenes en la tabla `orders`, con un paquete por ruta en `handlers/` que llama a `store/orderdb`. Esta fase introduce los comandos, publica modificar y cancelar como comandos que decide el engine, y hace del log durable y ordenado por book el registro principal. Motivación en `proposal.md`.

## Goals / Non-Goals

**Goals:**

- Que cada operación aceptada sobre una orden quede como un comando durable en el log, en orden, antes de responder.
- Que la tabla `orders` sea una proyección del log: se llena consumiendo `orders.commands`.

**Non-Goals:**

- Exactly-once con transacciones de Kafka. Cada mensaje lleva un `id` único: una entrega repetida del mismo comando (un reintento del producer, una relectura del consumidor) conserva su `id` y se descarta. Dos llamados sobre la misma orden, como dos `close` o dos `change`, son comandos distintos: comparten el `orderId` pero cada uno tiene su `id`, y quien los procesa decide cada uno.

## Decisions

### D1. Mensajes y envelope

`shared/eventlog/events` es el contrato del log: un solo envelope, `Message`, para todo mensaje de todo topic, y el catálogo de topics, tipos y payloads (`orders.go`). `producer` publica y `consumer` lee; ninguno de los dos conoce los payloads. El envelope lleva:

- `id` (UUIDv7) y `route`, `<topic>.<tipo>` (`orders.commands.NewOrder`): el único nombre del mensaje. Se publica en el topic de su ruta y su handler se suscribe a la ruta;
- `book`, del que sale la partición: todo mensaje del exchange es de un book;
- `schemaVersion` y `createdAt` (el timestamp en que OrderService aceptó la operación);
- el `payload` propio de cada tipo, que el handler decodifica con `ParsePayload`. Los de órdenes llevan `orderId` y `userId`; el de `NewOrder`, además, todos los campos de la orden, para que el consumidor la guarde sin otra fuente.

El `id` se genera una sola vez, antes de publicar. Los reintentos del producer reenvían el mismo record, así que conservan el mismo `id`. El catálogo solo declara las rutas (`RouteNewOrder`, `RouteModifyOrder`, `RouteCancelOrder`); el topic sale de la ruta con `GetTopic`.

### D2. Publicación directa al log

Los handlers publican el comando en `orders.commands` y esperan la confirmación del log (`acks=all`) antes de responder. No escriben en la base.

- **`POST`:** valida, arma la orden en `PENDING` (id UUIDv7, `createdAt`) y publica su `NewOrder`. Con el ack responde `201` con la orden, como en la fase 1. Si el log no confirma, responde `503` y no queda nada: ni orden ni comando.
- **`POST /orders/{id}/change`:** lee la orden de `orders` para validar que es propia y limit, y que el body aplica; publica el `ModifyOrder` y responde `201` con la orden tal como está (D7). La orden no cambia.
- **`POST /orders/{id}/close`:** lee la orden para validar que es propia, publica el `CancelOrder` y responde `201` con la orden tal como está. La orden no cambia. Un `close` repetido publica otro `CancelOrder`: el engine (fase 3) cancela con el primero y rechaza los siguientes con `OrderCancelRejected`, porque la orden ya es final. Un duplicado no libera fondos dos veces: la orden ya no está en el book y la wallet rechaza liberar más que lo reservado.
- **Por qué OrderService no decide el resultado:** una modificación o cancelación puede competir con un cruce de la misma orden. Solo quien aplica los comandos de un book en orden sabe cuál ocurrió primero, así que OrderService publica la intención y el resultado llega después con los eventos del engine, que la fase 5 proyecta sobre la orden. Por eso modificar y cancelar no son `PATCH` ni `DELETE` sobre la orden, que prometen que el recurso cambió.
- **Por qué una sola escritura:** "aceptado" significa "durable en el log". Si además se escribiera la base en la misma petición, una caída entre las dos escrituras dejaría órdenes sin comando o comandos sin orden. Escribiendo solo el log, el ack es la única fuente de verdad, y `orders` se reconstruye desde él (D4).
- **Alternativa descartada:** transactional outbox (orden y comando en una transacción de PostgreSQL y un relay que publica después). Da consistencia inmediata en la lectura, pero son dos escrituras más un proceso de relay, cuando el log ya es durable y ordenado.
- **Alternativa descartada:** un bloqueo en Redis para no publicar dos `CancelOrder`. Vuelve a ser una doble escritura (si la publicación falla después del bloqueo, la cancelación se pierde) y suma un servicio, para evitar un duplicado que el engine ya descarta sin efectos.

### D3. Redpanda con dos topics y partición explícita por book

- **`orders.commands`:** key = book. Lo escriben solo los handlers de OrderService.
- **`orders.events`:** se crea ahora, pero se usa desde la fase 3.
- **Partición explícita:** cada book del registro de `shared/books` tiene su partición (`BRL-VIB → 0`), y el producer usa un partitioner manual que la toma de ahí. Así quien lea un book sabe exactamente qué partición leer, sin depender del algoritmo de hash del cliente. Va en el mismo registro porque todo book configurado es operable: un segundo mapa solo podría desincronizarse. `shared/eventlog` tiene los nombres de los topics.
- **Producer:** `franz-go`, con `acks=all`, idempotent producer (el broker descarta los reintentos repetidos y conserva el orden por partición) y reintentos con backoff dentro del timeout de la petición. Se elige por su partitioner manual y su idempotent producer; el publisher de Gofr no permite elegir la partición.
- **Retención:** infinita en el MVP.
- **Alternativa descartada:** NATS JetStream o Redis Streams. Redpanda da la API de Kafka y un log particionado, con poca operación local.

### D4. Consumidor de `orders.commands`

- **Qué es:** `consumer.Start` (`shared/eventlog/consumer`), un consumidor `franz-go` con un solo cliente y consumer group. Cada handler se suscribe a una ruta `<topic>.<tipo>`, como una ruta HTTP: `consumer.Subscribe(events.RouteNewOrder, insertneworder.Handle)`, con `RouteNewOrder = "orders.commands.NewOrder"`. Lo que sigue al último punto es el tipo y el resto es el topic. El consumidor compara la `route` de cada mensaje con la de cada suscripción y le entrega el `Message`, cuyo payload decodifica el handler; una ruta sin tipo hace fallar `Start`; un tipo sin suscripción se confirma sin aplicarse. El projector solo se suscribe a `NewOrder`; en la fase 5 suma sus suscripciones a `orders.events` en la misma llamada. Arranca con `app.OnStart` en el rol `projector` y se detiene con el contexto de la app; es genérico en el contexto, así devuelve el `*gofr.Context` al handler sin que `shared/` importe Gofr.
- **`NewOrder`:** guarda la orden con `INSERT … ON CONFLICT (id) DO NOTHING` en `store/orderdb`. Una entrega repetida no tiene efecto.
- **`ModifyOrder` y `CancelOrder`:** el projector no se suscribe a ellos, así que el consumidor los confirma sin aplicarlos: la orden cambia cuando la fase 5 proyecta los eventos del engine (`OrderModified`, `OrderCancelled`). No hay tabla de pedidos: `orders` es la única proyección.
- **Commit del offset:** manual, solo después de que el handler guardó el comando. Si la base falla, el consumidor reintenta el mismo comando con backoff y no avanza: at-least-once, que el insert idempotente vuelve efectivamente una sola vez. Un record que no es un comando, o un comando de tipo desconocido, se salta con un log.
- **Por qué el mismo servicio publica y consume:** OrderService cumple dos roles. Los handlers HTTP escriben (validan y publican el comando; su única fuente de verdad es el log) y el consumidor lee (arma `orders` desde el log, como cualquier otro consumidor). `orders` es una proyección: se puede borrar y reconstruir leyendo `orders.commands` desde el offset 0.
- **Dos roles, dos despliegues:** el mismo binario arranca en el rol que dice `ROLE`. `api` registra las rutas y conecta el producer, sin consumir; `projector` solo corre el consumidor y su métrica: sin rutas Gofr no levanta el servidor HTTP, y expone solo `/metrics` en `METRICS_PORT`, que en local debe ser distinto del de la `api`. Se separan porque escalan distinto: la API con el tráfico HTTP y el proyector con las particiones, a lo sumo una instancia por partición. Juntos, cada réplica de la API entraría al consumer group sin partición que leer, cada deploy de la API rebalancearía el grupo y pausaría la proyección, y ponerse al día desde el offset 0 competiría con los requests por CPU y conexiones a PostgreSQL. Es un rol del mismo servicio y no un servicio aparte porque usa `store/orderdb` y `models` de OrderService.
- **Alternativa descartada:** construir `orders` desde `orders.events`, creando la fila con el `OrderAccepted` u `OrderRejected` del engine. Dejaría un solo consumidor, pero en esta fase todavía no hay engine, y desde la fase 3 una orden aceptada con `201` solo aparecería cuando el engine y la wallet la procesen, y no aparecería si el engine está caído.
- **Lectura eventual:** una orden aceptada aparece en `GET /orders` cuando el consumidor la guarda, unos milisegundos después del `201` (`FetchMaxWait` de 100 ms: el broker responde apenas llega un record). Un `GET`, `change` o `close` inmediato puede responder `404` mientras tanto. Se documenta en `docs/api.md`.
- **Alternativa descartada:** el subscriber de Gofr (`app.Subscribe`). Su lector de Kafka fija `MinBytes` en 10 KB sin `MaxWait`, y no se configura: con poco tráfico cada lectura espera hasta 10 s, medido en unos 7 s por orden. Además, cuando el handler falla pasa al mensaje siguiente, y al confirmar ese el anterior queda saltado.

### D5. Observabilidad

`order_projection_lag_seconds` = ahora menos `createdAt` de cada comando, medido cuando el consumidor lo guarda, y expuesto en Prometheus vía Gofr por el `projector`. Muestra cuánto tarda la proyección mientras el consumidor avanza; si se detiene, el gauge queda en su último valor. Un consumidor detenido o atrasado se detecta con el lag del consumer group `order-service` que expone Redpanda (offset final menos offset confirmado), que crece aunque el consumidor no corra.

### D6. Estructura de OrderService

| Paquete | Contenido | Responsabilidad |
| --- | --- | --- |
| `shared/books` | `books.go`, `registry.go` (cambian) | `Partition` en cada book del registro (D3) |
| `shared/eventlog/events` | `events.go`, `orders.go` (nuevos) | El envelope `Message` (D1) con `NewMessage`, `ParsePayload` y `GetTopic`, y el catálogo de órdenes: sus rutas (`RouteNewOrder`, …) y los payloads `NewOrder`, `ModifyOrder` y `CancelOrder` |
| `shared/eventlog/producer` | `producer.go`, `mock.go` (nuevos) | `Connect` y `Publish(ctx, message)`: el topic de la ruta, `acks=all`, idempotent producer, timeout de entrega y la partición del book del mensaje como destino y key (D3); `InitMock(t)`, que reemplaza el producer en los tests de los handlers como el mock de `orderdb` |
| `shared/eventlog/consumer` | `consumer.go`, `subscription.go` (nuevos) | `Start` y `Subscribe`, el consumidor de D4: una suscripción por ruta `<topic>.<tipo>`, cada record entregado como `Message`, commit tras aplicar y reintento |
| `handlers/create-order` | `handler.go` (cambia) | Arma la orden y su `NewOrder`, lo publica y responde `201`, o `503` si el log no confirma (D2) |
| `handlers/change-order`, `handlers/close-order` | `handler.go` (reemplazan a `modify-order` y `cancel-order`) | Mismo `request` y validaciones que en la fase 1; publican `ModifyOrder` / `CancelOrder` y responden `201` con la orden sin cambios (D7) |
| `handlers/insert-new-order` | `handler.go` (nuevo) | Handler de los `NewOrder` de `orders.commands`: guarda cada orden en `orders` y actualiza `order_projection_lag_seconds` (D4, D5) |
| `store/orderdb` | `postgres.go` (cambia) | `InsertOrder` idempotente con `ON CONFLICT (id) DO NOTHING` |
| `models` | `errors.go` (cambia) | `ErrOrderNotModifiable` para modificar una orden market |

- `orders` no cambia y no hay migraciones nuevas.
- `main.go` elige el rol con `ROLE`: `api` llama a `producer.Connect` y registra las rutas; `projector` registra el gauge y lanza `consumer.Start` en `app.OnStart`. Un rol desconocido termina el proceso al arrancar. La configuración es `ROLE`, `COMMAND_LOG_BROKERS` y `COMMAND_LOG_GROUP`.
- **Por qué en `shared/`:** el envelope de comandos y el mapa de particiones son el contrato entre OrderService (productor y consumidor) y el engine (consumidor desde la fase 3), y una vez definidos casi no cambian.
- **Por qué un envelope común:** con un solo `Message`, `producer` y `consumer` no conocen los payloads ni necesitan un tipo por familia de mensajes; cada handler decodifica el payload que espera. Los eventos del engine (fase 3) se agregan al catálogo de `shared/eventlog/events` como otro archivo, con el mismo envelope. `franz-go` es la única dependencia, justificada en D3.

### D7. Modificación y cancelación asíncronas

- **Respuesta:** `201` con la orden tal como está, el formato que Gofr da a un `POST` con datos. No se devuelve el `id` del comando ni se crea un recurso de pedido: el resultado se ve en la propia orden cuando la fase 5 proyecta los eventos del engine.
- **Por qué `POST` y no `PATCH`/`DELETE`:** la respuesta no puede asegurar el cambio, porque lo decide el engine.
- **Alternativa descartada:** un recurso de pedido (`order_requests` y `GET /orders/{id}/requests/{requestId}`) con su propio estado. Agrega una segunda proyección y una ruta que en esta fase siempre diría `PENDING`; la orden ya es la proyección de lo que pasó.
- **Alternativa descartada:** que `DELETE` y `PATCH` esperen el evento del engine antes de responder. Ataría la latencia de la API al engine y a la wallet, un timeout sería ambiguo y en esta fase todavía no hay engine.

## Risks / Trade-offs

- **[Lectura eventual tras el `POST`]** → El `201` ya devuelve la orden completa. Un `GET`, `change` o `close` inmediato puede responder `404` unos milisegundos; se documenta y la métrica de D5 muestra el atraso.
- **[El consumidor se detiene]** → Las órdenes aceptadas siguen en el log y aparecen cuando el consumidor vuelve, desde su último offset. `order_projection_lag_seconds` lo alerta.
- **[Un `POST` cuyo ack se pierde]** → El cliente recibe un error aunque el comando haya quedado en el log, y la orden aparece después. Un reintento del cliente crea otra orden, igual que en la fase 1 (sin idempotencia en la API de órdenes).
- **[`CancelOrder` duplicados]** → Sin efecto en fondos ni estado (D2). El engine rechaza los siguientes con `OrderCancelRejected` y la orden no cambia.

## Migration Plan

Se agregan Redpanda y Redpanda Console (`http://localhost:8081`) a compose y se corre el job de init. Antes de desplegar OrderService con la publicación y el consumidor, se vacía la tabla `orders` de desarrollo, porque las órdenes de la fase 1 no tienen comando y no se podrían reconstruir desde el log. Rollback: volver a la versión de la fase 1; los comandos quedan en el log.

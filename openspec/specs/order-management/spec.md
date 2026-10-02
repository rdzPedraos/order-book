# order-management Specification

## Purpose

Es la API pública con la que cada persona envía, modifica, cancela y consulta sus órdenes, y la vista consultable de su ciclo de vida y de los trades que produjeron.

## Requirements

### Requirement: Creación de órdenes
`POST /orders` MUST recibir `book`, `side` y, según la orden, `limit`, `quantity` o `amount`; validar la forma, generar un `orderId` en el servidor, guardar la orden con `status = PENDING` y responder `201` con la orden creada. El tipo de orden no se envía: es `LIMIT` si viene `limit` y `MARKET` si no, y la respuesta lo informa en `type`.

#### Scenario: Orden limit creada
- **WHEN** la persona envía `{"book":"BRL-VIB","side":"BUY","limit":"90.00","quantity":"10"}`
- **THEN** el sistema responde `201` con un `orderId` nuevo, `type = LIMIT`, `status = PENDING`, `filledQuantity = "0"` y `avgPrice = null`

#### Scenario: Envío repetido
- **WHEN** la persona envía dos veces la misma petición
- **THEN** se crean dos órdenes distintas, cada una con su `orderId`

#### Scenario: Base de datos no disponible
- **WHEN** la persistencia falla al recibir `POST /orders`
- **THEN** el sistema responde `503` y no queda ninguna orden guardada

### Requirement: Identificación y aislamiento
Todo endpoint de órdenes MUST identificar a la persona mediante el header `X-User-ID` y MUST rechazar la petición si el header falta. Una persona solo puede ver y operar sus propias órdenes: indicar el ID de una orden ajena se trata igual que una orden inexistente.

#### Scenario: Header ausente
- **WHEN** se llama a `GET /orders` sin `X-User-ID`
- **THEN** el sistema responde `401` con código `missing_user_id`

#### Scenario: Recurso ajeno
- **WHEN** la persona A consulta `GET /orders/{id}` de una orden de la persona B
- **THEN** el sistema responde `404`, sin revelar que la orden existe

### Requirement: Validación de forma
El sistema MUST rechazar con `400`, sin crear la orden, toda petición con: book desconocido; `side` distinto de `BUY`/`SELL`; `limit` con más decimales que la moneda del precio o ≤ 0; `quantity` no entera o menor a 1; `amount` con más decimales que la moneda o ≤ 0; o una combinación de campos distinta de estas: con `limit`, `quantity` (compra o venta); sin `limit`, `amount` para comprar y `quantity` para vender. Una combinación no permitida se responde con `unsupported_order`.

#### Scenario: Limit sin cantidad
- **WHEN** se envía `{"book":"BRL-VIB","side":"SELL","limit":"95.00"}`
- **THEN** el sistema responde `400` con código `unsupported_order`

#### Scenario: Limit con monto
- **WHEN** se envía `{"book":"BRL-VIB","side":"BUY","limit":"90.00","amount":"500.00"}`
- **THEN** el sistema responde `400` con código `unsupported_order`

#### Scenario: Venta market por monto
- **WHEN** se envía `{"book":"BRL-VIB","side":"SELL","amount":"500.00"}`
- **THEN** el sistema responde `400` con código `unsupported_order`

#### Scenario: Cantidad fraccionada
- **WHEN** se envía `{"book":"BRL-VIB","side":"BUY","limit":"90.00","quantity":"2.5"}`
- **THEN** el sistema responde `400` con código `invalid_quantity`

#### Scenario: Book desconocido
- **WHEN** se envía una orden con `book = "BTC-USD"`
- **THEN** el sistema responde `400` con código `unknown_book`

#### Scenario: Market buy por monto
- **WHEN** se envía `{"book":"BRL-VIB","side":"BUY","amount":"500.00"}`
- **THEN** el sistema responde `201` con `type = MARKET`, `amount = "500.00"` y `status = PENDING`

### Requirement: Identificador del book
Solo se aceptan los books configurados, por su identificador tal como está configurado (en el MVP, `BRL-VIB`). La API MUST aceptar el identificador sin distinguir mayúsculas y MUST guardar y devolver siempre el identificador configurado. Un identificador con los tickers en otro orden es un book desconocido. El identificador no define cuál moneda es la base: en `BRL-VIB` se opera VIB con precio en BRL.

#### Scenario: Minúsculas
- **WHEN** la persona crea una orden con `book = "brl-vib"`
- **THEN** el sistema responde `201` y la orden muestra `book = "BRL-VIB"`

#### Scenario: Tickers en otro orden
- **WHEN** la persona crea una orden con `book = "VIB-BRL"`
- **THEN** el sistema responde `400` con código `unknown_book`

#### Scenario: Filtro en minúsculas
- **WHEN** la persona llama a `GET /orders?book=brl-vib`
- **THEN** recibe sus órdenes del book `BRL-VIB`

### Requirement: Listado y detalle de órdenes
`GET /orders` MUST listar solo las órdenes de la persona, más recientes primero, con filtros por `status`, `side` y `book` combinables con la paginación: `cursor` (un `orderId`; devuelve las órdenes que siguen a esa en el listado, sin incluirla) y `limit` (de 1 a 100, 20 por defecto). La respuesta MUST incluir en `metadata.nextCursor` el valor a usar como `cursor` en la página siguiente, o `null` si no hay más órdenes. Cada orden MUST mostrar: `orderId`, `book`, `side`, `type`, `limit` o `amount`, cantidad original, cantidad ejecutada (0 mientras no haya ejecuciones), cantidad pendiente, precio promedio ejecutado (`null` mientras no haya ejecuciones), `status`, `createdAt` y `updatedAt`.

#### Scenario: Filtro por estado
- **WHEN** la persona llama a `GET /orders?status=PENDING`
- **THEN** recibe solo sus órdenes en `PENDING`

#### Scenario: Página siguiente con filtros
- **WHEN** la persona tiene 25 órdenes `PENDING` de `BUY`, pide `GET /orders?status=PENDING&side=BUY&limit=20` y luego la misma consulta con `cursor` igual al `metadata.nextCursor` recibido
- **THEN** la primera respuesta trae 20 órdenes y un `metadata.nextCursor`, y la segunda las 5 restantes con `metadata.nextCursor = null`, sin repetir ninguna

#### Scenario: Límite fuera de rango
- **WHEN** la persona pide `GET /orders?limit=500`
- **THEN** el sistema responde `400` con código `invalid_limit`

#### Scenario: Detalle propio
- **WHEN** la persona llama a `GET /orders/{id}` sobre una orden suya
- **THEN** recibe todos los campos de la orden

### Requirement: Comando de creación
Al crear una orden, OrderService MUST publicar su comando `NewOrder` en el log y esperar su confirmación antes de responder. La orden existe desde que su `NewOrder` está en el log: no puede existir una orden sin su comando ni un comando sin su orden.

#### Scenario: Orden con su comando
- **WHEN** la persona crea una orden limit de 10 VIB @ 90.00
- **THEN** el sistema responde `201` con la orden en `PENDING` y el log tiene un `NewOrder` para ese `orderId`

#### Scenario: Log no disponible al crear
- **WHEN** el log no confirma el `NewOrder`
- **THEN** el sistema responde `503` y no queda ni orden ni comando

### Requirement: Registro de órdenes desde el log
OrderService MUST guardar en su consulta de órdenes cada orden cuyo `NewOrder` está en el log, consumiendo los comandos en orden. Procesar más de una vez el mismo comando MUST NOT duplicar la orden, y `ModifyOrder` y `CancelOrder` MUST NOT cambiarla: su resultado llega después, como evento del engine. La consulta es eventualmente consistente: una orden aceptada aparece en `GET /orders` y `GET /orders/{id}` después de que el consumidor la guarda.

#### Scenario: Orden visible tras registrarse
- **WHEN** la persona crea una orden y el consumidor procesa su `NewOrder`
- **THEN** `GET /orders/{id}` devuelve la orden en `PENDING`

#### Scenario: Comando entregado dos veces
- **WHEN** el consumidor recibe dos veces el mismo `NewOrder`
- **THEN** existe una sola orden con ese `orderId`

#### Scenario: Evento antes que el comando
- **WHEN** el consumidor lee el `OrderAccepted` de una compra limit antes que su `NewOrder`
- **THEN** la orden queda en `OPEN` con sus datos, y el `NewOrder` que llega después no la cambia

### Requirement: Modificación asíncrona
`POST /orders/{id}/change` MUST aceptar un nuevo `limit`, un nuevo `quantity` pendiente (≥ 1) o ambos, solo para órdenes limit propias, aplicar las mismas reglas de validación que la creación, publicar un comando `ModifyOrder` y responder `201` con la orden tal como está. OrderService MUST NOT cambiar la orden: el resultado lo decide quien procesa el comando.

#### Scenario: Cambio de limit publicado
- **WHEN** la persona pide cambiar el `limit` de su orden de 90.00 a 92.00
- **THEN** el sistema responde `201` con la orden en `limit = 90.00` y el log tiene un `ModifyOrder` con `limit = 92.00`

#### Scenario: Orden market
- **WHEN** la persona intenta modificar una orden market
- **THEN** el sistema responde `409` con código `order_not_modifiable` y no publica ningún comando

### Requirement: Cancelación asíncrona
`POST /orders/{id}/close` MUST publicar un comando `CancelOrder` para una orden propia y responder `201` con la orden tal como está. OrderService MUST NOT cambiar el estado de la orden: la cancelación puede competir con una ejecución en curso, y el resultado lo decide quien procesa los comandos en orden. Cada llamado publica su propio `CancelOrder`; los que llegan sobre una orden ya final los rechaza quien procesa los comandos, sin efectos.

#### Scenario: Cancelación publicada
- **WHEN** la persona pide cancelar su orden en `PENDING`
- **THEN** el sistema responde `201` con la orden en `PENDING` y el log tiene un `CancelOrder`

#### Scenario: Cancelación repetida
- **WHEN** la persona envía dos veces `POST /orders/{id}/close` sobre la misma orden
- **THEN** ambas responden `201` con la orden en `PENDING` y el log tiene dos `CancelOrder` con `id` distintos

### Requirement: Ciclo de vida de la orden
Una orden MUST evolucionar solo por las transiciones `PENDING → OPEN`, `PENDING → REJECTED`, `PENDING → FILLED`, `PENDING → CANCELLED`, `OPEN → PARTIALLY_FILLED`, `OPEN → FILLED`, `OPEN → CANCELLED`, `PARTIALLY_FILLED → FILLED` y `PARTIALLY_FILLED → CANCELLED`. `FILLED`, `CANCELLED` y `REJECTED` son finales. Una orden queda `FILLED` cuando la suma de sus trades alcanza su cantidad, y `PARTIALLY_FILLED` mientras sea menor. Una orden market nunca queda en `OPEN` ni `PARTIALLY_FILLED`: sigue en `PENDING` hasta que se llena o se cancela su remanente. Una orden `REJECTED` o `CANCELLED` MUST exponer en `reason` el motivo que informó el engine.

#### Scenario: Rechazo por fondos
- **WHEN** se aplica `OrderRejected` con `reason = insufficient_funds`
- **THEN** la orden pasa a `REJECTED` con ese `reason` y sin trades

#### Scenario: Market con remanente
- **WHEN** una market buy de R$ 500 solo gasta R$ 410
- **THEN** la orden termina en `CANCELLED` con R$ 410 ejecutados y `reason = no_liquidity`

#### Scenario: Limit llenada al llegar
- **WHEN** una limit cruza por completo en el mismo comando en que entra
- **THEN** la orden termina en `FILLED`

#### Scenario: Limit llenada en partes
- **WHEN** una compra limit de 10 VIB recibe un trade de 4 VIB y luego otro de 6 VIB
- **THEN** la orden pasa a `PARTIALLY_FILLED` con 4 VIB ejecutados y luego a `FILLED` con 10

### Requirement: Proyección eventualmente consistente
El estado de las órdenes MUST actualizarse aplicando los eventos del engine en el orden de su partición, de forma idempotente: un evento repetido MUST NOT cambiar la orden otra vez.

#### Scenario: Evento duplicado
- **WHEN** el mismo `TradeExecuted` llega dos veces
- **THEN** la cantidad ejecutada de la orden aumenta una sola vez

#### Scenario: Consulta inmediata
- **WHEN** la persona consulta una orden justo después de crearla
- **THEN** puede verla en `PENDING` hasta que se aplique el evento del engine

### Requirement: Resultado de modificaciones y cancelaciones
Los precios, cantidades y la cantidad pendiente de una orden MUST reflejar cada `OrderModified` aplicado. Un `ModifyOrder` o `CancelOrder` que el engine no pudo aplicar no produce ningún evento, así que la orden MUST conservar su estado.

#### Scenario: Modificación aplicada
- **WHEN** se aplica `OrderModified` con `limit = 92.00` sobre una orden en `limit = 90.00`
- **THEN** la orden muestra `limit = 92.00`

#### Scenario: Cancelación que llega tarde
- **WHEN** la orden se llena por completo antes de que el engine procese su cancelación
- **THEN** la orden queda en `FILLED` y no cambia nada más

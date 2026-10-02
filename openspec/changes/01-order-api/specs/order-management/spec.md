# Spec Delta

## Purpose

Es la API pública con la que cada persona envía, modifica, cancela y consulta sus órdenes, y la vista consultable de su ciclo de vida y de los trades que produjeron.

## ADDED Requirements

### Requirement: Creación de órdenes
`POST /orders` MUST recibir `book`, `side` y, según la orden, `limit`, `quantity` o `amount`; validar la forma, generar un `orderId` en el servidor, guardar la orden con `status = PENDING` y responder `201` con la orden creada. El tipo de orden no se envía: es `LIMIT` si viene `limit` y `MARKET` si no, y la respuesta lo informa en `type`.

#### Scenario: Orden limit creada
- **WHEN** la persona envía `{"book":"BRL-VIB","side":"BUY","limit":"90.00","quantity":10}`
- **THEN** el sistema responde `201` con un `orderId` nuevo, `type = LIMIT`, `status = PENDING`, `filledQuantity = 0` y `avgPrice = null`

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
El sistema MUST rechazar con `400`, sin crear la orden, toda petición con: book desconocido; `side` distinto de `BUY`/`SELL`; `limit` con más decimales que la moneda del precio o ≤ 0; `quantity` no entera o menor a 1; `amount` con más decimales que la moneda o ≤ 0; o una combinación de campos distinta de estas: con `limit`, `quantity` (compra o venta); sin `limit`, `amount` para comprar y `quantity` para vender.

#### Scenario: Limit sin cantidad
- **WHEN** se envía `{"book":"BRL-VIB","side":"SELL","limit":"95.00"}`
- **THEN** el sistema responde `400` con código `invalid_quantity`

#### Scenario: Limit con monto
- **WHEN** se envía `{"book":"BRL-VIB","side":"BUY","limit":"90.00","amount":"500.00"}`
- **THEN** el sistema responde `400` con código `invalid_amount`

#### Scenario: Venta market por monto
- **WHEN** se envía `{"book":"BRL-VIB","side":"SELL","amount":"500.00"}`
- **THEN** el sistema responde `400` con código `invalid_amount`

#### Scenario: Cantidad fraccionada
- **WHEN** se envía `{"book":"BRL-VIB","side":"BUY","limit":"90.00","quantity":2.5}`
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

### Requirement: Endpoints de modificación y cancelación reservados
`PATCH /orders/{id}` y `DELETE /orders/{id}` MUST existir con su contrato de entrada: exigen `X-User-ID`, responden `404` si la orden no existe o es ajena, y `PATCH` valida su body (`limit` y/o `quantity`) con las mismas reglas de forma que la creación. Superadas esas validaciones, MUST responder `501` con código `not_implemented`, sin modificar la orden.

#### Scenario: Cancelación aún no disponible
- **WHEN** la persona envía `DELETE /orders/{id}` sobre su orden en `PENDING`
- **THEN** el sistema responde `501` con código `not_implemented` y la orden sigue en `PENDING`

#### Scenario: Modificación con body inválido
- **WHEN** la persona envía `PATCH /orders/{id}` con `quantity = 2.5`
- **THEN** el sistema responde `400` con código `invalid_quantity`

#### Scenario: Modificación de orden ajena
- **WHEN** la persona envía `PATCH /orders/{id}` sobre una orden de otra persona
- **THEN** el sistema responde `404`

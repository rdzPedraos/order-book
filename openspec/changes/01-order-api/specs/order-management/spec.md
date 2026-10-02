# Spec Delta

## Purpose

Es la API pública con la que cada persona envía, modifica, cancela y consulta sus órdenes, y la vista consultable de su ciclo de vida y de los trades que produjeron.

## ADDED Requirements

### Requirement: Creación de órdenes
`POST /orders` MUST validar la forma de la petición, generar un `orderId` en el servidor, guardar la orden con `status = PENDING` y responder `201` con la orden creada.

#### Scenario: Orden limit creada
- **WHEN** la persona envía `{"book":"BRL-VIB","side":"BUY","type":"LIMIT","price":"90.00","quantity":10}`
- **THEN** el sistema responde `201` con un `orderId` nuevo y `status = PENDING`

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
El sistema MUST rechazar con `400`, sin crear la orden, toda petición con: book desconocido; `side` distinto de `BUY`/`SELL`; `type` distinto de `LIMIT`/`MARKET`; `quantity` no entera o menor a 1; precio con más de 2 decimales o ≤ 0 en limit; precio presente en market; market BUY sin `quoteAmount` > 0; market SELL sin `quantity`.

#### Scenario: Cantidad fraccionada
- **WHEN** se envía una orden limit con `quantity = 2.5`
- **THEN** el sistema responde `400` con código `invalid_quantity`

#### Scenario: Book desconocido
- **WHEN** se envía una orden con `book = "BTC-USD"`
- **THEN** el sistema responde `400` con código `unknown_book`

#### Scenario: Market buy por monto
- **WHEN** se envía `{"book":"BRL-VIB","side":"BUY","type":"MARKET","quoteAmount":"500.00"}`
- **THEN** el sistema responde `201` con `status = PENDING`

### Requirement: Identificador canónico del book
Todo book MUST tener un identificador canónico formado por sus dos tickers en orden alfabético, separados por guion (en el MVP, `BRL-VIB`). La API MUST aceptar el book con los tickers en cualquier orden, sin distinguir mayúsculas, y MUST guardar y devolver siempre el identificador canónico. El nombre no cambia qué moneda es la base: en `BRL-VIB` se opera VIB con precio en BRL.

#### Scenario: Tickers invertidos
- **WHEN** la persona crea una orden con `book = "VIB-BRL"`
- **THEN** el sistema responde `201` y la orden muestra `book = "BRL-VIB"`

#### Scenario: Minúsculas
- **WHEN** la persona crea una orden con `book = "brl-vib"`
- **THEN** la orden muestra `book = "BRL-VIB"`

#### Scenario: Filtro con tickers invertidos
- **WHEN** la persona llama a `GET /orders?book=VIB-BRL`
- **THEN** recibe sus órdenes del book `BRL-VIB`

### Requirement: Listado y detalle de órdenes
`GET /orders` MUST listar solo las órdenes de la persona, más recientes primero, con filtros por `status`, `side` y `book` combinables con la paginación: `cursor` (un `orderId`; devuelve las órdenes que siguen a esa en el listado, sin incluirla) y `limit` (de 1 a 100, 20 por defecto). La respuesta MUST incluir en `metadata.nextCursor` el valor a usar como `cursor` en la página siguiente, o `null` si no hay más órdenes. Cada orden MUST mostrar: `orderId`, `book`, `side`, `type`, precio límite o `quoteAmount`, cantidad original, cantidad ejecutada, cantidad pendiente, precio promedio ejecutado, `status`, `createdAt` y `updatedAt`.

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
`PATCH /orders/{id}` y `DELETE /orders/{id}` MUST existir con su contrato de entrada: exigen `X-User-ID`, responden `404` si la orden no existe o es ajena, y `PATCH` valida su body (`price` y/o `quantity`) con las mismas reglas de forma que la creación. Superadas esas validaciones, MUST responder `501` con código `not_implemented`, sin modificar la orden.

#### Scenario: Cancelación aún no disponible
- **WHEN** la persona envía `DELETE /orders/{id}` sobre su orden en `PENDING`
- **THEN** el sistema responde `501` con código `not_implemented` y la orden sigue en `PENDING`

#### Scenario: Modificación con body inválido
- **WHEN** la persona envía `PATCH /orders/{id}` con `quantity = 2.5`
- **THEN** el sistema responde `400` con código `invalid_quantity`

#### Scenario: Modificación de orden ajena
- **WHEN** la persona envía `PATCH /orders/{id}` sobre una orden de otra persona
- **THEN** el sistema responde `404`

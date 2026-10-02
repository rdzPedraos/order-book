# Spec Delta

## Purpose

Define el contrato durable de comandos y eventos que conecta a los servicios del exchange: cómo se ordenan por book, qué contienen y cómo se consumen sin duplicar efectos.

## ADDED Requirements

### Requirement: Comandos ordenados por book
Los comandos `NewOrder`, `ModifyOrder` y `CancelOrder` MUST publicarse en un log durable usando el `book` como clave de partición, de modo que todos los comandos de un book conserven un orden total. OrderService MUST considerar aceptada una operación solo cuando el log confirmó la escritura de su comando, y responder después de esa confirmación. Una operación sin confirmación MUST NOT dejar efectos.

#### Scenario: Mismo book, misma partición
- **WHEN** se publican comandos para `BRL-VIB` desde varias instancias de OrderService
- **THEN** todos quedan en la misma partición y se leen en un único orden

#### Scenario: Orden preservado por instancia
- **WHEN** una instancia de OrderService acepta, en este orden, `NewOrder` A, `CancelOrder` A y `NewOrder` B
- **THEN** los tres comandos aparecen en el log en ese mismo orden

#### Scenario: Log no disponible
- **WHEN** el log está caído al recibir un `POST /orders`
- **THEN** OrderService responde `503` y no queda ninguna orden

#### Scenario: Caída antes de la confirmación
- **WHEN** OrderService cae después de enviar un comando y antes de recibir la confirmación del log
- **THEN** el cliente no recibe una respuesta de éxito, y la orden existe solo si su comando quedó en el log

### Requirement: Envelope de mensaje
Todo mensaje del log MUST llevar `id` único, su ruta `<topic>.<tipo>`, `book`, versión de esquema, el timestamp en que se creó y su payload; el payload de un comando de órdenes MUST llevar `orderId` y `userId`. Un mismo mensaje publicado más de una vez MUST conservar el mismo `id`.

#### Scenario: Reintento del producer
- **WHEN** el producer reenvía un comando porque no recibió la confirmación del log
- **THEN** el comando queda una sola vez en el log, con su `id` original

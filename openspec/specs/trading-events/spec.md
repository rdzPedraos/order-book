# trading-events Specification

## Purpose

Define el contrato durable de comandos y eventos que conecta a los servicios del exchange: cómo se ordenan por book, qué contienen y cómo se consumen sin duplicar efectos.

## Requirements

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

### Requirement: Envelope de evento
Todo evento MUST publicarse con el `book` como clave, en el envelope común (`id`, ruta `orders.events.<tipo>`, `book`, versión de esquema, `createdAt` y payload), y su payload MUST llevar `sequence`, índice dentro del comando, el `id` del comando que lo produjo y el offset de ese comando en `orders.commands`. El `id` del evento MUST derivarse de forma determinística del `id` del comando y del índice, de modo que un lote reintentado o un reinicio produzcan los mismos IDs.

#### Scenario: Lote reintentado con mismos IDs
- **WHEN** el engine reintenta un lote porque la wallet no respondió
- **THEN** cada evento del lote tiene el mismo `id` que en el intento anterior

### Requirement: Eventos de ciclo de vida
El engine MUST publicar `OrderAccepted`, `OrderRejected`, `OrderCancelled` y `OrderModified`: solo eventos que cambian una orden. `OrderRejected` MUST incluir `reason`. `OrderCancelled` MUST incluir la cantidad cancelada y el monto liberado.

#### Scenario: Rechazo con motivo
- **WHEN** una orden se rechaza por fondos
- **THEN** se publica `OrderRejected` con `orderId`, `userId` y `reason = insufficient_funds`

### Requirement: Eventos de ejecución
Por cada cruce el engine MUST publicar `TradeExecuted` con `tradeId` determinístico, `buyOrderId`, `sellOrderId`, `buyerId`, `sellerId`, `makerSide`, precio, cantidad y monto. Al final del comando MUST publicar un `OrderBookLevelChanged` con el estado absoluto (lado, precio, volumen y cantidad de órdenes) de cada nivel modificado.

#### Scenario: Trade con dos órdenes
- **WHEN** un comando produce un trade que llena por completo la orden en reposo y parcialmente la entrante
- **THEN** se publican `TradeExecuted` con las dos órdenes y la cantidad ejecutada, y después `OrderBookLevelChanged`, con el mismo `sequence`

#### Scenario: Nivel vaciado
- **WHEN** un trade consume la última orden de un nivel
- **THEN** `OrderBookLevelChanged` informa ese nivel con `volume = 0` y `orders = 0`

### Requirement: Consumidores idempotentes
Cada consumidor de eventos MUST mantener su propio progreso y producir el efecto de un evento una sola vez, aunque lo reciba varias veces.

#### Scenario: Entrega duplicada
- **WHEN** WalletService recibe dos veces el mismo `TradeExecuted`
- **THEN** paga el trade una sola vez

### Requirement: Datos de la orden en su primer evento
`OrderAccepted` y `OrderRejected` MUST traer los datos de la orden tal como llegaron en su `NewOrder`: `side`, `type`, `limit`, `quantity` y `amount`, con el `createdAt` del comando en el envelope. Así un consumidor que lee el evento antes que el comando puede crear la orden completa.

#### Scenario: Orden aceptada con sus datos
- **WHEN** el engine acepta una compra limit de 10 VIB @ 90
- **THEN** `OrderAccepted` trae `side = BUY`, `type = LIMIT`, `limit = 9000` y `quantity = 10`

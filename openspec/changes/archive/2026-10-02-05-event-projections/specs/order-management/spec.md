# Spec Delta

## ADDED Requirements

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

## MODIFIED Requirements

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

# Spec Delta

## ADDED Requirements

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

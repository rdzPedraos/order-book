# Spec Delta

## ADDED Requirements

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

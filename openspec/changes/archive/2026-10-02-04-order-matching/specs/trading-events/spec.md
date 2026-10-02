# Spec Delta

## ADDED Requirements

### Requirement: Eventos de ejecución
Por cada cruce el engine MUST publicar `TradeExecuted` con `tradeId` determinístico, `buyOrderId`, `sellOrderId`, `buyerId`, `sellerId`, `makerSide`, precio, cantidad y monto. Al final del comando MUST publicar un `OrderBookLevelChanged` con el estado absoluto (lado, precio, volumen y cantidad de órdenes) de cada nivel modificado.

#### Scenario: Trade con dos órdenes
- **WHEN** un comando produce un trade que llena por completo la orden en reposo y parcialmente la entrante
- **THEN** se publican `TradeExecuted` con las dos órdenes y la cantidad ejecutada, y después `OrderBookLevelChanged`, con el mismo `sequence`

#### Scenario: Nivel vaciado
- **WHEN** un trade consume la última orden de un nivel
- **THEN** `OrderBookLevelChanged` informa ese nivel con `volume = 0` y `orders = 0`

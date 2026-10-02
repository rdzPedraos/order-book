# Spec Delta

## ADDED Requirements

### Requirement: Determinismo
Aplicar la misma secuencia de comandos sobre el mismo estado inicial MUST producir exactamente los mismos eventos, con los mismos identificadores, cantidades y precios. El resultado MUST NOT depender del reloj, de la concurrencia ni del orden de iteración de mapas.

#### Scenario: Replay idéntico
- **WHEN** se reaplican los comandos 1 a N sobre un book vacío
- **THEN** la lista de eventos generada es idéntica a la original en todos sus campos de negocio

### Requirement: Price-time priority
Una orden entrante MUST cruzar primero contra el mejor precio contrario (la venta más baja para una compra, la compra más alta para una venta) y, a igual precio, contra la orden de menor `sequence`. Una compra cruza con una venta si su precio es ≥ al de la venta; una market cruza con cualquier precio disponible. Cada trade es por el mínimo pendiente de ambas órdenes.

#### Scenario: Recorrido del book
- **WHEN** hay ventas de 2 VIB @ 95 (A, seq 10), 3 VIB @ 95 (B, seq 11) y 5 VIB @ 98, y llega una compra limit de 6 VIB @ 100
- **THEN** se ejecutan 2 VIB @ 95 con A, 3 VIB @ 95 con B y 1 VIB @ 98, en ese orden

#### Scenario: Límite que corta el recorrido
- **WHEN** hay ventas @ 95 y @ 105 y llega una compra limit @ 100 por más de lo disponible a 95
- **THEN** solo se ejecuta contra el nivel @ 95 y el remanente queda en el book @ 100

### Requirement: Precio de ejecución y mejora de precio
Cada trade MUST ejecutarse al precio de la orden en reposo (maker). Cuando una compra limit se ejecuta por debajo de su límite, el engine MUST liberar en la wallet la diferencia reservada que ya no necesita, en el mismo lote del trade.

#### Scenario: Mejora de precio
- **WHEN** una compra limit de 10 VIB @ 90 (R$ 900 reservados) ejecuta 4 VIB contra una venta @ 85
- **THEN** se emite un trade de 4 @ 85 por R$ 340 y la wallet libera R$ 20
- **AND** la orden queda en el book con 6 VIB pendientes y R$ 540 reservados para ellos

### Requirement: Ejecución de órdenes market
Una market sell MUST vender su cantidad desde la compra más alta hacia abajo. Una market buy MUST comprar desde la venta más baja hacia arriba, solo en unidades enteras de VIB, mientras el remanente de `amount` alcance para 1 VIB al siguiente precio. La orden termina `FILLED` si se usó todo; si no, aplica la regla de que una market nunca reposa.

#### Scenario: Market buy con sobrante
- **WHEN** llega una market buy de R$ 500 y hay 3 VIB @ 100 y 5 VIB @ 110
- **THEN** se ejecutan 3 @ 100 y 1 @ 110, se liberan R$ 90 y la orden termina cancelada con `reason = no_liquidity`

#### Scenario: Market sell completa
- **WHEN** llega una market sell de 4 VIB y hay compras de 3 VIB @ 100 y 5 VIB @ 99
- **THEN** se ejecutan 3 @ 100 y 1 @ 99 y la orden termina `FILLED`

### Requirement: Self-trade prevention
Una orden MUST NOT ejecutarse contra otra orden de la misma persona. Cuando la siguiente contraparte elegible es propia, el engine MUST cancelar el remanente de la orden entrante con `reason = self_trade_prevented`, conservando los trades ya hechos, liberando su reserva no usada y dejando intacta la orden en reposo.

#### Scenario: Contraparte propia
- **WHEN** la persona P tiene una venta @ 95 en el book y envía una compra limit @ 100 cuya única contraparte es esa venta
- **THEN** la compra termina cancelada con `reason = self_trade_prevented` y la venta sigue en el book

### Requirement: Prioridad al modificar
Un `ModifyOrder` que solo reduce la cantidad MUST conservar el `sequence` de la orden. Cambiar el precio o aumentar la cantidad MUST asignarle un `sequence` nuevo (al final de la fila de su precio) y, si el nuevo precio cruza con el book, la orden MUST ejecutarse como entrante.

#### Scenario: Reducción conserva el lugar
- **WHEN** A (seq 10) y B (seq 11) están @ 95 y A reduce su cantidad
- **THEN** A sigue ejecutándose antes que B

#### Scenario: Cambio de precio pierde el lugar
- **WHEN** A (seq 10) baja su precio de 96 a 95, donde ya está B (seq 11)
- **THEN** A queda detrás de B en el nivel @ 95

#### Scenario: Nuevo precio que cruza
- **WHEN** una compra en reposo @ 90 se modifica a @ 100 y hay ventas @ 95
- **THEN** la compra ejecuta contra las ventas @ 95 al aplicar la modificación

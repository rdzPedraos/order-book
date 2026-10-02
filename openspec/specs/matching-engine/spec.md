# matching-engine Specification

## Purpose

Es la autoridad sobre el Order Book activo de cada book: aplica los comandos en orden estricto, asegura los fondos de cada orden, decide los trades con price-time priority y publica lo ocurrido, recuperando su estado exacto tras una caída.

## Requirements

### Requirement: Single writer por book
Los comandos de un mismo book MUST aplicarse de a uno, en el orden en que quedaron en el log. En cada momento MUST haber como máximo un engine activo aplicando comandos de un book. Books distintos MAY procesarse en paralelo.

#### Scenario: Comandos concurrentes
- **WHEN** cuatro personas envían órdenes al mismo book al mismo tiempo
- **THEN** el engine las aplica una tras otra, en el orden del log, sin intercalar sus efectos

### Requirement: Sequence monotónico
El engine MUST asignar a cada comando aplicado un `sequence` por book, estrictamente creciente y sin huecos. Todo evento emitido MUST llevar el `sequence` del comando que lo produjo.

#### Scenario: Secuencia sin huecos
- **WHEN** el engine aplica tres comandos seguidos
- **THEN** sus eventos llevan `sequence` N, N+1 y N+2

### Requirement: Comandos duplicados
El engine MUST ignorar un comando cuyo `id` de mensaje ya aplicó, y un `NewOrder` cuyo `orderId` ya conoce, sin asignarle `sequence` ni emitir eventos.

#### Scenario: Comando repetido en el log
- **WHEN** el mismo `NewOrder`, con el mismo `id`, aparece dos veces en el log (por ejemplo, por un reintento del producer)
- **THEN** la orden entra al book una sola vez

### Requirement: Reserva antes de entrar al book
Antes de que una orden entre al book, el engine MUST reservar en la wallet el máximo que puede necesitar: compra limit = cantidad × precio; compra market = `amount`; venta = cantidad de VIB. Si la reserva se rechaza, MUST emitir `OrderRejected` sin tocar el book. Si la wallet no responde, MUST reintentar sin avanzar al siguiente comando.

#### Scenario: Fondos insuficientes
- **WHEN** llega una compra limit de 10 VIB a R$ 90 y la persona solo tiene R$ 500 disponibles
- **THEN** el engine emite `OrderRejected` con `reason = insufficient_funds` y el book no cambia

#### Scenario: Fondos suficientes
- **WHEN** llega una compra limit de 10 VIB a R$ 90 y la persona tiene R$ 1.000 disponibles
- **THEN** la wallet queda con R$ 900 reservados y el engine emite `OrderAccepted`

#### Scenario: Wallet temporalmente caída
- **WHEN** la reserva falla por timeout
- **THEN** el engine reintenta la misma reserva y no procesa el comando siguiente hasta obtener una respuesta definitiva

### Requirement: Orden limit que reposa
La parte no ejecutada de una orden limit MUST quedar en el book con su precio y `sequence` hasta llenarse o cancelarse. Las órdenes limit no expiran.

#### Scenario: Limit sin contraparte
- **WHEN** llega una venta limit de 5 VIB @ 120 con fondos suficientes y nada contra qué cruzar
- **THEN** se emite `OrderAccepted` y la orden queda en el book

### Requirement: Orden market nunca reposa
Una orden market MUST NOT quedar en el book. Lo que no se ejecute en el mismo comando MUST cancelarse con `reason = no_liquidity`, y su reserva no usada MUST liberarse en la wallet antes de pasar al comando siguiente.

#### Scenario: Market sin contraparte
- **WHEN** llega una market sell de 3 VIB y no hay compras en el book
- **THEN** la orden termina con `OrderCancelled`, `reason = no_liquidity`, y los 3 VIB vuelven a `available`

### Requirement: Cancelación en el engine
Un `CancelOrder` sobre una orden en el book MUST sacarla del book, liberar en la wallet su reserva no usada antes de pasar al comando siguiente y emitir `OrderCancelled` con el monto liberado. Sobre una orden inexistente, ajena o ya final, MUST NOT cambiar nada ni emitir eventos; solo lo registra en el log.

#### Scenario: Cancelar orden en reposo
- **WHEN** se cancela una compra limit de 6 VIB @ R$ 90 que está en el book
- **THEN** la wallet libera R$ 540 y se emite `OrderCancelled` con `released = 540.00`

#### Scenario: Cancelación repetida
- **WHEN** llegan dos `CancelOrder` para una compra limit que está en el book
- **THEN** el primero la cancela y libera su reserva, y el segundo no libera nada ni emite eventos

#### Scenario: Cancelar orden desconocida
- **WHEN** llega un `CancelOrder` de un `orderId` que el engine no conoce
- **THEN** no se emite ningún evento

### Requirement: Ajuste de reserva al modificar
Un `ModifyOrder` MUST recalcular la reserva requerida por el nuevo precio y la nueva cantidad pendiente: reservar la diferencia si aumenta (si no alcanza, la orden queda como estaba y no se emite ningún evento) o liberar el excedente si disminuye. Solo después MUST aplicar el cambio y emitir `OrderModified`. Sobre una orden inexistente, ajena o ya final, MUST NOT cambiar nada ni emitir eventos.

#### Scenario: Reducción de cantidad
- **WHEN** una compra en reposo de 10 VIB @ R$ 90 se reduce a 4 VIB
- **THEN** la wallet libera R$ 540 y se emite `OrderModified`

#### Scenario: Aumento sin fondos
- **WHEN** se aumenta una compra de 5 a 50 VIB y no hay fondos para la diferencia
- **THEN** la orden queda como estaba y no se emite ningún evento

### Requirement: Reconstrucción al reiniciar
Al arrancar, el engine MUST reconstruir su book releyendo su partición de `orders.commands` desde el principio. La relectura MUST NOT mover fondos otra vez ni publicar eventos que ya se publicaron, y MUST publicar los eventos que un lote no llegó a publicar antes de una caída.

#### Scenario: Reinicio tras una caída
- **WHEN** el engine se detiene después de aplicar 1.000 comandos y vuelve a arrancar
- **THEN** reconstruye el mismo book y los saldos de la wallet no cambian por la relectura

#### Scenario: Reinicio sin eventos repetidos
- **WHEN** el engine vuelve a arrancar
- **THEN** `orders.events` no recibe de nuevo ningún evento de los comandos ya publicados

#### Scenario: Caída antes de publicar
- **WHEN** el engine se cae después de que la wallet aplicó un lote y antes de publicar todos sus eventos
- **THEN** al arrancar publica los eventos que faltaron, una sola vez

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

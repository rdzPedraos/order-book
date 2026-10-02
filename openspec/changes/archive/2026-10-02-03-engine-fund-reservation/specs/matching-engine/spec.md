# Spec Delta

## Purpose

Es la autoridad sobre el Order Book activo de cada book: aplica los comandos en orden estricto, asegura los fondos de cada orden, decide los trades con price-time priority y publica lo ocurrido, recuperando su estado exacto tras una caída.

## ADDED Requirements

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

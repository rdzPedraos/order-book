# Spec Delta

## ADDED Requirements

### Requirement: Comando de creación
Al crear una orden, OrderService MUST publicar su comando `NewOrder` en el log y esperar su confirmación antes de responder. La orden existe desde que su `NewOrder` está en el log: no puede existir una orden sin su comando ni un comando sin su orden.

#### Scenario: Orden con su comando
- **WHEN** la persona crea una orden limit de 10 VIB @ 90.00
- **THEN** el sistema responde `201` con la orden en `PENDING` y el log tiene un `NewOrder` para ese `orderId`

#### Scenario: Log no disponible al crear
- **WHEN** el log no confirma el `NewOrder`
- **THEN** el sistema responde `503` y no queda ni orden ni comando

### Requirement: Registro de órdenes desde el log
OrderService MUST guardar en su consulta de órdenes cada orden cuyo `NewOrder` está en el log, consumiendo los comandos en orden. Procesar más de una vez el mismo comando MUST NOT duplicar la orden, y `ModifyOrder` y `CancelOrder` MUST NOT cambiarla. La consulta es eventualmente consistente: una orden aceptada aparece en `GET /orders` y `GET /orders/{id}` después de que el consumidor la guarda.

#### Scenario: Orden visible tras registrarse
- **WHEN** la persona crea una orden y el consumidor procesa su `NewOrder`
- **THEN** `GET /orders/{id}` devuelve la orden en `PENDING`

#### Scenario: Comando entregado dos veces
- **WHEN** el consumidor recibe dos veces el mismo `NewOrder`
- **THEN** existe una sola orden con ese `orderId`

### Requirement: Modificación asíncrona
`POST /orders/{id}/change` MUST aceptar un nuevo `limit`, un nuevo `quantity` pendiente (≥ 1) o ambos, solo para órdenes limit propias, aplicar las mismas reglas de validación que la creación, publicar un comando `ModifyOrder` y responder `201` con la orden tal como está. OrderService MUST NOT cambiar la orden: el resultado lo decide quien procesa el comando.

#### Scenario: Cambio de limit publicado
- **WHEN** la persona pide cambiar el `limit` de su orden de 90.00 a 92.00
- **THEN** el sistema responde `201` con la orden en `limit = 90.00` y el log tiene un `ModifyOrder` con `limit = 92.00`

#### Scenario: Orden market
- **WHEN** la persona intenta modificar una orden market
- **THEN** el sistema responde `409` con código `order_not_modifiable` y no publica ningún comando

### Requirement: Cancelación asíncrona
`POST /orders/{id}/close` MUST publicar un comando `CancelOrder` para una orden propia y responder `201` con la orden tal como está. OrderService MUST NOT cambiar el estado de la orden: la cancelación puede competir con una ejecución en curso, y el resultado lo decide quien procesa los comandos en orden. Cada llamado publica su propio `CancelOrder`; los que llegan sobre una orden ya final los rechaza quien procesa los comandos, sin efectos.

#### Scenario: Cancelación publicada
- **WHEN** la persona pide cancelar su orden en `PENDING`
- **THEN** el sistema responde `201` con la orden en `PENDING` y el log tiene un `CancelOrder`

#### Scenario: Cancelación repetida
- **WHEN** la persona envía dos veces `POST /orders/{id}/close` sobre la misma orden
- **THEN** ambas responden `201` con la orden en `PENDING` y el log tiene dos `CancelOrder` con `id` distintos

## REMOVED Requirements

### Requirement: Endpoints de modificación y cancelación reservados
**Reason**: Los stubs `PATCH` y `DELETE /orders/{id}`, que respondían `501`, se reemplazan por `POST /orders/{id}/change` y `POST /orders/{id}/close`, porque la respuesta no puede asegurar que la orden cambió.
**Migration**: Los clientes usan `POST /orders/{id}/change` (mismo body: `limit` y/o `quantity`) y `POST /orders/{id}/close`, y ven el resultado en la orden cuando se proyectan los eventos del engine.

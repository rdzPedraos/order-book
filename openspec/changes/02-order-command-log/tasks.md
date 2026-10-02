# Tasks

## 1. Infraestructura del log

- [ ] 1.1 Agregar Redpanda a `deploy/docker-compose.yml` y un job de init que crea `orders.commands` y `orders.events` con retención infinita, y Redpanda Console; verificar con `rpk topic list` y `rpk topic describe` que existen con la configuración esperada, y que Console muestra los topics en `http://localhost:8081`

## 2. Comandos y partición

- [ ] 2.1 `shared/events`: escribir tests de round-trip JSON de `NewOrder`, `ModifyOrder` y `CancelOrder` con su envelope (`commandId`, `type`, `book`, `userId`, `orderId`, `schemaVersion`, `acceptedAt`, payload), y verlos fallar; implementar los tipos en `commands.go`; refactor en verde
- [ ] 2.2 `shared/topics`: escribir los tests de «Mismo book, misma partición» (`trading-events`), que todo comando de `BRL-VIB` va a la partición 0, y de que un book desconocido devuelve error, y verlos fallar; implementar en `topics.go` los nombres de los topics y `Partition(book)` con el mapa `book → partition`; refactor en verde

## 3. Publicación en OrderService

- [ ] 3.1 `store/commandlog`: escribir los tests de su `InitMock(t)` (registra lo publicado, falla con `mock.Err`) y el test de integración de que un comando publicado queda en la partición de su book con `acks=all`, y verlos fallar; implementar el producer `franz-go` con idempotent producer, el partitioner que usa `topics.Partition` y el mock; refactor en verde
- [ ] 3.2 `POST /orders`: escribir los tests de «Orden con su comando» y «Log no disponible al crear» (`order-management`), y verlos fallar; cambiar `handlers/create-order` para que arme el `NewOrder` con la orden completa, lo publique con `commandlog`, deje de escribir en `orderdb` y responda `201` o `503`; refactor en verde
- [ ] 3.3 `PATCH /orders/{id}`: escribir los tests de «Cambio de limit solicitado» y «Orden market» (`order-management`), y verlos fallar; reemplazar el `501` de `handlers/modify-order` por la publicación del `ModifyOrder` y la respuesta `202` con `orderId` y `commandId`, con `ErrOrderNotModifiable` en `models`; refactor en verde
- [ ] 3.4 `DELETE /orders/{id}`: escribir los tests de «Cancelación solicitada» y «Cancelación repetida» (`order-management`), y verlos fallar; reemplazar el `501` de `handlers/cancel-order` por la publicación del `CancelOrder` y la respuesta `202` con `orderId` y `commandId`, sin cambiar la orden; refactor en verde

## 4. Consumidor de comandos

- [ ] 4.1 `handlers/order-commands`: escribir los tests de «Orden visible tras registrarse» y «Comando entregado dos veces» (`order-management`) con `orderdb.InitMock`, y verlos fallar; implementar el subscriber de Gofr sobre `orders.commands` que guarda cada `NewOrder`, ignora `ModifyOrder` y `CancelOrder` y devuelve error si la base falla, y en `store/orderdb` el `InsertOrder` idempotente con `ON CONFLICT (id) DO NOTHING` (con su test de integración); registrar el subscriber y el producer en `main.go`; refactor en verde
- [ ] 4.2 Resiliencia: escribir los tests de integración de «Mismo book, misma partición», «Orden preservado por instancia», «Log no disponible», «Caída antes de la confirmación» y «Reintento del producer» (`trading-events`) y verlos fallar; ajustar en `store/commandlog` los reintentos con backoff dentro del timeout de la petición y el idempotent producer; refactor en verde
- [ ] 4.3 `order_projection_lag_seconds`: escribir el test de que la métrica refleja la antigüedad del último comando guardado y crece si el consumidor se detiene, y verlo fallar; implementarla en `handlers/order-commands`; refactor en verde y verificar que aparece en `/metrics`

## 5. Documentación

- [ ] 5.1 Actualizar `docs/api.md` con la semántica asíncrona de `PATCH`/`DELETE` (`202` con `orderId` y `commandId`, y los resultados posibles de una cancelación: cancelada, cancelada en parte o rechazada porque la orden ya se llenó), la lectura eventual tras el `POST` y el `503` cuando el log no confirma, y documentar en `docs/internal.md` el topic `orders.commands` (clave, partición, envelope, ejemplos JSON de los tres comandos y garantías de entrega); verificar que los ejemplos deserializan con `shared/events`

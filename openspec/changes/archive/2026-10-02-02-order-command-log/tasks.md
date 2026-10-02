# Tasks

## 1. Infraestructura del log

- [x] 1.1 Agregar Redpanda a `deploy/docker-compose.yml` y un job de init que crea `orders.commands` y `orders.events` con retención infinita, y Redpanda Console; verificar con `rpk topic list` y `rpk topic describe` que existen con la configuración esperada, y que Console muestra los topics en `http://localhost:8081`

## 2. Comandos y partición

- [x] 2.1 `shared/eventlog` (envelope y comandos): escribir tests de round-trip JSON del envelope `Message` (`id`, `route`, `book`, `schemaVersion`, `createdAt`, payload) y de los payloads `NewOrder`, `ModifyOrder` y `CancelOrder` (con `orderId` y `userId`), y verlos fallar; implementar los tipos en `commands.go`; refactor en verde
- [x] 2.2 `shared/books` y `shared/eventlog`: escribir los tests de «Mismo book, misma partición» (`trading-events`), que todo comando de `BRL-VIB` va a la partición 0, y de los nombres de los topics, y verlos fallar; agregar `Partition` a cada book del registro e implementar en `shared/eventlog` los nombres de los topics; refactor en verde

## 3. Publicación en OrderService

- [x] 3.1 `shared/eventlog`: escribir los tests de su `InitMock(t)` (registra lo publicado, falla con `mock.Err`) y el test de integración de que un comando publicado queda en la partición de su book con `acks=all`, y verlos fallar; implementar el producer `franz-go` con idempotent producer, el partitioner manual que usa la partición del book y el mock; refactor en verde
- [x] 3.2 `POST /orders`: escribir los tests de «Orden con su comando» y «Log no disponible al crear» (`order-management`), y verlos fallar; cambiar `handlers/create-order` para que arme el `NewOrder` con la orden completa, lo publique con `commands.Publish`, deje de escribir en `orderdb` y responda `201` o `503`; refactor en verde
- [x] 3.3 `POST /orders/{id}/change`: escribir los tests de «Cambio de limit publicado» y «Orden market» (`order-management`), y verlos fallar; reemplazar `handlers/modify-order` por `handlers/change-order`, que publica el `ModifyOrder` y responde `201` con la orden sin cambios, con `ErrOrderNotModifiable` en `models`; quitar la ruta `PATCH /orders/{id}` de `main.go`; refactor en verde
- [x] 3.4 `POST /orders/{id}/close`: escribir los tests de «Cancelación publicada» y «Cancelación repetida» (`order-management`), y verlos fallar; reemplazar `handlers/cancel-order` por `handlers/close-order`, que publica el `CancelOrder` y responde `201` con la orden sin cambios; quitar la ruta `DELETE /orders/{id}` de `main.go`; refactor en verde

## 4. Consumidor de comandos

- [x] 4.1 `handlers/insert-new-order`: escribir los tests de «Orden visible tras registrarse», «Comando entregado dos veces» y de que un tipo sin suscripción (`ModifyOrder`, `CancelOrder`) se confirma sin aplicarse (`order-management`) con `orderdb.InitMock`, y verlos fallar; implementar `consumer.Start` (consumidor `franz-go` con commit manual tras guardar y reintento con backoff) y el handler que guarda cada `NewOrder` en `orders`, suscrito por topic y tipo, y en `store/orderdb` el `InsertOrder` idempotente con `ON CONFLICT (id) DO NOTHING` (con su test de integración); registrar el subscriber y el producer en `main.go`; refactor en verde
- [x] 4.2 Resiliencia: escribir los tests de integración de «Mismo book, misma partición», «Orden preservado por instancia», «Log no disponible», «Caída antes de la confirmación» y «Reintento del producer» (`trading-events`) y verlos fallar; ajustar en `shared/eventlog` los reintentos con backoff dentro del timeout de la petición y el idempotent producer; refactor en verde
- [x] 4.3 `order_projection_lag_seconds`: escribir el test de que la métrica refleja la antigüedad del comando recién guardado, y verlo fallar; implementarla en `handlers/insert-new-order`; refactor en verde y verificar que aparece en `/metrics` y que el lag del consumer group en Redpanda (`rpk group describe order-service`) crece con el consumidor detenido
- [x] 4.4 Roles: separar en `main.go` el rol `api` (producer y rutas) del rol `projector` (consumidor y métrica) con `ROLE`, que termina el proceso si es desconocido; agregar `ROLE` a `configs/.env.example`; verificar corriendo los dos procesos que una orden creada en la `api` aparece por el `projector`, que la `api` sola no consume y que el `projector` no abre el puerto HTTP

## 5. Documentación

- [x] 5.1 Actualizar `docs/api.md` con `POST /orders/{id}/change` y `POST /orders/{id}/close` (en lugar de `PATCH` y `DELETE`), que responden la orden sin cambios, los resultados posibles de una cancelación (cancelada, cancelada en parte o rechazada porque la orden ya se llenó), la lectura eventual tras el `POST` y el `503` cuando el log no confirma, y escribir `.claude/standards/eventlog.md` (cargado desde `CLAUDE.md`) con el envelope, las rutas, cómo publicar y suscribirse y las garantías de entrega

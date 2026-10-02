# Proposal

## TL;DR

Fase 2 de 8. Introducir los comandos de órdenes (`NewOrder`, `ModifyOrder` y `CancelOrder`) y publicarlos en Redpanda (API de Kafka), en el topic `orders.commands` particionado por book. El log es el registro principal: `POST`, `PATCH` y `DELETE /orders` publican su comando directamente, y un consumidor de OrderService guarda la orden en la tabla `orders` apenas su `NewOrder` está en el log. Esta fase también agrega modificar y cancelar como solicitudes asíncronas (`202`): OrderService registra la intención y no decide el resultado.

## Why

El engine procesará cada book con un single writer: un único proceso aplica los comandos del book, uno tras otro. Para eso, todos los comandos de un book tienen que llegar en un orden total y durable. El Event Log da ese orden, desacopla OrderService del engine y permite el replay que usará la recuperación. Escribir primero en el log, y solo en el log, hace que "aceptado" signifique "durable en el log": no hay una segunda escritura que pueda quedar a medias, así que no puede existir una orden sin su comando ni un comando sin su orden.

## Goals

- Publicar todos los comandos sin perder ninguno y sin desordenarlos.
- Que todos los comandos de un book vayan a la misma partición.
- Que una caída de Redpanda o de OrderService no deje órdenes sin comando ni comandos sin orden.
- Que la tabla `orders` se reconstruya desde el log, consumiendo cada comando sin duplicar efectos.

## What Changes

- **Comandos:** tipos de comando y su envelope en `shared/events`, y los topics con la partición de cada book en `shared/topics`.
- **Publicación directa:** `POST /orders` publica `NewOrder` con `acks=all` y responde `201` con la orden en `PENDING`; si el log no confirma, responde `503` y no queda nada.
- **Consumidor:** un subscriber de OrderService lee `orders.commands` y guarda cada `NewOrder` en `orders` de forma idempotente. La lectura es eventualmente consistente: un `GET` inmediato puede no ver todavía la orden.
- **Solicitudes:** `PATCH /orders/{id}` y `DELETE /orders/{id}` dejan de ser stubs (`501`). Publican `ModifyOrder` / `CancelOrder`, responden `202` con el `orderId` y el `commandId`, y no cambian la orden. Repetir una cancelación publica otro `CancelOrder`; el engine rechaza los que llegan sobre una orden ya final.
- **Infra:** Redpanda en `deploy/docker-compose.yml`, con un job de init que crea `orders.commands` y `orders.events` con retención infinita, y Redpanda Console para ver en desarrollo los topics, los mensajes y el lag de los consumidores.
- **Producer:** `acks=all`, idempotent producer y reintentos del cliente, con un partitioner manual que toma la partición de `shared/topics`.
- **Métricas y docs:** métrica `order_projection_lag_seconds` (cuánto tarda una orden aceptada en aparecer en `orders`) y documentación del topic en `docs/internal.md`.

## Capabilities

### New Capabilities

- `trading-events`: contrato durable de comandos, con orden por book.

### Modified Capabilities

- `order-management`: se agregan el comando de creación y las solicitudes asíncronas de modificación y cancelación.

## Assumptions

- Depende de `01-order-api`.
- Las órdenes creadas durante la fase 1 no tienen comando. Son datos de desarrollo y se descartan al desplegar esta fase.
- Redpanda local con un solo broker y replication factor 1. En Kubernetes (fase 6) pasa a 3.

## Concepts

| Concepto | Significado |
| --- | --- |
| Comando | Intención del usuario sobre una orden: `NewOrder`, `ModifyOrder` o `CancelOrder`. |
| Consumidor de comandos | Subscriber de OrderService que lee `orders.commands` y guarda en `orders` cada orden creada, de forma idempotente. |
| Lectura eventual | Una orden aceptada aparece en `GET /orders` cuando el consumidor la guarda, unos milisegundos después del `201`. |

## Impact

- **Código:** `microservices/order-service` (publicación en los handlers, consumidor de `orders.commands`, `PATCH`/`DELETE`), `shared/events`, `shared/topics`.
- **Infra:** Redpanda y Redpanda Console en compose.
- **Docs:** `docs/api.md` (semántica de `202`, resultados posibles de una cancelación y lectura eventual) y `docs/internal.md` (nuevo: topic `orders.commands`).
- **Dependencias:** `franz-go` (cliente Kafka con partitioner manual y batching).

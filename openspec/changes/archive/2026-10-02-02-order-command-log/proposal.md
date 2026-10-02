# Proposal

## TL;DR

Fase 2 de 8. Introducir los comandos de órdenes (`NewOrder`, `ModifyOrder` y `CancelOrder`) y publicarlos en Redpanda (API de Kafka), en el topic `orders.commands` particionado por book. El log es el registro principal: crear, modificar y cancelar publican su comando directamente, y un consumidor de OrderService guarda la orden en la tabla `orders` apenas su `NewOrder` está en el log. Modificar y cancelar pasan a ser asíncronos (`POST /orders/{id}/change` y `POST /orders/{id}/close`): OrderService publica la intención y la orden cambia después, cuando se proyectan los eventos del engine.

## Why

El engine procesará cada book con un single writer: un único proceso aplica los comandos del book, uno tras otro. Para eso, todos los comandos de un book tienen que llegar en un orden total y durable. El Event Log da ese orden, desacopla OrderService del engine y permite el replay que usará la recuperación. Escribir primero en el log, y solo en el log, hace que "aceptado" signifique "durable en el log": no hay una segunda escritura que pueda quedar a medias, así que no puede existir una orden sin su comando ni un comando sin su orden.

## Goals

- Publicar todos los comandos sin perder ninguno y sin desordenarlos.
- Que todos los comandos de un book vayan a la misma partición.
- Que una caída de Redpanda o de OrderService no deje órdenes sin comando ni comandos sin orden.
- Que la tabla `orders` se reconstruya desde el log, consumiendo cada comando sin duplicar efectos.

## What Changes

- **Comandos:** los nombres de los topics, los tipos de comando, sus payloads y el envelope común `Message` en `shared/eventlog`, y la partición de cada book en el registro de `shared/books`.
- **Publicación directa:** `POST /orders` publica `NewOrder` con `acks=all` y responde `201` con la orden en `PENDING`; si el log no confirma, responde `503` y no queda nada.
- **Consumidor:** un subscriber de OrderService lee `orders.commands` y guarda cada `NewOrder` en `orders` de forma idempotente. La lectura es eventualmente consistente: un `GET` inmediato puede no ver todavía la orden.
- **Modificar y cancelar:** los stubs `PATCH` y `DELETE /orders/{id}` (`501`) se reemplazan por `POST /orders/{id}/change` y `POST /orders/{id}/close`. Cada uno publica su `ModifyOrder` / `CancelOrder`, responde `201` con la orden sin cambios; la fase 5 la actualiza con los eventos del engine. Repetir una cancelación publica otro `CancelOrder`, que el engine rechaza si la orden ya es final.
- **Infra:** Redpanda en `deploy/docker-compose.yml`, con un job de init que crea `orders.commands` y `orders.events` con retención infinita, y Redpanda Console para ver en desarrollo los topics, los mensajes y el lag de los consumidores.
- **Producer:** `acks=all`, idempotent producer y reintentos del cliente, con un partitioner manual que toma la partición del book.
- **Métricas y docs:** métrica `order_projection_lag_seconds` (cuánto tarda una orden aceptada en aparecer en `orders`) y las reglas del log en `.claude/standards/eventlog.md`.

## Capabilities

### New Capabilities

- `trading-events`: contrato durable de comandos, con orden por book.

### Modified Capabilities

- `order-management`: se agregan el comando de creación y la modificación y cancelación asíncronas.

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

- **Código:** `microservices/order-service` (publicación en los handlers, consumidor de `orders.commands`, rutas `change`/`close`), `shared/eventlog` y `shared/books`.
- **Infra:** Redpanda y Redpanda Console en compose.
- **Docs:** `docs/api.md` (modificación y cancelación asíncronas, sus resultados posibles y la lectura eventual) y `.claude/standards/eventlog.md` (nuevo: cómo publicar y consumir el log).
- **Dependencias:** `franz-go` (cliente Kafka con partitioner manual y batching).

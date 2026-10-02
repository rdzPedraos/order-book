# Proposal

## TL;DR

Fase 1 de 8. Crear el monorepo Go + Gofr y la API REST de órdenes (OrderService). Una persona, identificada con `X-User-ID`, puede crear y consultar sus órdenes, que se guardan en PostgreSQL. Las órdenes nuevas quedan en `PENDING`.

## Why

El PRD ([docs/PDR/order-book-vibranium.md](../../../docs/PDR/order-book-vibranium.md)) y la solución técnica empiezan por la puerta de entrada: la API pública de órdenes. Construirla primero fija el contrato con los clientes (formato de dinero, validaciones, identidad y estados), del que dependen las demás fases. Además permite probarla sola, sin otra infraestructura que la base de datos.

## Goals

- Contrato REST de órdenes estable y documentado en `docs/api.md`.
- Validación completa de la forma de las órdenes (market y limit, compra y venta) según las reglas del PRD.
- Cada persona ve y opera solo sus propias órdenes.

## What Changes

- Monorepo Go con un solo módulo (`go.mod` en la raíz) y paquetes compartidos `shared/money`, `shared/books` y `shared/identity`.
- OrderService con `POST /orders`, `GET /orders` y `GET /orders/{id}`, más `PATCH` y `DELETE /orders/{id}` como stubs que validan la entrada y responden `501`.
- Tabla `orders` en PostgreSQL.
- `docker-compose.yml` mínimo con PostgreSQL.
- `docs/api.md` con las convenciones comunes y los endpoints de órdenes. El PRD actualizado con los desvíos aprobados: estados `PENDING`/`REJECTED`, saldo de un trade eventualmente disponible, profundidad pública e identidad por `X-User-ID`.

## Capabilities

### New Capabilities

- `order-management`: identificación por `X-User-ID` y aislamiento entre personas, creación, validación, listado y detalle de órdenes, y el contrato de entrada de modificación y cancelación.

### Modified Capabilities

Ninguna.

## Assumptions

- Un solo book: `BRL-VIB`. VIB es entero; BRL lleva 2 decimales. Internamente todo es `int64` en unidades mínimas.
- `X-User-ID` es obligatorio, pero en esta fase no se verifica contra un registro de personas.

## Concepts

| Concepto | Significado |
| --- | --- |
| Book | Par negociable; en el MVP solo `BRL-VIB`. |
| `PENDING` | Estado inicial de toda orden creada. |

## Impact

- **Código nuevo:** `go.mod`, `shared/*`, `microservices/order-service`, más los esqueletos de los demás servicios.
- **Infra:** `deploy/docker-compose.yml` (PostgreSQL).
- **Docs:** `docs/api.md` (nuevo) y `docs/PDR/order-book-vibranium.md` (ajustes).
- **Dependencias:** Gofr y el driver de PostgreSQL.

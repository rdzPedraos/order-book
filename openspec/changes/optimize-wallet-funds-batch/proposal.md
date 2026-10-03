# Proposal

## TL;DR

Aplicar cada lote de `funds:batch` con **una sola llamada** a PostgreSQL en vez de ~1.500. Una función de PostgreSQL, `apply_funds_batch`, recibe el lote completo y hace por dentro lo mismo que hoy hace Go: por cada operación revisa si su mensaje ya se aplicó, mueve el saldo si alcanza y lo anota en el `ledger`. El endpoint, sus resultados y el engine no cambian.

## Why

El benchmark de la fase 6 (escenario B a 1.000/s con 5.000 personas) mostró que el engine solo procesa ~575 órdenes/s, y que el cuello de botella es `funds:batch`: p50 de 66 ms, p90 de 854 ms y p99 de 1,4 s. Hoy un lote de 500 operaciones es una transacción con 3 consultas por operación (`SELECT` del ledger, `UPDATE` de `balances`, `INSERT` en el ledger), una detrás de otra. Las filas de `balances` quedan bloqueadas hasta el `COMMIT`, y el rol `trades` espera ese bloqueo para pagar los trades (`pg_stat_activity`: `Lock` / `transactionid`). Sin resolverlo, la tarea 3.2 de la fase 6 (5.000/s sostenidas) no puede pasar.

## Goals

- Un lote de `funds:batch` se aplica en un solo viaje a la base.
- Un lote dura y bloquea `balances` mucho menos que hoy, medido con `tools/loadgen` contra la línea base.
- El engine sigue el ritmo del escenario B a 1.000/s sin que crezca su atraso.

## Non-Goals

- Cambiar el contrato de `POST /wallet/internal/funds:batch` o el cliente del engine.
- Optimizar el rol `trades` o procesar en lotes los consumidores de Kafka (`trades`, `projector`, `market`). Se evalúa después, según lo que mida esta optimización.
- Cambiar de motor de base de datos.

## Concepts

- **Viaje a la base:** Go manda una consulta y espera la respuesta antes de mandar la siguiente. Hoy un lote hace ~1.500 viajes; con la función hace uno.
- **Bloqueo de fila:** cuando una transacción actualiza el saldo de una persona, nadie más puede actualizarlo hasta que termine. Mientras más dura la transacción, más esperan los demás.

## What Changes

- Una migración nueva de WalletService crea la función `apply_funds_batch`. Recibe el lote como JSON y devuelve el resultado de cada operación (`OK`, `insufficient_funds` o `release_exceeds_reservation`), con las mismas reglas que hoy.
- `walletdb.ApplyFundsBatch` llama a la función con una sola consulta, sin abrir una transacción propia ni recorrer las operaciones en Go.

## Capabilities

### New Capabilities

Ninguna.

### Modified Capabilities

Ninguna. El comportamiento no cambia (`skip_specs: true`): los requirements actuales de `wallet` («Reserva atómica de fondos», «Liberación de fondos», «Ledger inmutable» y el invariante no negativo bajo concurrencia de «Saldos por moneda») son el criterio de aceptación de la función.

## Assumptions

- PostgreSQL 16, el de compose y el del chart, trae PL/pgSQL habilitado.
- La línea base son las corridas de la fase 6 en minikube: escenario B a 1.000/s durante 30 s, con 200 y con 5.000 personas.

## Impact

- **Código:** `microservices/wallet-service/migrations` (una migración nueva) y `microservices/wallet-service/store/walletdb` (`postgres_funds.go` y sus tests).
- **API:** ninguno.
- **Dependencias:** ninguna.
- **Operación:** la función se crea al arrancar los pods, como las demás migraciones.

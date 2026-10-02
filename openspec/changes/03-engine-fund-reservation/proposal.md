# Proposal

## TL;DR

Fase 3 de 8. Crear WalletService (saldos, depósitos, retiros, ledger y la operación interna `funds:batch`) y el MatchingEngine en modo "sin cruce". El engine consume `orders.commands` de su book en orden y, al agregar una orden, llama a WalletService para reservar los fondos. Al cancelarla, los libera. Publica los eventos de ciclo de vida en `orders.events`. Las órdenes limit quedan en el book en memoria; el matching llega en la fase 4.

## Why

La regla central del PRD es que ninguna orden entra al book sin su dinero congelado, y que ese dinero no se puede usar en otra orden, en otro book ni en un retiro. Resolver la reserva y la liberación antes del matching deja probada por separado la parte más delicada para el dinero (doble gasto, concurrencia, idempotencia), y también el loop single writer del engine.

## Goals

- WalletService como autoridad única del dinero, con reservas atómicas e idempotentes.
- Un engine single writer por book que aplica los comandos en el orden del log.
- Que al reiniciar el engine reconstruya su book releyendo el log, sin congelar dinero dos veces ni publicar eventos que ya salieron.

## Non-Goals

- Cruzar órdenes y generar trades (fase 4).
- Liquidar trades en la wallet (fase 5).
- Snapshots (fase 7): por ahora el reinicio relee el log desde el principio.
- Registrar personas (HU-01 del PDR). En el MVP toda persona existe: el `X-User-ID` no se valida contra un registro, y la wallet de un id que nunca operó tiene saldo cero.

## What Changes

- WalletService:
  - `GET /wallet`, `POST /wallet/deposits` (BRL y VIB), `POST /wallet/withdrawals` (solo BRL) y `GET /wallet/movements`;
  - interno: `POST /wallet/internal/funds:batch` con operaciones `RESERVE` y `RELEASE`, solo en el rol `funds`: el servicio corre como `api` (rutas públicas) o `funds` (ruta interna), así la ruta interna no existe donde llegan las personas.
- MatchingEngine:
  - loop por book sobre la partición fija, con micro-batches y dedupe por el `id` del mensaje y el `orderId`;
  - `LevelStore` en memoria (heap + map + lista doblemente enlazada + índice por `orderId`);
  - reserva de fondos al aceptar, liberación al cancelar o modificar a la baja;
- Eventos `OrderAccepted`, `OrderRejected`, `OrderCancelled` y `OrderModified` en `orders.events`, con envelope e IDs determinísticos.

## Capabilities

### New Capabilities

- `wallet`: saldos, depósitos, retiros, reservas, liberaciones y ledger.
- `matching-engine`: single writer por book, `sequence`, reserva antes de entrar al book, cancelación, ajuste de reserva al modificar y reconstrucción al reiniciar sin eventos repetidos.

### Modified Capabilities

- `trading-events`: se agregan el envelope de evento y los eventos de ciclo de vida (introducida en `02-order-command-log`).

## Assumptions

- Depende de `01-order-api` y `02-order-command-log`.
- Se pueden depositar BRL y VIB (sin VIB no hay oferta inicial); solo se retira BRL.
- Mientras no exista la fase 4, una orden market no encuentra contraparte: se reserva, se cancela con `no_liquidity` y se libera. Es el comportamiento final del PRD para un book sin liquidez.

## Impact

- **Código:** `microservices/wallet-service`, `microservices/matching-engine`, `shared/eventlog`.
- **Datos:** base de WalletService: `balances` (los saldos) y `ledger` (su log inmutable, que también hace idempotentes las reservas y liberaciones).
- **Docs:** `docs/api.md` (endpoints públicos de WalletService) y `.claude/standards/eventlog.md` (rutas de `orders.events`); `funds:batch` se describe en el package comment de su handler.

# Spec Delta

## Purpose

Es la autoridad única sobre el dinero y los activos de cada persona: mantiene saldos disponibles y reservados por moneda, reserva y libera fondos para órdenes, liquida trades y registra cada movimiento, garantizando cero doble gasto.

## ADDED Requirements

### Requirement: Saldos por moneda
Cada wallet MUST mantener, para BRL y VIB, un saldo `available` y uno `reserved`, y exponer `total = available + reserved`. Ningún saldo MUST ser negativo en ningún momento. BRL se expresa con 2 decimales y VIB en unidades enteras.

#### Scenario: Consulta de saldo
- **WHEN** la persona llama a `GET /wallet`
- **THEN** recibe para BRL y VIB los campos `available`, `reserved` y `total`

#### Scenario: Wallet sin movimientos
- **WHEN** se llama a `GET /wallet` con un `X-User-ID` que nunca operó
- **THEN** el sistema responde `200` con BRL y VIB en `available = 0` y `reserved = 0`

#### Scenario: Invariante no negativo bajo concurrencia
- **WHEN** llegan en paralelo dos reservas de R$ 800 sobre una wallet con R$ 1.000 disponibles
- **THEN** exactamente una tiene éxito y la otra se rechaza con `insufficient_funds`
- **AND** el saldo final es `available = 200`, `reserved = 800`

### Requirement: Depósito simulado
La persona MUST poder depositar BRL o VIB con un monto positivo, que se suma a `available`.

#### Scenario: Depósito de BRL
- **WHEN** la persona envía `POST /wallet/deposits` con `currency = BRL` y `amount = "150.00"`
- **THEN** su `available` de BRL aumenta en 150,00 y queda un movimiento `DEPOSIT` en el ledger

#### Scenario: VIB fraccionado
- **WHEN** la persona deposita `amount = "1.5"` de VIB
- **THEN** el sistema responde `400` con código `invalid_amount`

### Requirement: Retiro simulado de BRL
La persona MUST poder retirar BRL solo desde su saldo `available`. Un retiro que supere el disponible MUST rechazarse completo. El retiro de VIB MUST rechazarse.

#### Scenario: Retiro con saldo congelado
- **WHEN** la persona tiene `available = 100` y `reserved = 900` en BRL y pide retirar 500
- **THEN** el sistema responde `422` con código `insufficient_funds` y los saldos no cambian

#### Scenario: Retiro de VIB
- **WHEN** la persona pide retirar VIB
- **THEN** el sistema responde `422` con código `currency_not_withdrawable`

### Requirement: Reserva atómica de fondos
El sistema MUST ofrecer una operación interna de reserva que mueve un monto de `available` a `reserved` en una sola operación atómica, solo si `available` alcanza. MUST ser idempotente por el `id` del mensaje que la causa: repetirla devuelve el mismo resultado sin mover fondos otra vez.

#### Scenario: Reserva exitosa
- **WHEN** se reservan R$ 800 para la orden `o-1` sobre una wallet con R$ 1.000 disponibles
- **THEN** la wallet queda con `available = 200` y `reserved = 800`

#### Scenario: Reserva sin fondos
- **WHEN** se reservan R$ 800 para la orden `o-2` sobre una wallet con R$ 500 disponibles
- **THEN** la reserva se rechaza con `insufficient_funds` y los saldos no cambian

#### Scenario: Reserva repetida
- **WHEN** se repite la reserva de la orden `o-1`, con el mismo `id` de mensaje
- **THEN** el resultado es el mismo que el original y `reserved` no aumenta

### Requirement: Operación interna separada
`POST /wallet/internal/funds:batch` MUST atenderse solo en el rol `funds` de WalletService, y las rutas públicas solo en el rol `api`. Un rol MUST NOT registrar las rutas del otro.

#### Scenario: Ruta interna en el rol público
- **WHEN** se llama a `POST /wallet/internal/funds:batch` en una instancia con `ROLE=api`
- **THEN** el sistema responde `404` y no aplica ninguna operación

#### Scenario: Ruta pública en el rol interno
- **WHEN** se llama a `GET /wallet` en una instancia con `ROLE=funds`
- **THEN** el sistema responde `404`

### Requirement: Liberación de fondos
El sistema MUST ofrecer una operación interna que mueve de `reserved` a `available` el monto no usado de una orden. MUST ser idempotente por el `id` del mensaje que la causa y MUST NOT dejar `reserved` negativo.

#### Scenario: Cancelación de orden abierta
- **WHEN** se libera la reserva de una orden cancelada con R$ 540 sin usar
- **THEN** la wallet mueve R$ 540 de `reserved` a `available`

#### Scenario: Liberación repetida
- **WHEN** la misma liberación (mismo `id` de mensaje) llega dos veces
- **THEN** los R$ 540 se liberan una sola vez

#### Scenario: Liberación excesiva
- **WHEN** se pide liberar más que el `reserved` de la wallet en esa moneda
- **THEN** la operación se rechaza con `release_exceeds_reservation` y los saldos no cambian

### Requirement: Ledger inmutable
Cada cambio de saldo MUST registrarse como un movimiento inmutable con tipo, moneda, monto, la orden, el `id` del mensaje que lo causó (si viene del log) y fecha. La persona MUST poder listar sus movimientos paginados y filtrados por rango de fechas.

#### Scenario: Reconstrucción del saldo
- **WHEN** se suman todos los movimientos de una persona por moneda
- **THEN** el resultado coincide con su `total` actual de esa moneda

#### Scenario: Listado de movimientos
- **WHEN** la persona llama a `GET /wallet/movements?from=...&to=...`
- **THEN** recibe sus movimientos del rango, más recientes primero, con cursor de paginación

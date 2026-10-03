# Proposal

## TL;DR

Que los tres lectores del log (el `projector` de OrderService, MarketService y el rol `trades` de WalletService) apliquen los mensajes **en lotes**, con una sola llamada a PostgreSQL y una sola confirmación en Kafka por lote, en vez de una consulta y una confirmación por mensaje. Es la misma técnica que ya aplicamos en `funds:batch`. Las reglas de cada proyección no cambian.

## Why

Con la wallet ya optimizada, el cuello pasó a los lectores del log. Después de las corridas de carga, el engine estaba al día y `wallet-trades` y `market-service` iban con lag 0, pero el `projector` de OrderService iba **152.609 mensajes atrasado**, drenando a ~780 mensajes/s. Por eso una orden cancelada siguió mostrándose `PARTIALLY_FILLED` unos 20 minutos, aunque sus fondos ya estaban liberados.

Cada lector hace lo mismo con cada mensaje: lo lee, lo aplica con una consulta que se confirma sola en la base y lo confirma en Kafka, y recién entonces pasa al siguiente. Son ~1,3 ms por mensaje. Cada orden le genera al projector unos 3 mensajes (el comando y sus eventos), así que a 5.000 órdenes/s necesitaría ~15.000 mensajes/s.

## Goals

- Cada lector aplica un lote de mensajes con una sola llamada a la base y confirma en Kafka una vez por lote.
- Los tres lectores siguen al engine: cuando una corrida de carga termina y el engine se pone al día, su lag llega a 0 en segundos, no en minutos.

## Non-Goals

- Cambiar qué guarda cada proyección o cómo se ve en la API.
- Tener más de un lector por consumer group: siguen con una réplica por partición.

## What Changes

- **`shared/eventlog/consumer`:** `consumer.StartGroup`, un lector de consumer group por lotes, reemplaza a `consumer.Start`. Lee lo que haya disponible, hasta un máximo, de las rutas del lector, lo entrega a un solo handler y confirma el lote en Kafka cuando el handler termina bien. Salen `consumer.Start`, `consumer.Subscribe` y el handler por mensaje. **BREAKING** en `shared/`, pero los únicos que lo usan son estos tres lectores, que cambian en este change.
- **OrderService (`projector`):** los 6 handlers por ruta se juntan en un handler de lote. Una función nueva de PostgreSQL aplica el lote completo con las mismas consultas de hoy (insertar la orden, su primer evento, cancelación, modificación y trades).
- **MarketService:** un handler de lote guarda solo el último estado de cada nivel del lote y lo aplica con una sola sentencia.
- **WalletService (`trades`):** un handler de lote. Una función nueva de PostgreSQL paga todos los trades del lote con los mismos movimientos de hoy.
- **Standards:** `eventlog.md` y `handlers.md` describen el lector por lotes en lugar del handler por ruta.

## Capabilities

### New Capabilities

Ninguna.

### Modified Capabilities

Ninguna. El comportamiento no cambia (`skip_specs: true`). El criterio de aceptación son los escenarios actuales de las proyecciones: «Consumidores idempotentes» de `trading-events`, los de estado y trades de `order-management`, «Nivel vaciado» de `market-data` y «Pago de trades» de `wallet`.

## Assumptions

- La línea base es la del drenaje observado en minikube: el projector en ~780 mensajes/s, con un lag de 152.609 que tardó unos 3 minutos en bajar a cero.
- Un lote puede mezclar `orders.commands` y `orders.events`, como hoy un lector puede recibirlos en cualquier orden entre topics. Las proyecciones ya toleran ese desorden.

## Impact

- **Código:** `shared/eventlog/consumer`; `microservices/order-service` (handlers, `store/orderdb`, una migración, `main.go`); `microservices/market-service` (handler, `store/marketdb`, `main.go`); `microservices/wallet-service` (handler, `store/walletdb`, una migración, `main.go`).
- **Standards:** `.claude/standards/eventlog.md` y `.claude/standards/handlers.md`.
- **API:** ninguno.
- **Operación:** los consumer groups siguen siendo los mismos, así que cada lector retoma desde su último offset confirmado.

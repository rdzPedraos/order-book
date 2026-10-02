# Tasks

## 1. WalletService: pago de trades

- [x] 1.1 Pago de `TradeExecuted`: escribir los tests de «Pago de un trade», «Trade duplicado» y «Conservación» (`wallet`) y «Entrega duplicada» (`trading-events`), y verlos fallar; implementar la migración del ledger (`TRADE_PAID`, `TRADE_RECEIVED` y `UNIQUE (type, message_id, currency)`), `handlers/apply-trade`, `models.Trade` y `walletdb.ApplyTrade`, con su test contra PostgreSQL; refactor en verde
- [x] 1.2 Rol `trades`: agregarlo al `switch` de `main.go` con `consumer.Start` y la ruta `TradeExecuted`, y su consumer group a `configs/.env.example`; verificar con `go test ./microservices/wallet-service/...`
- [x] 1.3 Orden entre liberación y pago: escribir el test contra PostgreSQL que aplica el `RELEASE` de una cancelación y el pago de un trade previo de la misma orden, en ambos órdenes, y comprueba el mismo saldo final

## 2. OrderService: estado de la orden

- [x] 2.1 Datos de la orden en su primer evento: escribir los tests de round-trip de `OrderAccepted` y `OrderRejected` con sus datos en `shared/eventlog/events`, y el de «Orden aceptada con sus datos» (`trading-events`) en el engine, y verlos fallar; agregar los campos al contrato y llenarlos en `handlers/apply-commands/new_order.go`; refactor en verde
- [x] 2.2 Eventos de ciclo de vida: escribir los tests de «Rechazo por fondos», «Modificación aplicada», «Cancelación que llega tarde» y «Evento antes que el comando» (`order-management`), y de que una market sigue en `PENDING` al ser aceptada, y verlos fallar; implementar la migración de `reason`, los estados en `models`, `handlers/apply-order-accepted`, `apply-order-rejected`, `apply-order-cancelled` y `apply-order-modified`, y `orderdb.InsertOrUpdateOrder`, `UpdateCancelledOrder` y `UpdateModifiedOrder` (una sentencia cada uno, con la condición en el `WHERE`), con sus tests contra PostgreSQL que aplican el comando y el primer evento en ambos órdenes, y sumar las rutas al `consumer.Start` del rol `projector`; refactor en verde
- [x] 2.3 Trades en la orden: escribir los tests de «Limit llenada al llegar», «Limit llenada en partes», «Market con remanente» y «Evento duplicado» (`order-management`), y verlos fallar; implementar la migración de la tabla `trades` y de `filled_amount`, `handlers/apply-trade-executed`, `models.Trade` y `orderdb.InsertTrade` (insertar el trade y, solo si entró, sumar a las dos órdenes y calcular su estado en el mismo `UPDATE`, en una transacción), el `avgPrice` al leer la orden, con su test contra PostgreSQL, y sumar la ruta al rol `projector`; refactor en verde

## 3. MarketService

- [x] 3.1 Crear `microservices/market-service/` con su `main.go` y su `configs/.env.example`, agregar `CREATE DATABASE market_service;` a `deploy/postgres/init.sql` y la migración de `levels`; verificar que corre sobre una base vacía y que `/.well-known/health` responde `200`
- [x] 3.2 Lectura de `OrderBookLevelChanged`: escribir los tests de un nivel nuevo, uno que cambia, «Nivel vaciado» (`market-data`) y un evento repetido, y verlos fallar; implementar `handlers/apply-level-changed` y `marketdb.UpdateLevel` (upsert, o delete con `volume = 0`), con su test contra PostgreSQL; refactor en verde
- [x] 3.3 `GET /market/orderbook/{book}?depth=`: escribir los tests de «Book con varios niveles», «Book en minúsculas», «Book desconocido», «Depth explícito» y «Depth inválido» (`market-data`), y verlos fallar; implementar `handlers/get-orderbook` sin `X-User-ID`, `ErrInvalidDepth` y `ErrBookNotFound` en `models` y `marketdb.ListLevels`, con su test contra PostgreSQL; refactor en verde

## 4. Prueba y documentación

- [x] 4.1 Con los servicios reales y un entorno limpio (`deploy/reset-dev.sh`), reproducir el ejemplo de HU-12 del PDR: depósitos, una venta y una compra que cruzan, y comprobar los saldos finales, el estado de las órdenes, la profundidad del book y que la suma de BRL y de VIB no cambia
- [x] 4.2 Documentar en `docs/api.md` `GET /market/orderbook/{book}`, los estados de una orden y su `reason`, los movimientos `TRADE_PAID` y `TRADE_RECEIVED` de la wallet y que el saldo recibido en un trade llega unos milisegundos después; verificar que cada ejemplo responde lo indicado

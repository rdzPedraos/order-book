# Tasks

## 1. Núcleo de matching

- [x] 1.1 `shared/money`: escribir los tests de `QuantityFor(base, amount, price)` (hacia abajo) y `AveragePrice(base, total, qty)` (half-up, cantidad cero) con VIB y con una moneda base de prueba con decimales, más overflow y moneda desconocida, y verlos fallar; implementarlos en `shared/money/arithmetic.go` con `math/big`; refactor en verde
- [x] 1.2 Cruce de órdenes limit: escribir los tests de «Recorrido del book» y «Límite que corta el recorrido» (`matching-engine`) y verlos fallar; implementar en `handlers/apply-commands/match.go` la price-time priority, el precio del maker, las reservas de D3 y el remanente en reposo, y en `models/trade.go` el `Trade`; refactor en verde
- [x] 1.3 Mejora de precio: escribir el test de «Mejora de precio» (`matching-engine`), que comprueba el saldo en la wallet, y verlo fallar; implementar en `handlers/apply-commands` la liberación de la diferencia en el `RELEASE` del lote; refactor en verde
- [x] 1.4 Ejecución de órdenes market: escribir los tests de «Market buy con sobrante» y «Market sell completa» (`matching-engine`) y verlos fallar; implementar en `match.go` la sell por cantidad, la buy por `amount` en unidades enteras y el remanente cancelado con `no_liquidity` y liberado; refactor en verde
- [x] 1.5 Self-trade prevention: escribir el test de «Contraparte propia» (`matching-engine`) y verlo fallar; implementar en `match.go` la cancelación del remanente de la entrante con `self_trade_prevented`, su liberación y la conservación de los trades previos; refactor en verde
- [x] 1.6 Prioridad en `ModifyOrder`: escribir los tests de «Reducción conserva el lugar», «Cambio de precio pierde el lugar» y «Nuevo precio que cruza» (`matching-engine`) y verlos fallar; implementar en `modify_order.go` que la orden modificada pase por el cruce al cambiar el precio o aumentar; refactor en verde

## 2. Eventos de ejecución

- [x] 2.1 `shared/eventlog/events`: escribir los tests de round-trip JSON de `TradeExecuted` y `OrderBookLevelChanged` y de estabilidad del `tradeId`, y verlos fallar; implementar `trades.go` con sus rutas, payloads y el `tradeId` determinístico; refactor en verde
- [x] 2.2 Emisión de eventos: escribir los tests de «Trade con dos órdenes» y «Nivel vaciado» (`trading-events`) y verlos fallar; implementar en `handlers/apply-commands` la emisión de `TradeExecuted` y `level_changes.go` (un `OrderBookLevelChanged` por nivel tocado); refactor en verde

## 3. Determinismo y rendimiento

- [x] 3.1 Determinismo: escribir el test golden de «Replay idéntico» (`matching-engine`; 10.000 comandos aleatorios con semilla fija, aplicados dos veces sobre un book vacío, dan eventos idénticos) y verlo fallar; corregir cualquier fuente de no determinismo; refactor en verde
- [x] 3.2 Agregar `go test -bench` del cruce con los perfiles A, B y C; verificar con `go test -bench . ./microservices/matching-engine/handlers/apply-commands/` que se superan 25.000 comandos/s y registrar `ns/op`, `allocs/op` y throughput en `docs/benchmark.md`

## 4. Entorno y documentación

- [x] 4.1 Agregar a `deploy/` un script `reset-dev.sh` que recrea los topics y las bases de desarrollo, y explicar en su comentario de cabecera que activar el matching requiere un entorno limpio; verificar que el script deja el entorno vacío y que una orden de compra y una de venta que cruzan publican su `TradeExecuted` con los servicios reales
- [x] 4.2 Documentar en `.claude/standards/eventlog.md` los eventos de ejecución con ejemplos JSON del caso de HU-12 del PRD; verificar que los ejemplos deserializan con `shared/eventlog/events`

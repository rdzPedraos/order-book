# Tasks

## 1. Scaffolding

- [ ] 1.1 Crear el módulo único con `go.mod` en la raíz (`github.com/rdzpedraos/order-book`), las carpetas `shared/`, `microservices/{order-service,wallet-service,matching-engine,market-service}` (cada una con `main.go` mínimo) y `tools/loadgen`; verificar que `go build ./...` desde la raíz termina sin errores y que `go build ./microservices/order-service` compila solo ese servicio
- [ ] 1.2 Crear `deploy/docker-compose.yml` con PostgreSQL y un script de init con una base por servicio; verificar con `docker compose up -d` y `psql` que existen las bases

## 2. Paquetes compartidos

- [ ] 2.1 `shared/money`: escribir tests table-driven por moneda (BRL con 2 decimales, VIB con 0) de decimales de más, negativos, cero, fuera de rango, round-trip `Parse`/`Format`, overflow en `Notional(qty, price)` y redondeo half-up del promedio, y verlos fallar; implementar el registro de monedas con su escala, `Parse`, `Format`, `Notional` y el promedio redondeado sobre `int64`; refactor en verde
- [ ] 2.2 `shared/books`: escribir tests de que `VIB-BRL`, `brl-vib` y `BRL-VIB` devuelven `BRL-VIB` y de que un book desconocido devuelve error, y verlos fallar; implementar el registro (`BRL-VIB`, base VIB, quote BRL) y `Normalize`; refactor en verde
- [ ] 2.3 `shared/identity`: escribir el test HTTP del escenario «Header ausente» (`order-management`) y verlo fallar; implementar con solo `net/http` y `context` (sin importar Gofr) el middleware estándar que exige `X-User-ID` y lo inyecta en el contexto; refactor en verde

## 3. OrderService

- [ ] 3.1 Crear la migración Gofr de `orders` con su índice; verificar que corre limpia sobre una base vacía y que `/.well-known/health` responde `200`
- [ ] 3.2 `POST /orders`: escribir los tests de los escenarios «Orden limit creada», «Envío repetido», «Base de datos no disponible», «Cantidad fraccionada», «Book desconocido», «Market buy por monto», «Tickers invertidos» y «Minúsculas» (`order-management`), y verlos fallar; implementar `handler/create_order.go`, la validación de forma en `service` y el insert en `PENDING` en `store`; refactor en verde
- [ ] 3.3 `GET /orders` y `GET /orders/{id}`: escribir los tests de «Filtro por estado», «Filtro con tickers invertidos», «Página siguiente con filtros», «Límite fuera de rango», «Detalle propio» y «Recurso ajeno» (`order-management`), y verlos fallar; implementar `handler/list_orders.go` y `handler/get_order.go`, y en `store` el listado por `cursor`/`limit` con filtros `status`, `side` y `book` y la lectura filtrada por `user_id`; refactor en verde
- [ ] 3.4 Stubs de `PATCH` y `DELETE /orders/{id}`: escribir los tests de «Cancelación aún no disponible», «Modificación con body inválido» y «Modificación de orden ajena» (`order-management`), y verlos fallar; implementar `handler/modify_order.go` y `handler/cancel_order.go` con la validación de identidad, propiedad y forma del body, y la respuesta `501 not_implemented`; refactor en verde

## 4. Documentación

- [ ] 4.1 Crear `docs/api.md` con las convenciones (header `X-User-ID`, formato de dinero, formato de errores, book canónico en orden alfabético) y los endpoints de órdenes, incluido el contrato de entrada de `PATCH`/`DELETE` y su `501` actual; verificar que cada ejemplo documentado responde lo indicado contra el servicio en compose
- [ ] 4.2 Actualizar el PRD (`docs/PDR/order-book-vibranium.md`) con los desvíos aprobados (estados `PENDING`/`REJECTED`, saldo de un trade eventualmente disponible, profundidad pública, identidad por `X-User-ID`, book canónico `BRL-VIB`); verificar que no queden contradicciones con las specs de las 8 fases

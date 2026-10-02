# Tasks

## 1. Scaffolding

- [x] 1.1 Crear el módulo único con `go.mod` en la raíz (`github.com/rdzpedraos/order-book`), las carpetas `shared/` y `microservices/order-service/` (con su `main.go`); cada servicio o herramienta se crea en la fase que lo usa; verificar que `go build ./...` desde la raíz termina sin errores y que `go build ./microservices/order-service` compila solo ese servicio
- [x] 1.2 Crear `deploy/docker-compose.yml` con PostgreSQL y un script de init con la base de OrderService (`order_service`); cada fase posterior agrega la base del servicio que la necesite; verificar con `docker compose up -d` y `psql` que existe la base

## 2. Paquetes compartidos

- [x] 2.1 `shared/money`: escribir tests por moneda (BRL con 2 decimales, VIB con 0) de decimales de más, negativos, cero, fuera de rango, moneda desconocida y round-trip `Parse`/`Format`, y verlos fallar; implementar el registro de monedas con su escala (`Currency.Decimals()`, con `ErrUnknownCurrency`), `Parse` y `Format` sobre `int64`; refactor en verde
- [x] 2.2 `shared/books`: escribir tests de que `brl-vib` y `BRL-VIB` devuelven `BRL-VIB` y de que `VIB-BRL` y un book desconocido devuelven error, y verlos fallar; implementar el registro (`BRL-VIB`, base VIB, quote BRL) y `Normalize`; refactor en verde
- [x] 2.3 `shared/identity`: escribir el test HTTP del escenario «Header ausente» (`order-management`) y verlo fallar; implementar con solo `net/http` y `context` (sin importar Gofr) el middleware estándar que exige `X-User-ID` y lo inyecta en el contexto; refactor en verde

## 3. OrderService

- [x] 3.1 Crear la migración Gofr de `orders` con su índice; verificar que corre limpia sobre una base vacía y que `/.well-known/health` responde `200`
- [x] 3.2 `POST /orders`: escribir los tests de los escenarios «Orden limit creada», «Envío repetido», «Base de datos no disponible», «Cantidad fraccionada», «Limit sin cantidad», «Limit con monto», «Venta market por monto», «Book desconocido», «Market buy por monto», «Minúsculas» y «Tickers en otro orden» (`order-management`), y verlos fallar; implementar `handler/create_order.go`, la validación de forma en `service` y el insert en `PENDING` en `store`; refactor en verde
- [x] 3.3 `GET /orders` y `GET /orders/{id}`: escribir los tests de «Filtro por estado», «Filtro en minúsculas», «Página siguiente con filtros», «Límite fuera de rango», «Detalle propio» y «Recurso ajeno» (`order-management`), y verlos fallar; implementar `handler/list_orders.go` y `handler/get_order.go`, y en `store` el listado por `cursor`/`limit` con filtros `status`, `side` y `book` y la lectura filtrada por `user_id`; refactor en verde
- [x] 3.4 Stubs de `PATCH` y `DELETE /orders/{id}`: escribir los tests de «Cancelación aún no disponible», «Modificación con body inválido» y «Modificación de orden ajena» (`order-management`), y verlos fallar; implementar `handler/modify_order.go` y `handler/cancel_order.go` con la validación de identidad, propiedad y forma del body, y la respuesta `501 not_implemented`; refactor en verde

## 4. Documentación

- [x] 4.1 Crear `docs/api.md` con las convenciones (header `X-User-ID`, formato de dinero, formato de errores, identificador del book configurado) y los endpoints de órdenes, incluido el contrato de entrada de `PATCH`/`DELETE` y su `501` actual; verificar que cada ejemplo documentado responde lo indicado contra el servicio en compose
- [x] 4.2 Actualizar el PRD (`docs/PDR/order-book-vibranium.md`) con los desvíos aprobados (estados `PENDING`/`REJECTED`, saldo de un trade eventualmente disponible, profundidad pública, identidad por `X-User-ID`, book canónico `BRL-VIB`); verificar que no queden contradicciones con las specs de las 8 fases

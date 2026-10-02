# Design

## Context

El repositorio es greenfield: solo existen el PRD ([docs/PDR/order-book-vibranium.md](../../../docs/PDR/order-book-vibranium.md)) y la configuración de OpenSpec. El stack lo fija el proyecto: Go + Gofr, APIs REST documentadas en `docs/api.md` y tests de Go. Esta es la primera de 8 fases (ver `proposal.md`), y las decisiones de estructura que se tomen aquí las heredan todas las demás.

## Goals / Non-Goals

**Goals:**

- Dejar la estructura del monorepo con OrderService y los paquetes compartidos. Los demás servicios se crean en la fase que los usa.
- Una API de órdenes simple: cada endpoint lee o escribe la tabla `orders`.

## Decisions

### D1. Monorepo Go con un solo módulo

```text
go.mod / go.sum      módulo único github.com/rdzpedraos/order-book
shared/        money, books, identity
microservices/
  order-service/
deploy/docker-compose.yml
docs/api.md
```

- **Por qué:** los tipos compartidos (dinero, books) deben ser idénticos en todos los servicios.
- **Por qué en `shared/`:** los tres paquetes son contratos entre servicios, no código de dominio. `money` y `books` fijan formatos que tienen que coincidir en todos los servicios (un precio o un book interpretado distinto es un bug de dinero), e `identity` es la convención HTTP común. Una vez definidos casi no cambian.
- **Qué no va en `shared/`:** los modelos de dominio (cada servicio tiene los suyos en su `models/`), las reglas de negocio, el SQL y las migraciones, y la conexión o los mocks de base (Gofr ya da `ctx.SQL` y `container.NewMockContainer`).
- **Un solo `go.mod` en la raíz:** todos los paquetes se importan por su path completo desde el módulo (`github.com/rdzpedraos/order-book/shared/money`). `go build ./...` y `go test ./...` corren desde la raíz, y cada servicio se compila solo con `go build ./microservices/<x>`.
- **Aislamiento:** un servicio no importa paquetes de otro. Es una regla de estructura; el compilador no la impone porque comparten módulo.
- **Alternativa descartada:** un `go.mod` por servicio (con `go.work` o con `replace`). Da caché de Docker más fina y versiones por servicio, pero multiplica los `go.mod`/`go.sum` sin necesidad para un equipo único.
- Cada servicio sigue el layout de capas de `.claude/standards/architecture.md` (`handler → service → store`).
- **Alternativa descartada:** repos separados con dependencias versionadas. Agrega fricción sin beneficio para un equipo único.

### D2. Dinero como `int64` en unidades mínimas, con escala por moneda

- **Representación:** todo monto es un `int64` en la unidad mínima de su moneda, sin `float64` en ningún punto. La cantidad de decimales la define cada moneda en un registro de `shared/money`, no un factor fijo en el código:

  | Moneda | Decimales | Unidad mínima | Ejemplo |
  | --- | --- | --- | --- |
  | BRL | 2 | centavo | `"90.00"` → `9000` |
  | VIB | 0 | 1 VIB | `10` → `10` |

  Agregar una moneda (por ejemplo COP con 0 o 2 decimales) es una entrada en ese registro, sin cambiar la lógica.
- **Precio:** se expresa en unidades mínimas de la moneda quote por 1 unidad de la base. En `BRL-VIB`, `9000` = R$ 90,00 por 1 VIB.
- **API:** los montos viajan como strings decimales (`"90.00"`). `money.Parse(currency, s)` valida contra la escala de la moneda: rechaza más decimales que los permitidos, negativos y valores fuera de rango, y `money.Format(currency, v)` arma el texto. Se usan strings porque un número JSON se lee como `float64` en muchos clientes (por ejemplo, JavaScript).
- **Moneda desconocida:** `Parse` y `Format` obtienen la escala con `currency.Decimals()`, que devuelve `money.ErrUnknownCurrency` si la moneda no está en el registro. Ninguna conversión usa una escala por defecto.
- **Aritmética:** esta fase solo parsea y formatea montos; no multiplica ni divide. La aritmética entre monedas (el monto de una cantidad a un precio, la cantidad que alcanza un monto, el precio promedio) llega con quien la usa, en las fases 3 y 4, y recibe la moneda base para reescalar con sus decimales en vez de suponer que es entera.
- **Base de datos:** columnas `BIGINT`, con la moneda implícita en la columna (`limit_price` y `amount` en la quote, `quantity` en la base).
- **Alternativa descartada:** `float64`, por los errores de redondeo (`0.1 + 0.2 != 0.3`). También una librería decimal (`shopspring/decimal`, `math/big`): es exacta, pero reserva memoria en cada operación y es mucho más lenta que un `int64` en el hot path del engine.

### D3. Registro de books

`shared/books` define los books válidos, configurados en código. Cada uno tiene su identificador (`BRL-VIB`) y sus monedas base y quote (VIB y BRL).

- **`books.Normalize(input string) (Book, error)`:** pasa a mayúsculas y busca el resultado en el registro, sin reordenar nada: `VIB-BRL` no es `BRL-VIB`. Devuelve el `Book` (identificador canónico, base y quote) o el error sentinela `books.ErrUnknownBook`.
- **Errores:** `shared/` no conoce HTTP. Solo devuelve errores sentinela (`books.ErrUnknownBook`, y en `money` errores como `money.ErrTooManyDecimals` o `money.ErrUnknownCurrency`), y es el `handler` de cada servicio el que los mapea a la respuesta: `400` con código `unknown_book`, `invalid_quantity`, etc.
- **Uso:** toda entrada de la API (body, query y path) pasa por `Normalize`, y solo el identificador configurado se guarda, se publica y se devuelve.
- **Por qué sin reordenar:** el book se identifica por lo que está configurado, no por una regla sobre el nombre. Así no hay dos formas válidas de escribir el mismo book, y la base y la quote salen del registro, no del orden del nombre.

### D4. Identidad y aislamiento

- **`shared/identity` es agnóstico del framework:** solo usa la librería estándar (`net/http` y `context`), sin importar Gofr.
  - `identity.Middleware(next http.Handler) http.Handler` lee `X-User-ID`. Si falta, responde `401` con código `missing_user_id` en el formato de error de `docs/api.md`; si está, lo guarda en el `context.Context` del request.
  - `identity.UserID(ctx context.Context) (string, error)` lo recupera, o devuelve `identity.ErrMissingUserID`.
  - Cada servicio lo registra con `app.UseMiddleware(identity.Middleware)` (Gofr acepta middlewares estándar), y el `handler` lo lee con `identity.UserID(ctx)`, porque `*gofr.Context` incluye el `context.Context` del request.
  - **Por qué:** `shared/` son contratos estables, y acoplarlos a Gofr ataría todos los servicios a una versión del framework. Además, así se testea con `httptest` sin levantar Gofr.
- **Excepción a "shared no conoce HTTP":** `identity` es la convención HTTP común, así que trabaja con `net/http`. Igual no depende de ningún framework.
- Todas las consultas filtran por `user_id`. Una orden ajena responde `404`, igual que una inexistente.
- El header es el contrato que mañana inyectará un gateway con auth real.

### D5. Tabla `orders`

Columnas:

- `id` (UUIDv7), `user_id`, `book`, `side` y `type` (deducido: `LIMIT` si hay `limit`, `MARKET` si no);
- `limit_price` (el `limit` de la API; nulo en market, y se llama así porque `limit` es palabra reservada de SQL) y `amount` (solo en market buy), en unidades mínimas de la quote, y `quantity` en unidades mínimas de la base, todos `BIGINT`;
- `filled_quantity` (`BIGINT NOT NULL DEFAULT 0`: al crear no hay nada ejecutado) y `avg_price` (`BIGINT NULL`: queda en `NULL` mientras la orden no tenga ejecuciones, porque no hay precio que promediar);
- `status`, `created_at` y `updated_at`.

Índice `orders_by_user (user_id, id DESC)` para el listado.

- **Orden por `id`:** el `id` es un UUIDv7, que lleva el timestamp de creación en sus primeros bits. Ordenar por `id` es ordenar por momento de creación, y como es único no hace falta otra columna para desempatar.
- **Paginación por `cursor` + `limit`, no por `OFFSET`:** `GET /orders?cursor=<orderId>&limit=20` devuelve las órdenes que siguen a esa en el listado, sin incluirla (`WHERE user_id = ? AND id < ? ORDER BY id DESC LIMIT ?`). Sin `cursor`, arranca desde la más reciente. `limit` va de 1 a 100 (20 por defecto). La respuesta es `{"data": [...], "metadata": {"nextCursor": "<orderId>"}}` (el formato de `response.Response` de Gofr). `nextCursor` es el `orderId` de la última orden de la página, o `null` si no hay más; el cliente lo reenvía como `cursor`. Para saber si hay más, el `store` pide `limit + 1` filas y descarta la extra.
  - **Por qué no `OFFSET`:** para llegar a la página N, `OFFSET` lee y descarta todas las filas anteriores. Además, si entra una orden nueva mientras se pagina, las páginas se corren y una orden se repite o se saltea. Con `cursor`, cualquier página cuesta lo mismo y las órdenes nuevas no desplazan nada.
  - **Parámetros explícitos:** El `cursor` es un `orderId` visible, no un token, y se combina libremente con los filtros `status`, `side` y `book`. El cliente puede pedir "desde esta orden" con cualquier filtro, sin depender de un token armado por el servidor.
  - **Nombre:** `cursor` y no `before` (confunde en un listado de más reciente a más vieja) ni `from` (sugiere que incluye la orden indicada).
  - **Alternativa descartada:** un cursor opaco (base64). Permite cambiar el formato interno sin romper clientes, pero oculta el punto de partida y obliga al cliente a pasar por la primera página para obtenerlo.
- **Filtros sin índice propio:** en esta fase toda orden está en `PENDING`, así que un índice por `status` no aporta. `side` y `book` tienen muy pocos valores y se filtran sobre el índice principal.

### D6. Sin idempotencia en la API de órdenes

Cada `POST /orders` crea una orden nueva. Un reintento tras perder la respuesta crea un duplicado, que el cliente ve en `GET /orders`.

### D7. Stubs de modificación y cancelación

`PATCH` y `DELETE` se publican ya con su contrato de entrada: identidad, propiedad de la orden y forma del body. Así los clientes integran desde la fase 1 y solo cambia la respuesta cuando la fase 2 los implementa. Responden `501` porque cambiar la orden desde OrderService crearía una race condition con su ejecución.

### D8. Acceso a Postgres con Gofr

- **Conexión y pool:** los arma Gofr desde la configuración (`DB_*` en el entorno) y los expone como `ctx.SQL`.
- **Consultas:** el `store` las escribe a mano como SQL con `ctx.SQL` (`ExecContext`, `QueryContext`, o `Select` para llenar structs).
- **Migraciones:** son funciones Go registradas con `app.Migrate(migrations.All())`, versionadas por número, y Gofr las aplica al arrancar.
- **Tests:** el `service` se testea con su interface `Store` mockeada. El `store` se testea con `container.NewMockContainer`, que da un mock SQL, y las consultas críticas (listado con `cursor`, índices) se verifican además contra un PostgreSQL real.
- **Alternativas descartadas:** sqlc (genera código tipado desde archivos `.sql`, pero suma una herramienta y un paso de generación), un ORM (oculta el SQL justo donde importan los locks y los índices) y un repositorio genérico en `shared/` (las consultas son propias de cada servicio y no entran en un CRUD genérico).

### D9. Capas de OrderService

| Capa | Componente | Responsabilidad |
| --- | --- | --- |
| `handler` | `create_order.go`, `list_orders.go`, `get_order.go`, `modify_order.go` y `cancel_order.go` (stubs) | Bind del JSON, parseo de dinero con `shared/money` y del book con `books.Normalize`, mapeo de los errores sentinela (de `shared/` y de `models`) a status y código |
| `service` | un archivo por caso de uso, como en `handler` | Reglas de la orden: combinaciones válidas de `side`, `limit`, `quantity` y `amount`, y el `type` deducido |
| `store` | `orders.go` | SQL de `orders`: insert, listado con `cursor`/`limit` y filtros, y lectura propia |
| `models` | `order.go`, `errors.go` | `Order`, `Status` y los errores de dominio (`ErrOrderNotFound` y los errores de validación) |
| `migrations` | `orders` | Tabla e índice de D5 |

- La validación de forma vive en `service`, no en `handler`, para testearla sin HTTP.
- `service` define la interface `Store` y `handler` la interface `Service`; cada capa se testea con la de abajo mockeada.

### D10. Aserciones de tests con testify

- **Decisión:** los tests usan `github.com/stretchr/testify/assert` (`assert.Equal`, `assert.NoError`, `assert.ErrorIs`) en lugar de `if` con `t.Error`.
- **Por qué:** cada comprobación queda en una línea y, si falla, testify imprime el valor esperado y el obtenido sin escribir un mensaje a mano.
- **Costo:** ya está en `go.mod` como dependencia indirecta de Gofr; pasa a ser directa, sin sumar un módulo nuevo.
- **Alternativa descartada:** solo la librería estándar (`if` + `t.Errorf`), que obliga a formatear cada mensaje de fallo.

## Risks / Trade-offs

- **[`X-User-ID` sin verificar permite usar IDs inventados]** → Aceptado en el MVP. Solo se despliega en una red privada.

## Migration Plan

Greenfield. Se levanta `docker compose up` con PostgreSQL, se corren las migraciones de OrderService y se inicia el servicio. Rollback: detener el servicio; los datos quedan en PostgreSQL.

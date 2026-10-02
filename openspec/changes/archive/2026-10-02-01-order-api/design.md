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
shared/        money, books, identity, fault
microservices/
  order-service/
deploy/docker-compose.yml
docs/api.md
```

- **Por qué:** los tipos compartidos (dinero, books) deben ser idénticos en todos los servicios.
- **Por qué en `shared/`:** los tres paquetes son contratos entre servicios, no código de dominio. `money` y `books` fijan formatos que tienen que coincidir en todos los servicios (un precio o un book interpretado distinto es un bug de dinero), e `identity` es la convención HTTP común. Una vez definidos casi no cambian.
- **Qué no va en `shared/`:** los modelos de dominio (cada servicio tiene los suyos en su `models/`), las reglas de negocio, el SQL y las migraciones, y la conexión a la base (Gofr ya da `ctx.SQL`).
- **Un solo `go.mod` en la raíz:** todos los paquetes se importan por su path completo desde el módulo (`github.com/rdzpedraos/order-book/shared/money`). `go build ./...` y `go test ./...` corren desde la raíz, y cada servicio se compila solo con `go build ./microservices/<x>`.
- **Aislamiento:** un servicio no importa paquetes de otro. Es una regla de estructura; el compilador no la impone porque comparten módulo.
- **Alternativa descartada:** un `go.mod` por servicio (con `go.work` o con `replace`). Da caché de Docker más fina y versiones por servicio, pero multiplica los `go.mod`/`go.sum` sin necesidad para un equipo único.
- Cada servicio sigue el layout de `.claude/standards/architecture.md`: un paquete por entrada en `handlers/<ruta>/` que llama al `store`, sin una capa de reglas aparte (ver D9).
- **Alternativa descartada:** repos separados con dependencias versionadas. Agrega fricción sin beneficio para un equipo único.

### D2. Dinero como `int64` en unidades mínimas, con escala por moneda

- **Representación:** todo monto es un `int64` en la unidad mínima de su moneda, sin `float64` en ningún punto. La cantidad de decimales la define cada moneda en un registro de `shared/money`, no un factor fijo en el código:

  | Moneda | Decimales | Unidad mínima | Ejemplo |
  | --- | --- | --- | --- |
  | BRL | 2 | centavo | `"90.00"` → `9000` |
  | VIB | 0 | 1 VIB | `"10"` → `10` |

  Agregar una moneda (por ejemplo COP con 0 o 2 decimales) es una entrada en ese registro, sin cambiar la lógica.
- **Precio:** se expresa en unidades mínimas de la moneda quote por 1 unidad de la base. En `BRL-VIB`, `9000` = R$ 90,00 por 1 VIB.
- **API:** los montos y las cantidades viajan como strings decimales (`"90.00"`, `"10"`), también en el request. `money.Parse(currency, value *string) (*int64, error)` valida contra la escala de la moneda (un campo que no se envió llega `nil` y vuelve `nil`): rechaza más decimales que los permitidos, negativos y valores fuera de rango, y `money.Format(currency, v)` arma el texto. Se usan strings porque un número JSON se lee como `float64` en muchos clientes (por ejemplo, JavaScript).
- **Moneda desconocida:** `Parse` y `Format` obtienen la escala con `currency.Decimals()`, que devuelve `money.ErrUnknownCurrency` si la moneda no está en el registro. Ninguna conversión usa una escala por defecto.
- **Aritmética:** esta fase solo parsea y formatea montos; no multiplica ni divide. La aritmética entre monedas (el monto de una cantidad a un precio, la cantidad que alcanza un monto, el precio promedio) llega con quien la usa, en las fases 3 y 4, y recibe la moneda base para reescalar con sus decimales en vez de suponer que es entera.
- **Base de datos:** columnas `BIGINT`, con la moneda implícita en la columna (`limit_price` y `amount` en la quote, `quantity` en la base).
- **Alternativa descartada:** `float64`, por los errores de redondeo (`0.1 + 0.2 != 0.3`). También una librería decimal (`shopspring/decimal`, `math/big`): es exacta, pero reserva memoria en cada operación y es mucho más lenta que un `int64` en el hot path del engine.

### D3. Registro de books

`shared/books` define los books válidos, configurados en código. Cada uno tiene su identificador (`BRL-VIB`) y sus monedas base y quote (VIB y BRL).

- **`books.Normalize(input string) (Book, error)`:** pasa a mayúsculas y busca el resultado en el registro, sin reordenar nada: `VIB-BRL` no es `BRL-VIB`. Devuelve el `Book` (identificador canónico, base y quote) o el error sentinela `books.ErrUnknownBook`.
- **Errores:** todos los errores, también los de `shared/` (`books.ErrUnknownBook`, `money.ErrTooManyDecimals`, `money.ErrUnknownCurrency`), son valores `fault.Error` con su status y su código, así cualquier servicio que los responda por HTTP ya sabe cómo. Un consumidor que no es HTTP, como el engine, los usa como cualquier `error`. Los errores de dominio de `models` también son `fault.Error` (`invalid_quantity`, `order_not_found`), y el `handler` devuelve `fault.From(err)`.
- **Uso:** toda entrada de la API (body, query y path) pasa por `Normalize`, y solo el identificador configurado se guarda, se publica y se devuelve.
- **Por qué sin reordenar:** el book se identifica por lo que está configurado, no por una regla sobre el nombre. Así no hay dos formas válidas de escribir el mismo book, y la base y la quote salen del registro, no del orden del nombre.

### D4. Identidad y aislamiento

- **`shared/identity` es agnóstico del framework:** solo usa la librería estándar (`net/http` y `context`), sin importar Gofr.
- **`shared/fault` define el formato de error de `docs/api.md` una sola vez:** `fault.Error` implementa `StatusCode()` y `Response()`, que Gofr usa para renderizarlo, y `Write(w)` para los middlewares `net/http` como `identity`. Así el error de un handler y el de un middleware salen con el mismo JSON.
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
- **Paginación por `cursor` + `limit`, no por `OFFSET`:** `GET /orders?cursor=<orderId>&limit=20` devuelve las órdenes que siguen a esa en el listado, sin incluirla (`WHERE user_id = ? AND id < ? ORDER BY id DESC LIMIT ?`). Sin `cursor`, arranca desde la más reciente. `limit` va de 1 a 100 (20 por defecto). La respuesta es `{"data": [...], "metadata": {"nextCursor": "<orderId>"}}` (el formato de `response.Response` de Gofr). `nextCursor` es el `orderId` de la última orden de la página, o `null` si no hay más; el cliente lo reenvía como `cursor`. Para saber si hay más, el handler pide al `store` `limit + 1` filas y descarta la extra.
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
- **Consultas:** `store/orderdb` las escribe a mano como SQL con `ctx.SQL` (`ExecContext`, `QueryContext`, `QueryRowContext`) y las expone como funciones del paquete (`orderdb.InsertOrder`, `orderdb.ListOrders`, `orderdb.GetOrder`). Los handlers las llaman directamente, sin interfaces ni constructores.
- **Migraciones:** son funciones Go registradas con `app.Migrate(migrations.All())`, versionadas por número, y Gofr las aplica al arrancar.
- **Tests:** `orderdb` guarda la base en una variable del paquete, y `orderdb.InitMock(t)` la reemplaza durante un test por una base en memoria: registra lo escrito (`mock.Orders`), se puede sembrar con órdenes existentes y hace fallar todas las llamadas con `mock.Err`. Los tests de cada handler usan ese mock. El SQL de `orderdb` se prueba contra PostgreSQL con la etiqueta `integration` (insert, listado con filtros y `cursor`, lectura propia y ajena), y la cobertura se mide con esos tests.
- **Alternativa descartada para los tests:** mocks de SQL (`container.NewMockContainer` con sqlmock). Comparan el texto de la consulta, así que se rompen cuando se reescribe sin cambiar lo que hace, y no detectan un SQL que no funciona contra PostgreSQL.
- **Alternativas descartadas:** sqlc (genera código tipado desde archivos `.sql`, pero suma una herramienta y un paso de generación), un ORM (oculta el SQL justo donde importan los locks y los índices) y un repositorio genérico en `shared/` (las consultas son propias de cada servicio y no entran en un CRUD genérico).

### D9. Estructura de OrderService

| Paquete | Contenido | Responsabilidad |
| --- | --- | --- |
| `handlers/create-order`, `list-orders`, `get-order`, `modify-order`, `cancel-order` | `handler.go` + `handler_test.go` | Cada uno arma su `request` con lo que recibe (body, ruta, query y usuario), lo convierte una sola vez (dinero con `shared/money`, book con `books.Normalize`; un valor que no se puede convertir responde el error de su campo), llama a `orderdb` y responde. `create-order` arma el `models.Order` y llama a `order.Validate()`; `modify-order` y `cancel-order` validan identidad, propiedad y body y responden `501` (D7) |
| `models` | `order.go`, `order_json.go`, `errors.go` | `Order` y sus tipos; `Order.Validate()` con las reglas de la orden (`side` válido, combinaciones permitidas de `limit`, `quantity` y `amount` → `unsupported_order`, valores mayores que cero); el JSON de la API (`MarshalJSON`, montos y cantidades como strings decimales); los errores de dominio como `fault.Error` |
| `store/orderdb` | `main.go`, `postgres.go`, `mock.go` | Las funciones públicas y `ListQuery` (los filtros del listado), el SQL de `orders` contra PostgreSQL y el mock en memoria de D8 |
| `migrations` | `orders` | Tabla e índice de D5 |

- **Sin capa de reglas aparte:** las reglas de cada caso de uso viven en su handler, junto a la entrada que validan, y las reglas de la orden en `models.Order`. La forma de escribir un handler está en `.claude/standards/handlers.md`.
- **Errores:** los de varios handlers o del store en `models`; los de un solo handler, como `invalid_cursor`, en su `handler.go`; un error de base de datos se envuelve como `fault.ErrServiceUnavailable` y el handler responde `fault.From(err)`.

### D10. Aserciones de tests con testify

- **Decisión:** los tests usan `github.com/stretchr/testify/require` mediante `c := require.New(t)` al inicio de cada `t.Run` (`c.Equal`, `c.NoError`, `c.ErrorIs`, `c.JSONEq`), en lugar de `if` con `t.Error`.
- **Por qué:** cada comprobación queda en una línea y, si falla, testify imprime el valor esperado y el obtenido; `require` corta el caso en el primer fallo, en vez de seguir con datos inválidos como hace `testify/assert`.
- **Costo:** ya está en `go.mod` como dependencia indirecta de Gofr; pasa a ser directa, sin sumar un módulo nuevo.
- **Alternativa descartada:** solo la librería estándar (`if` + `t.Errorf`), que obliga a formatear cada mensaje de fallo.

## Risks / Trade-offs

- **[`X-User-ID` sin verificar permite usar IDs inventados]** → Aceptado en el MVP. Solo se despliega en una red privada.

## Migration Plan

Greenfield. Se levanta `docker compose up` con PostgreSQL, se corren las migraciones de OrderService y se inicia el servicio. Rollback: detener el servicio; los datos quedan en PostgreSQL.

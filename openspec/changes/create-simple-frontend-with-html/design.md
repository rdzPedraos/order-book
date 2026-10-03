# Design

## Context

La API ya está completa y se sirve por un solo host con un ingress por path (`06-container-infrastructure`, D3): `/orders`, `/wallet` y `/market`. No hay CORS en ningún servicio, y `X-User-ID` lo manda el cliente y el ingress lo reenvía sin tocarlo. Motivación en `proposal.md`; comportamiento en `specs/web-ui/spec.md`.

El diseño visual está aprobado en el artifact de Claude Design `https://claude.ai/artifact/YEDHKxykqQjURXgon1B1Az` (privado). Su código fuente se guarda en `mockup/Main.dc.html` dentro de este change, como referencia: usa el runtime de Claude Design (`<x-dc>`, `<sc-for>`), así que no corre fuera de él y **no se copia tal cual**; se traduce a HTML, CSS y JavaScript planos.

## Goals / Non-Goals

**Goals:**

- La menor cantidad de lógica posible en el cliente: leer, pintar, enviar.
- Lo único con reglas (montos, validación, cuerpo de la orden) en funciones puras con tests.
- Sin build, sin dependencias de npm, sin framework.

**Non-Goals:**

- Un SPA con router, store o componentes.
- Compartir código entre la página y los servicios Go.

## Decisions

### D1. Un servicio `client` que sirve archivos estáticos

`microservices/client` es un servicio Gofr como los demás, con solo `main.go` y una carpeta `static/`. `main.go` crea la app, llama a `app.AddStaticFiles` y corre. Gofr resuelve la ruta contra el directorio de trabajo, así que busca `./static` (la imagen, y correr desde la carpeta del servicio) y, si no existe, `./microservices/client/static` (correr desde la raíz del repo); no tiene handlers, store ni base de datos, y no importa nada de `shared/`.

| Ruta | Qué es |
| --- | --- |
| `microservices/client/main.go` | Wiring: `gofr.New()`, `AddStaticFiles`, `Run`. Lleva el package comment (`// Command client ...`) y `buildApp()` para poder probarlo, como `wallet-service` |
| `microservices/client/static/` | Los archivos de la página. Son assets, no una capa nueva: no hay Go dentro |
| `microservices/client/configs/.env.example` | `APP_NAME`, `HTTP_PORT` y `METRICS_PORT`, como los demás |
| `microservices/client/Dockerfile` | Mismo patrón multi-stage; copia `static/` a `/static` y deja `WORKDIR /`, que es donde Gofr la busca |

- **Por qué un servicio y no nginx:** la arquitectura define servicios Gofr, con la misma imagen base, probes y Dockerfile que los otros cuatro. Gofr ya sirve `index.html` de un directorio y expone `/.well-known/alive` y `/.well-known/health` para las probes.
- **Alternativas descartadas:** (a) una imagen de nginx: agrega otra base, otro archivo de configuración y otro patrón de probes; (b) servir la página desde `market-service` u otro servicio: acopla un servicio de datos con la UI y la hace escalar junto a él; (c) un ConfigMap montado en el ingress: no se versiona ni se prueba como una imagen.
- **No va a `shared/`:** solo lo usa este servicio.

### D2. Tres archivos de página y tres módulos de JavaScript

```text
microservices/client/static/
  index.html          markup: header, hero, tarjeta Operar, tarjeta Mi cuenta
  app.css             estilos y variables de color y tipografía
  js/
    amount.js         puro: parseAmount, formatAmount (BigInt, por moneda)
    order.js          puro: buildOrderBody, getReserve, validateOrder, getVisibleFields
    deposit.js        puro: buildDepositBody, validateDeposit
    name.js           puro: chooseName, loadName, saveName (el storage se recibe como parámetro)
    orders.js         puro: isActive, filterOrders, describeOrder (orden de la API → fila a pintar)
    present.js        puro: getLevelRows, getSpread, getBalanceRows, getAvailableFunds, describeMovement (datos de la API → filas a pintar)
    readings.js       puro: applyReadings (una lectura que falló no borra lo último que se mostró)
    api.js            fetch con X-User-ID y errores; sin lógica
    app.js            estado, polling, pintar el DOM, eventos
```

- Los módulos son ES modules (`<script type="module">`): nada que compilar. Cada archivo queda por debajo de 300 líneas.
- **Lo puro se prueba, el wiring no.** `amount.js`, `order.js`, `deposit.js`, `orders.js`, `name.js`, `present.js` y `readings.js` no tocan el DOM ni la red (el `localStorage` entra como parámetro) y tienen sus `*.test.js` al lado. `api.js` y `app.js` son wiring, como `main` en Go, y quedan fuera del umbral de cobertura; sus escenarios se comprueban en la prueba de humo.
- **Dinero:** los montos son strings decimales y nunca `Number`. `parseAmount(text, decimals)` devuelve `BigInt` en unidades mínimas (centavos de BRL, VIB enteros) o `null`; `formatAmount` hace el camino inverso. Los decimales vienen de una tabla `{BRL: 2, VIB: 0}`, no de un `* 100`. Mismo criterio que `shared/money`.
- **Salida de texto:** todo lo que escribe la persona (el nombre) o devuelve la API (mensajes de error) se pinta con `textContent`, nunca con `innerHTML`.

### D3. Polling: una vuelta que lo repinta todo

```text
cada 2 s (y justo después de una escritura 201):
  GET /market/orderbook/BRL-VIB?depth=5      ┐
  GET /wallet                                ├─ en paralelo, con Promise.allSettled
  GET /wallet/movements?limit=5              │  (las tres últimas solo si hay nombre)
  GET /orders?limit=50                       ┘
  por cada respuesta exitosa → reemplaza su parte del estado y repinta su zona
```

- Un `setInterval(refresh, 2000)` y nada más. Si la vuelta anterior sigue en vuelo, la siguiente se salta (una bandera), para no apilar peticiones.
- **Las filas del libro se crean una sola vez** (6 por lado) y cada vuelta solo cambia su texto y el ancho de su barra, con una transición corta: la lista no parpadea ni cambia de altura aunque el libro cambie o un lado quede vacío.
- **Se repinta cada zona de datos** (libro, saldos, movimientos), **nunca los campos del formulario**, así lo que la persona está escribiendo no se pierde.
- **Sin merge ni diff:** la respuesta reemplaza lo anterior. Si una lectura falla, esa zona conserva lo último que mostró y se reintenta en la próxima vuelta; no hay reintentos ni backoff.
- **Después de una escritura** se llama al mismo `refresh()`. La API documenta que una lectura puede ir unos milisegundos atrasada; la vuelta de 2 s la corrige sola.
- **Qué se pide:** `depth=6` por lado y `limit=5` de movimientos, por la maqueta.
- **Alternativas descartadas:** SSE o WebSocket (la API no los tiene y agregaría un servicio de push); pausar el polling con la pestaña oculta (más lógica por una demo); refrescar solo lo que cambió (exige comparar).

### D4. Estado mínimo

Un objeto en memoria: `{ name, book, wallet, movements, busy }`.

- `name` vive también en `localStorage` (`vibranium.name`), leído al arrancar con `loadName` y escrito al cambiarlo con `saveName`, dentro de `try/catch`: si el navegador lo bloquea, la página sigue y pide el nombre en cada visita. `chooseName(draft, current)` decide el nombre vigente: el borrador sin espacios sobrantes, o el actual si queda vacío.
- Comprar o Vender y Límite o Mercado son dos grupos de `<input type="radio">` con aspecto de pestañas: el DOM ya guarda su valor, así que no hay estado propio. Cuando cambian, `getVisibleFields(side, type)` dice qué campos mostrar (`limit`, `quantity`, `amount`) y `app.js` los muestra u oculta con el atributo `hidden`.
- Por defecto: Comprar y Límite.

### D5. La orden y su validación

- `buildOrderBody(side, type, values)` arma el JSON exacto de la tabla de `docs/api.md`: límite → `limit` + `quantity`; compra a mercado → `amount`; venta a mercado → `quantity`. Siempre con `book: "BRL-VIB"`.
- `getReserve(side, type, values)` calcula lo que se congela: compra a límite `quantity × limit` en BRL, compra a mercado `amount` en BRL, venta `quantity` en VIB. Se usa para el texto "Reservamos…" y para comprobar el saldo.
- `validateOrder(...)` devuelve el texto del problema o `''`: valor que no es un número mayor que 0 con los decimales de la moneda, o reserva mayor que `available` de la última lectura del wallet. No reemplaza a la API, solo evita el viaje y da un mensaje claro.
- **El rechazo asíncrono por fondos** (`REJECTED`, `insufficient_funds`) no se consulta: no se hace seguimiento de la orden. Ver riesgos.

### D5b. Mis órdenes

- **Lectura:** `GET /orders?limit=50`, las 50 más recientes. `GET /orders` filtra por un solo `status` y las activas son tres, así que `filterOrders(orders, 'ACTIVE' | 'ALL')` filtra en la página. El filtro es un par de radios (Activas | Todas, Activas por defecto) y no genera llamadas.
- **Cada fila** sale de `describeOrder(order)`: lado, descripción (`Compra 10 VIB · límite R$ 90.00`, `Compra de mercado · gasta R$ 500.00`), estado con su etiqueta, lo ejecutado (`filledQuantity` de `quantity` y `avgPrice`), la razón de las canceladas y rechazadas, y `canCancel`. La razón viene de `reason` (`no_liquidity`, `insufficient_funds`, `invalid_amount`); una orden `CANCELLED` sin `reason` se muestra como «Cancelada por ti».
- **Cancelar:** `POST /orders/{id}/close`. La API responde `201` con la orden sin cambios, porque la cancelación se aplica de forma asíncrona; la página avisa que pidió la cancelación y refresca de inmediato. Mientras la lectura siga trayendo la orden activa, la página guarda en memoria que se pidió cancelarla y, en lugar del botón, muestra «Cancelación pedida»: la orden no se oculta ni cambia de estado, porque el estado que se muestra sigue siendo el que la API leyó. Las lecturas llegan de proyecciones que pueden ir atrasadas (con mucha carga, minutos), y sin este aviso la persona pulsaría Cancelar otra vez sin saber que ya está pedido. La marca se olvida cuando la orden llega final. Pulsarlo dos veces es seguro (la API lo documenta).
- **Alternativas descartadas:** pedir una lista por cada estado activo (3 llamadas por vuelta); ocultar la orden al pulsar Cancelar sin esperar a la API (mostraría un estado que puede no ser real, porque la orden pudo ejecutarse antes).

### D6. Cargar dinero

Un selector BRL | VIB, un campo y un botón. Reutiliza `parseAmount` con los decimales de la moneda elegida y llama a `POST /wallet/deposits`. Al cambiar la moneda el campo se prellena con un valor razonable (`150.00` o `10`) y la persona lo edita o confirma.

### D7. Entrega: imagen, Helm e ingress

- **Dockerfile** de `microservices/client`: mismo patrón que los otros, copiando `microservices/client/` (con `static/`) y construyendo `./microservices/client`.
- **Helm** (`deploy/helm/orderbook`): una entrada `client` en `deployments` (`service: client`, `http: true`, 2 réplicas) y `/: client` en `ingress.routes`. Con ingress-nginx gana el prefijo más largo, así que `/orders`, `/wallet` y `/market` siguen yendo a la API y solo lo demás llega a la página.
- **El pod `client` no recibe `DB_*` ni brokers.** Hoy todo Deployment toma el ConfigMap `common` (con `DB_HOST`) y el secreto de la base, y Gofr intentaría conectarse a PostgreSQL. El template pasa a omitir `DB_NAME`, `DB_PASSWORD` y el `envFrom` de `common` cuando el Deployment no declara `database`, y `client` define por sí mismo `HTTP_PORT`, `METRICS_PORT` y `GOFR_TELEMETRY` en su `env`. Se verifica con `helm template`.
- **Local:** el camino soportado es el cluster con ingress (`http://orderbook.local`), donde la página y la API comparten origen. Correr `go run ./microservices/client` solo sirve la página, y sus llamadas a la API no llegarían: no se agrega CORS ni un proxy por eso.

### D8. Pruebas, siguiendo `go.md`

- **Go:** `microservices/client/main_test.go` levanta `buildApp()` como `wallet-service` y comprueba que `GET /` responde la página y que `GET /app.css` responde el CSS. `main` queda fuera del umbral de 85 %, pero el escenario "Abrir la página" queda cubierto.
- **JavaScript:** `node --test` sobre `static/js/*.test.js`, con `node:assert` y un `test()` explícito por escenario del spec, sin tabla de casos con `for`, igual que en Go. Se mide cobertura de los siete módulos puros con `--experimental-test-coverage` y deben llegar al 85 %.
- **Por qué `node --test`:** viene dentro de Node, no instala paquetes y su único `package.json`, en `microservices/client/` (fuera de `static/`, así que no se sirve), solo declara `"type": "module"` para que `node` lea los módulos, sin dependencias. Node ya existe en los runners de GitHub Actions. Alternativas descartadas: no probar el JavaScript (rompe la regla de TDD del proyecto), un motor de JavaScript embebido en Go como `goja` (una dependencia Go por probar tres funciones), o un framework (Jest, Vitest: `node_modules` y build).
- **CI:** un paso nuevo en `.github/workflows/test.yml` que corre los tests de JavaScript y su cobertura.
- **Manual (no automatizable aquí):** una prueba de humo en el cluster, con los pasos de `tasks.md`, porque no hay navegador en CI.

### D9. Traducción del diseño

Valores fijos de la maqueta, para no depender del artifact:

| Elemento | Valor |
| --- | --- |
| Amarillo (header y hero) | `#FFE600` |
| Azul oscuro (títulos, saldos) | `#2D3277` |
| Azul (botones de acción) | `#2968C8` |
| Verde de compra / rojo de venta | `#00803F` / `#D6293C`, con texto blanco (contraste ≥ 4.5:1) |
| Fondo / texto / texto secundario | `#F4F4F6` / `#33355C` / `#5F6288` |
| Títulos / cuerpo | Bricolage Grotesque 800 / Figtree, de Google Fonts, con `system-ui` de respaldo |

- **Estructura:** header con logo y nombre; hero amarillo con título y el campo del nombre; debajo dos tarjetas: *Operar* (Comprar | Vender, Límite | Mercado, campos, Confirmar, y el volumen del mercado en una sola lista: ventas en rojo arriba, spread en medio, compras en verde abajo, con barras del volumen acumulado) *Mi cuenta* (saldos BRL y VIB, Cargar dinero, 5 movimientos), y debajo, a todo el ancho, *Mis órdenes* (filtro Activas | Todas, lista y botón Cancelar).
- Las variables van como custom properties en `:root`. En pantalla estrecha las dos tarjetas se apilan (`flex-wrap`), sin scroll horizontal.
- Las fuentes se cargan de Google Fonts: la demo necesita internet para verlas con la tipografía del diseño; sin red cae a `system-ui` y todo funciona.

## Risks / Trade-offs

- **[Una orden rechazada por fondos no se nota]** El engine puede rechazarla de forma asíncrona aunque la página haya comprobado el saldo (otra persona con el mismo nombre gastó el saldo en medio). → Aceptado: el dinero no se reserva, no aparece el movimiento `RESERVE` y el saldo no cambia. Si molesta en la demo, se puede consultar `GET /orders/{id}` hasta que salga de `PENDING`; queda fuera de este change.
- **[Dos personas con el mismo nombre comparten cuenta]** → Aceptado en una demo; el nombre solo identifica.
- **[Carga de 4 lecturas cada 2 s por pestaña]** ≈ 2 req/s por pestaña abierta, una de ellas de hasta 50 órdenes. → Aceptable con 2 réplicas de `market` y de `wallet-api`; se vigila en el benchmark de la demo.
- **[La maqueta y la página se separan]** El artifact queda como referencia, no se mantiene sincronizado. → La fuente guardada en `mockup/` y la tabla de D9 son suficientes para rehacer un cambio visual.
- **[Sin pruebas en un navegador real]** → La lógica con riesgo está en funciones puras probadas; el resto se cubre con la prueba de humo manual.
- **[El template del chart hoy asume base de datos en todo Deployment]** → Se corrige en D7 y se verifica con `helm template`.

## Migration Plan

Se construye la imagen `orderbook/client`, se actualiza el release con `helm upgrade` y la página queda en `http://orderbook.local/`. Rollback: `helm rollback`; el resto del sistema no cambia.

## Open Questions

- **`static/` dentro de un servicio.** `architecture.md` define las capas `handlers`, `store`, `models` y `utils`, y no tiene un lugar para assets. Aquí se tratan como archivos del servicio, no como una capa nueva: el Go es solo `main.go`. Conviene confirmarlo al revisar; si se prefiere otra ubicación, solo cambia la ruta.
- **Mejor bid y ask destacados.** La maqueta final quitó la tarjeta del hero, y el spec pide el mercado como listas con el mejor precio primero. Si se quiere un resumen "Vender a / Comprar a" arriba, es un requisito adicional sin cambios de API.

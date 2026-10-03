# Proposal

## TL;DR

Una página web de un solo archivo HTML, sin framework ni build, para la demo del MVP: la persona elige un nombre ficticio (su `X-User-ID`), ve el mercado, compra o vende y ve cómo se mueve su cuenta. La lógica es la mínima: la página consulta la API cada 2 s y repinta todo con lo que responde. La sirve un servicio nuevo, `client`, detrás del mismo ingress y host que la API, así que no hace falta CORS. El diseño aprobado está en el artifact de Claude Design (ver `design.md`).

## Why

La API de órdenes, wallet y market ya cubre todo el flujo, pero hoy solo se puede probar con `curl`. Para mostrar el MVP hace falta una pantalla que haga visible el libro, las órdenes y el dinero moviéndose, sin que la demo dependa de que alguien escriba comandos.

## Goals

- Elegir un nombre ficticio y usarlo como `X-User-ID` en todas las llamadas.
- Ver el libro (`bids` y `asks` con su volumen) y el dinero de la cuenta (saldos y movimientos) siempre al día, sin recargar.
- Comprar y vender en los 4 casos de la API: compra y venta, cada una a límite o a mercado.
- Cargar dinero: BRL o VIB.
- Máximo dos clics para operar: elegir Comprar o Vender y confirmar (con Límite preseleccionado).

## Non-Goals

- Login real, sesiones o contraseñas: el nombre es solo un identificador.
- Retiros, cambiar una orden, cancelar una orden y lista de órdenes. La API los tiene, la UI de la demo no.
- Framework de frontend, bundler, TypeScript o estado global.
- Tiempo real por WebSocket o SSE: la actualización es por polling.
- Diseño para uso en producción (accesibilidad completa, i18n, temas): es una demo.
- Correr la UI contra servicios locales sueltos (cada uno en su puerto): el camino soportado es el stack del Helm chart, con un solo host.

## What Changes

- **Servicio nuevo `microservices/client`** (Go + Gofr): solo sirve la carpeta `static/` en `/`. No tiene handlers, store ni base de datos.
- **`static/index.html`, `static/app.css` y `static/app.js`**: la página, convertida del artifact a HTML, CSS y JavaScript planos.
- **Polling cada 2 s** a `GET /market/orderbook/BRL-VIB`, `GET /wallet` y `GET /wallet/movements`. Cada vuelta repinta mercado, saldos y movimientos.
- **Dos escrituras**: `POST /orders` y `POST /wallet/deposits`, con `X-User-ID` y los montos como strings decimales.
- **Helm**: un Deployment `client` y la ruta `/` del ingress hacia él.
- **Dockerfile** del servicio `client`, con el mismo patrón que los demás.

## Capabilities

### New Capabilities

- `web-ui`: la página de la demo: identificación por nombre, mercado, operar, cuenta y refresco periódico.

### Modified Capabilities

Ninguna. La API de `market-data`, `wallet` y `order-management` no cambia; la página solo la consume.

## Concepts

- **Nombre ficticio:** texto libre que la persona escribe. Se guarda en `localStorage` y se manda como `X-User-ID`. Dos personas con el mismo nombre comparten cuenta; es aceptable en una demo.
- **Polling:** `setInterval` de 2 s. Cada respuesta reemplaza el estado anterior; no hay merge ni diff.
- **Refrescar todo:** en cada vuelta se vuelven a pedir y a pintar las tres lecturas, y también justo después de una escritura.
- **Mismo origen:** la página y la API salen del mismo host por el ingress, así que el navegador no necesita CORS y las rutas son relativas (`/market/...`).

## Assumptions

- Depende de `06-container-infrastructure` (aún en curso) (Dockerfiles, chart e ingress). Sin el ingress por path, la página no tiene dónde vivir junto a la API.
- El único book es `BRL-VIB`; la página lo lleva fijo.
- Las lecturas pueden ir un instante atrasadas respecto de una escritura (la API lo documenta). El próximo polling lo corrige, por eso no hay lógica de reintento.
- Una orden puede ser rechazada de forma asíncrona (`insufficient_funds`). La página lo evita comparando con el saldo disponible que ya leyó; si aun así el engine la rechaza, el dinero simplemente no se reserva y el movimiento no aparece.
- Los dos clics cuentan sin contar el llenado de campos. Cambiar de Límite a Mercado o de Comprar a Vender suma un clic.

## Impact

- **Código:** `microservices/client` (nuevo), `deploy/helm/orderbook` (Deployment y ruta `/`).
- **APIs:** ninguna cambia.
- **Dependencias:** ninguna nueva en Go. Para probar el JavaScript se usa `node --test` (integrado en Node, sin paquetes); ver `design.md`.
- **Docs:** `docs/ui.md` (nuevo): cómo abrir la UI y qué consulta.

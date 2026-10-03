# Design

## Context

Desde la fase 5 el sistema está completo, pero se levanta pieza por pieza: `docker-compose.yml` solo tiene PostgreSQL y Redpanda. Esta fase lo empaqueta, lo lleva a un cluster y lo mide. Compose no crece: sigue siendo la infraestructura para desarrollar y para los tests de integración. Motivación en `proposal.md`.

## Goals / Non-Goals

**Goals:**

- Todo el stack en un cluster de Kubernetes, kind en local.
- Un benchmark reproducible con resultados versionados.

**Non-Goals:**

- Autoscaling. El engine escala por book, no por réplicas, y los demás servicios se escalan a mano en el MVP.

## Decisions

### D1. Imágenes

Un `Dockerfile` multi-stage por servicio (`microservices/<x>/Dockerfile`), construido con el contexto en la raíz del repo: un build con `golang` y un runtime `distroless/static` (el engine pasa a `distroless/cc` en la fase 8 por cgo).

- **Qué copia:** primero `go.mod` y `go.sum` (con su capa de `go mod download`), luego solo `shared/` y `microservices/<x>/`, y corre `go build ./microservices/<x>`. La imagen solo contiene ese binario.
- **Caché:** la capa de dependencias es común a todos los servicios (hay un solo `go.mod`), así que se invalida en todas las imágenes cuando cambia una dependencia. Es poco frecuente y aceptable. Cambios en el código de un servicio solo reconstruyen su imagen.
- **Por qué uno por servicio:** cada imagen se construye y despliega sola. En CI se reconstruye `<x>` cuando cambia `microservices/<x>/**` o `shared/**`, y todas cuando cambia `go.mod`.
- **cgo solo en el engine:** RocksDB (fase 8) se descarga para todos, pero solo se compila en el engine, que es el único que lo importa. Los demás siguen con `CGO_ENABLED=0`.
- **Alternativa descartada:** un único `Dockerfile` parametrizado por `SERVICE` que copia todo el repo. Invalida la caché de todas las imágenes ante cualquier cambio.

### D2. Topología en Kubernetes

| Componente | Recurso | Por qué |
| --- | --- | --- |
| OrderService `api`, WalletService `api`, MarketService | Deployment, 2 réplicas, HPA desactivado | Stateless. Las réplicas de OrderService (`ROLE=api`) publican en `orders.commands` y no consumen; las de WalletService (`ROLE=api`) solo tienen las rutas públicas. |
| WalletService `funds` | Deployment, 2 réplicas, Service interno | `ROLE=funds`: solo `funds:batch`. Fuera del ingress, así que solo se llega desde dentro del cluster. |
| OrderService `projector` | Deployment, 1 réplica | `ROLE=projector`: proyecta `orders.commands` en `orders`. A lo sumo una réplica por partición; más quedarían ociosas en el consumer group. Sin rutas no abre el puerto HTTP, así que su probe usa `/metrics`. |
| WalletService `trades` | Deployment, 1 réplica | `ROLE=trades`: consume `orders.events` y paga los trades (fase 5). A lo sumo una réplica por partición; más quedarían ociosas en el consumer group. Sin rutas no abre el puerto HTTP, así que su probe usa `/metrics`. Separado de `funds` para que un atraso de liquidaciones no compita con las reservas del engine. |
| MatchingEngine, uno por book | StatefulSet `replicas: 1` + PVC por cada book de `books` en `values.yaml`, con el book en el nombre (`orderbook-engine-brl-vib`) | Single writer de su book. El PVC queda listo para los snapshots (fase 7) y RocksDB (fase 8). Un book nuevo también tiene que existir en `shared/books`. |
| Redpanda | Subchart oficial, 1 broker | El chart pone cada broker en un nodo distinto, así que en un cluster de un nodo (minikube) solo cabe uno. Con más nodos se suben juntos `redpanda.statefulset.replicas` y `topics.replicas` a 3, para que el log sobreviva a la caída de un broker. |
| PostgreSQL | Chart propio `deploy/helm/postgres` (StatefulSet con `postgres:16-alpine`), dependencia de `orderbook`, con una base por servicio que use PostgreSQL | Aislamiento de datos por servicio, con la misma imagen que compose. Se descartó el chart de Bitnami porque desde 2025 sus imágenes gratuitas ya no se mantienen. El engine no usa PostgreSQL. |

- **Migraciones:** cada pod migra al arrancar, como en local, sin `Job` aparte. Gofr toma un lock en `gofr_migration_locks` antes de migrar y, ya con el lock, vuelve a mirar la versión aplicada, así que dos réplicas nunca aplican la misma migración. El único choque posible es la primera vez de una base vacía: Gofr crea sus propias tablas antes de tomar el lock, y si dos réplicas lo hacen a la vez una falla (fase 4). Kubernetes reinicia ese pod, que encuentra las tablas y sigue. Pasa una sola vez por base.
- **Probes:** `readiness` del engine = replay terminado. `liveness` = el loop avanzó en los últimos N segundos o no hay comandos pendientes.
- **Single writer en Kubernetes:** `replicas: 1` con un StatefulSet garantiza a lo sumo un pod con ese nombre. El fencing real queda para el trabajo de HA, fuera de alcance.

### D3. Punto de entrada único por path

Los clientes ven una sola API, en un mismo host y puerto. Un ingress enruta por prefijo de path hacia cada servicio:

| Prefijo | Servicio |
| --- | --- |
| `/orders` | OrderService |
| `/wallet` | WalletService |
| `/market` | MarketService |

- **Kubernetes:** un recurso `Ingress` del chart, con ingress-nginx como controller (también en kind).
- **Rutas internas:** `/wallet/internal/funds:batch` vive solo en el rol `funds` de WalletService (fase 3, D9). El ingress manda `/wallet` solo a los pods de `api`, donde esa ruta no existe y responde `404`; el engine la llama por el Service interno de `funds`.
- **Transporte de `funds:batch`:** se decide aquí, con el benchmark de D4. Se pasa a gRPC en el rol `funds` si la latencia de `funds:batch` limita el throughput del engine; en ese caso con un Service headless y balanceo `round_robin` en el cliente, porque un Service normal balancea por conexión y gRPC mantiene una sola. Si no, queda en HTTP.
- **Health:** `/.well-known/*` tampoco se expone; lo usan las probes de cada pod.
- **Redpanda Console:** la UI del log (topics, mensajes y atraso de cada consumer group), del mismo chart de Redpanda, en su propio host `console.orderbook.local`, porque sirve desde `/`.
- **Identidad:** el ingress reenvía `X-User-ID` sin tocarlo. El día que haya auth real, es el lugar donde validar el token e inyectar el header.
- **Alternativa descartada:** un API gateway dedicado (Kong, Envoy Gateway). Agrega otra pieza a operar sin aportar nada que el MVP necesite; un `Ingress` con reglas por path alcanza.

### D4. Load generator

- **Herramienta:** `tools/loadgen` en Go, con un pool de personas pre-fondeadas. Genera tasas fijas (1k, 2,5k, 5k, 10k y 25k/s) y dos perfiles:
  - B: 90 % reposa, el de la carga sostenida de la spec;
  - E: las órdenes de B en un burst de 1k a 10k/s, para ver un pico.

  Se descartaron los perfiles «todo cruza», «50 % cancelaciones» y «book profundo»: la meta de 5.000/s se mide con B, y cada perfil extra suma código que hay que mantener y explicar.
- **Medición:** latencia de aceptación (`201`/`202`) y latencia hasta el evento final (consumiendo `orders.events`). El reporte termina con un veredicto: el engine sigue el ritmo cuando todas las órdenes aceptadas tienen su evento y el p99 hasta el evento queda por debajo de 1 s; si no, se atrasa. En los dos casos dice cuántas órdenes/s procesó el engine.
- **Alternativa descartada:** k6. Escribir en Go permite reutilizar `shared/eventlog` para medir de punta a punta.

### D5. Test de caída

Un script sobre kind corre el escenario B y borra pods en secuencia, que Kubernetes vuelve a crear. Al final verifica:
- que cada `orderId` creado tiene un estado final o está en el book;
- la conservación de BRL y VIB;
- que no hay saldos negativos.

Sin snapshots, el engine se recupera desde el offset 0, y la duración queda medida como línea base para la fase 7.

### D6. Componentes Go de la fase

| Ubicación | Componente | Responsabilidad |
| --- | --- | --- |
| `tools/loadgen` | `main` standalone (no es un servicio) | Fondear personas, enviar órdenes por HTTP a las tasas y perfiles de D4, consumir `orders.events` y reportar throughput y latencias |

- `tools/loadgen` lleva su package comment (`// Command loadgen ...`) y su lógica pura (perfiles, ritmo de envío, percentiles) en paquetes con tests, para cumplir el 85 % de cobertura por paquete.
- `tools/loadgen` importa `shared/eventlog`, `shared/money` y `shared/books`; nada importa `tools/`.
- Esta fase no agrega nada a `shared/`: Dockerfiles, chart y scripts son infraestructura, y el load generator solo consume los contratos existentes.
- Esta fase no agrega componentes a los servicios: solo los empaqueta y despliega.

## Risks / Trade-offs

- **[El benchmark no alcanza 5.000/s de punta a punta]** → Se comparan las latencias que mide `tools/loadgen` con el benchmark del engine para ubicar el cuello. Las palancas previstas son el tamaño de lote, gRPC para `funds:batch` y el batching del producer de OrderService. Si no alcanza, la fase no se cierra.
- **[El replay desde 0 hace lento el test de caída]** → Aceptado. Es la línea base que la fase 7 debe mejorar.

## Migration Plan

Se publican las imágenes y se instala el chart en un namespace nuevo; los entornos anteriores eran de desarrollo. Rollback: `helm rollback` o `helm uninstall`. Los datos viven en los PVC de Redpanda, PostgreSQL y el engine.

## Open Questions

- **Alta disponibilidad del engine.** La solución técnica la pone como objetivo, con un engine active y un hot standby por book. Este change la deja fuera (`replicas: 1`): si el engine cae, su book no procesa órdenes hasta que Kubernetes lo reinicia y reconstruye el book. No se pierde nada, porque los comandos esperan en el log. Falta decidir si la HA entra en estas fases o queda para después. Si entra, la propuesta inicial es:
  - **Elección del active:** un Lease de Kubernetes (`client-go/leaderelection`).
  - **Fencing por época:** el lease no alcanza, porque un engine congelado puede seguir escribiendo después de perderlo (split-brain). Cada recurso rechaza las escrituras de una época vieja. En `orders.events`, con el transactional producer de `franz-go` (`transactional.id = engine-{book}`; el broker responde `ProducerFenced` al viejo). En `funds:batch`, con la época por book guardada en WalletService.
  - **Referencias:** Kleppmann, "How to do distributed locking"; *Designing Data-Intensive Applications*, cap. 8 ("Fencing tokens"); KIP-98 (zombie fencing en Kafka).

# Tasks

## 1. Imágenes

- [x] 1.1 Crear un `Dockerfile` multi-stage por servicio (contexto en la raíz, copia `go.mod`/`go.sum` con su capa de `go mod download`, luego solo `shared/` y `microservices/<x>/`, `go build ./microservices/<x>`, runtime `distroless/static`); verificar que `docker build -f microservices/<x>/Dockerfile .` construye cada servicio por separado, que la imagen arranca y responde health, y que cambiar solo `microservices/order-service/` no invalida la caché de las otras imágenes

## 2. Kubernetes

- [x] 2.1 Crear el Helm chart `deploy/helm/orderbook` (Deployments para Order, Wallet y Market; StatefulSet `replicas: 1` con PVC para el engine; subcharts de Redpanda y PostgreSQL; ConfigMaps; probes); verificar con `helm lint` y `helm template`
- [x] 2.2 Agregar el `Job` de init de topics; verificar desplegando en kind sobre bases vacías que todos los pods llegan a `Ready`, aunque alguno se reinicie una vez al crear las tablas de Gofr (D2), y que el ejemplo de HU-12 de la fase 5 pasa contra el cluster
- [x] 2.3 Punto de entrada único (D3): agregar al chart el `Ingress` con las reglas por prefijo, sin enrutar `/.well-known/*` y con `/wallet` solo hacia los pods de `api`; verificar contra kind que `/orders`, `/wallet` y `/market` llegan a su servicio por la misma URL base y que `/wallet/internal/funds:batch` responde `404` desde afuera

## 3. Carga y resiliencia

- [x] 3.1 Crear `tools/loadgen` test-first: escribir los tests de la lógica pura (generación de los perfiles A–E, ritmo de envío a tasa fija y cálculo de percentiles p50–p99.9) y verlos fallar; implementar `tools/loadgen` (personas pre-fondeadas, tasas fijas, perfiles A–E, latencia de aceptación y hasta el evento final) con su package comment `// Command loadgen ...`; refactor en verde; verificar con `go test -cover ./tools/...` (85 % por paquete con lógica) y con una corrida corta a 1.000/s que reporta sus mediciones
- [ ] 3.2 Throughput sostenido: escribir la verificación de «Carga sostenida» (`matching-engine`) como corrida de `tools/loadgen` con el escenario B a 5.000/s durante 10 minutos que comprueba atraso del engine y conservación de BRL y VIB, y verla fallar contra el stack sin ajustar; ajustar las palancas de Risks / Trade-offs del design (tamaño de lote, batching del producer de OrderService) hasta que pase; correr los escenarios B y E a 1k, 2,5k, 5k, 10k y 25k/s y registrar en `docs/benchmark.md` throughput, latencias p50–p99.9, RAM, CPU, órdenes abiertas y RAM por orden abierta
- [ ] 3.3 Test de caída: escribir primero las verificaciones de «Caída durante la carga» (`matching-engine`) (cada `orderId` con estado final o en el book, conservación de BRL y VIB, saldos no negativos) y verlas fallar sin el script de reinicio; implementar el script que, sobre kind, borra en secuencia los pods del engine, de WalletService y de OrderService durante el escenario B; refactor en verde y registrar el tiempo de recuperación como línea base

## 4. Documentación

- [ ] 4.1 Escribir `docs/operations.md` (compose para desarrollar y los tests de integración, crear el cluster kind y desplegar con Helm, cómo corren las migraciones (D2), reset de entornos de desarrollo y regla de log nuevo ante cambios de resultado); verificar siguiendo el documento desde cero en una máquina limpia

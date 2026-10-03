# Proposal

## TL;DR

Fase 6 de 8. Empaquetar los cuatro servicios en imágenes Docker y crear un Helm chart para Kubernetes: Deployments para los servicios stateless y un StatefulSet con PVC para el engine. Todo el stack corre en un cluster (kind en local); `docker-compose.yml` queda como está, solo con PostgreSQL y Redpanda para desarrollar y correr los tests de integración. Con el stack completo corriendo, demostrar con un benchmark de punta a punta que se sostienen 5.000 órdenes/s y que el sistema resiste la caída de cualquier servicio.

## Why

El requerimiento no funcional central del PRD es de 5.000 operaciones/s con recuperación de fallos, y solo se puede validar con el sistema completo desplegado como en producción. El proyecto fija Docker, Kubernetes y Helm como forma de despliegue.

## Goals

- `helm install` despliega el stack completo en un cluster (kind en local).
- Medición de 5.000 órdenes/s sostenidas y de los escenarios B y E.
- Prueba de caída de cada servicio sin pérdida ni descuadre.

## Non-Goals

- HA del engine (standby con fencing): sigue con `replicas: 1`.
- Infra de nube específica (EKS, RDS, etc.).

## What Changes

- Dockerfiles multi-stage por servicio.
- `deploy/helm/orderbook`, con dependencias de Redpanda y PostgreSQL, ConfigMaps (tamaño de lote, registro de books) y probes de health/readiness.
- `tools/loadgen` con los escenarios B y E.
- Tests de punta a punta y de caída sobre el stack.
- `docs/operations.md` y resultados en `docs/benchmark.md`.

## Capabilities

### New Capabilities

Ninguna.

### Modified Capabilities

- `matching-engine`: se agrega el requirement de throughput sostenido de punta a punta.

## Assumptions

- Depende de `05-event-projections` (el stack completo).
- El benchmark corre en una máquina con el sizing de la PoC (engine 4–8 vCPU y 8–16 GB; Redpanda y PostgreSQL 4 CPU y 8 GB con SSD).

## Impact

- **Código:** Dockerfiles, `deploy/`, `tools/loadgen`.
- **Docs:** `docs/operations.md` (nuevo) y `docs/benchmark.md`.

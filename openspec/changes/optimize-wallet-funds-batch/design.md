# Design

## Context

Motivación en `proposal.md` (Why). Estado actual:

- `postgres{}.applyFundsBatch` (`store/walletdb/postgres_funds.go`) abre una transacción con `runInTransaction` y, por cada operación, manda `findResult` (`SELECT` del ledger), `reserveFunds` o `releaseFunds` (`UPDATE` de `balances`) e `insertMovement` (`INSERT` en el ledger).
- El engine llama a `funds:batch` dos veces por lote: las reservas antes de aplicar los comandos y las liberaciones después (`apply-commands/handler.go`).
- El ledger evita aplicar dos veces un mismo mensaje con `UNIQUE (type, message_id, currency)`.
- El mock del store (`store/walletdb/mock.go`) aplica las mismas reglas en memoria para los tests de handlers.

## Goals / Non-Goals

**Goals:**

- Mismos resultados que hoy para cualquier lote, incluidas las repeticiones y los rechazos.

**Non-Goals:**

- Cambiar el mock del store o los handlers.

## Decisions

### D1. Una función de PostgreSQL que recorre el lote

`apply_funds_batch(operations jsonb)` recorre las operaciones en el orden recibido y, para cada una, hace lo mismo que hoy hace Go:

```sql
FOR op IN SELECT * FROM jsonb_to_recordset(operations) AS (...) LOOP
  -- ¿ya se aplicó este mensaje? devolver el resultado guardado
  -- RESERVE: UPDATE balances ... WHERE available >= op.amount
  -- RELEASE: UPDATE balances ... WHERE reserved >= op.amount
  -- anotar en el ledger, salvo una liberación que no cupo (como hoy)
  RETURN NEXT;  -- position y result de esta operación
END LOOP;
```

Es una función y no un procedure porque tiene que devolver una fila por operación. Un procedure no devuelve filas.

No se puede aplicar todo el lote con una sola sentencia sobre el conjunto (`UPDATE ... FROM unnest(...)`). La misma persona puede tener varias operaciones en un lote, y cada una tiene que ver el saldo que dejó la anterior. Un `UPDATE` no actualiza la misma fila dos veces.

**Alternativas descartadas:**

| Alternativa | Por qué no |
| --- | --- |
| Una sentencia por operación (`WITH` que encadena `SELECT`, `UPDATE` e `INSERT`) | Siguen siendo ~500 viajes por lote, uno detrás de otro, con las filas bloqueadas |
| Una transacción corta por operación, en paralelo por persona | Más conexiones y más commits, y el lote deja de aplicarse todo o nada |
| Un trigger en `balances` que escriba el ledger | El trigger no conoce el `message_id` ni la orden |

### D2. Entrada y salida

- **Entrada:** un solo parámetro `jsonb` con un arreglo. Go arma cada elemento con:
  - `position`: el índice en el lote;
  - el `id` del movimiento (UUIDv7) y su `createdAt`;
  - `type`, `message_id`, `order_id`, `user_id`, `currency` y `amount`.

  Las claves van en snake_case porque `jsonb_to_recordset` las busca por el nombre de las columnas que declara la función. El `amount` va como string, como el resto del JSON del proyecto, y la función lo lee como `bigint`. Go sigue generando el id y la fecha, como en los demás movimientos, así que en el ledger ordenar por id sigue siendo ordenar por creación.
- **Salida:** `TABLE (position int, result text)`, leída con `ORDER BY position`. Go arma el mismo `[]models.FundsResult` de hoy. Si vuelven menos filas que operaciones, es un error.
- **Resultados:** los mismos de hoy (`OK`, `insufficient_funds`, `release_exceeds_reservation`). Dicen si la operación se pudo aplicar y, si no, por qué, sin cambiar el endpoint ni el engine.
- **Alternativa descartada:** un arreglo por columna con `unnest`. Son nueve parámetros, y depende de cómo el driver (`lib/pq`) codifica cada tipo de arreglo.

### D3. Todo o nada, sin transacción en Go

Una sentencia es atómica por sí misma: si falla cualquier operación, no queda aplicada ninguna del lote. Por eso el store ya no usa `runInTransaction`.

Si el mismo mensaje llega en dos llamadas a la vez, el `UNIQUE` del ledger hace fallar la segunda llamada completa. El engine la reintenta, y la repetición encuentra los resultados guardados, igual que hoy.

### D4. Dónde vive cada componente

| Componente | Capa y path | Responsabilidad |
| --- | --- | --- |
| `apply_funds_batch` | `microservices/wallet-service/migrations/20261006000000_create_apply_funds_batch.go` | Crear la función con `CREATE OR REPLACE FUNCTION` |
| `applyFundsBatch` (constante) y `postgres{}.applyFundsBatch` | `microservices/wallet-service/store/walletdb/postgres_funds.go` | Armar el JSON del lote, llamar a la función con una consulta y leer los resultados |

- Salen de `postgres_funds.go` las consultas `findResult`, `reserveFunds` y `releaseFunds`, y las funciones que las usan. `insertMovementIn` queda, porque la usan los depósitos y los retiros.
- Nada va a `shared/`, y no hay dependencias nuevas.
- Encaja en las capas actuales. El store sigue escribiendo SQL a mano, la migración crea un objeto del esquema y, como pide `go.md`, el SQL se prueba contra PostgreSQL con el tag `integration`. La regla del saldo ya vivía en el SQL (`WHERE available >= $3`); ahora también viven ahí la revisión del mensaje repetido y la escritura del ledger.
- Cambiar una regla más adelante es una migración nueva con la función completa (`CREATE OR REPLACE FUNCTION`). La migración anterior queda como historia.

### D5. Tests

- **Unit** (`store/walletdb`, con el SQL mock de Gofr): la llamada espera un solo `ExpectQuery(applyFundsBatch)`, sin `ExpectBegin`, con el JSON armado desde las operaciones. Cubre:
  - las filas se convierten en resultados en el orden del lote;
  - un error de la base se devuelve;
  - menos filas que operaciones es un error.
- **Integración** (`-tags integration`, contra el PostgreSQL de compose):
  - Los tests actuales de `ApplyFundsBatch` y de concurrencia prueban ahora la función, sin cambios.
  - Se agregan dos casos: dos operaciones de la misma persona en un lote (la segunda ve el saldo que dejó la primera), y un lote con una operación inválida (`amount` 0 incumple el `CHECK` del ledger), que falla sin aplicar ninguna.

### D6. Medición

Se repiten las corridas de la línea base en minikube:

1. Reconstruir la imagen de WalletService y reiniciar sus pods.
2. Esperar a que el engine se ponga al día.
3. Correr `tools/loadgen` con el escenario B a 1.000/s durante 30 s, con 200 y con 5.000 personas.
4. Sacar los percentiles de `funds:batch` del `response_time` de los logs de `wallet-funds`.

| Corrida | `funds:batch` p50 | p90 | p99 | Engine, p50 hasta el primer evento |
| --- | --- | --- | --- | --- |
| Línea base, 200 personas | 54 ms | 1.557 ms | 2.373 ms | 34,4 s |
| Línea base, 5.000 personas | 66 ms | 854 ms | 1.411 ms | 13,8 s |

La optimización cumple si `funds:batch` baja en p50, p90 y p99, y si el engine sigue el ritmo de 1.000/s: p50 hasta el primer evento por debajo de 1 s, sin atraso creciente. Los números entran en `docs/benchmark.md` con la tarea 3.2 de la fase 6.

## Risks / Trade-offs

- **[Las reglas de fondos quedan en PL/pgSQL, fuera de los unit tests y de CI]** → Los tests de integración cubren cada regla, y se corren antes de cerrar cada tarea.
- **[El lote sigue siendo una transacción que bloquea filas mientras corre]** → Ahora dura solo el trabajo dentro de la base, sin viajes de red. Si el rol `trades` sigue esperando, la palanca siguiente es procesar `trades` en lotes, fuera de este change.
- **[Cambiar una regla obliga a una migración nueva]** → Aceptado: las reglas de reserva y liberación cambian poco.

## Migration Plan

1. Se despliega la imagen nueva de WalletService. Cada pod migra al arrancar, con el lock de Gofr, así que la función existe antes de que el pod atienda.
2. Durante el rolling update, los pods viejos siguen con el camino anterior, que no usa la función. Los dos caminos conviven sin problema.
3. **Rollback:** `helm rollback` o la imagen anterior. La función queda en la base, sin uso.

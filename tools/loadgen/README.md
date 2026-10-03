# loadgen

Responde una sola pregunta: **¿el sistema aguanta N órdenes por segundo?**

## Cómo lo mide

1. Fondea 5.000 personas de prueba (`loadgen-0`, `loadgen-1`, ...) con BRL y VIB de sobra, para que ninguna orden se rechace por fondos.
2. Manda órdenes a la API al ritmo del perfil. El 90 % son limit que quedan en el book y el 10 % son market que las cruzan.
3. Para cada orden mide dos tiempos:
   - **`API answer`**: cuánto tarda OrderService en responder `201`.
   - **`engine event`**: cuánto tarda en aparecer en `orders.events` el primer evento del engine sobre esa orden (`OrderAccepted` u `OrderRejected`). Incluye la API, la reserva de fondos en WalletService y el matching.
4. Cada 5 s escribe una línea de progreso y, al final, un resumen con el veredicto.

Por qué no alcanza con medir la API: OrderService escribe la orden en el log y responde, y el engine la procesa después. A 5.000/s la API responde en milisegundos aunque el engine vaya segundos atrasado.

## Correrlo

Necesita el stack desplegado en minikube, en el namespace `orderbook`. loadgen corre como un pod dentro del cluster, porque Redpanda solo se alcanza desde adentro.

```bash
# la primera vez, o después de cambiar loadgen
eval $(minikube docker-env)
docker build -t orderbook/loadgen:dev -f tools/loadgen/Dockerfile .

# una corrida: los flags van a loadgen
tools/loadgen/run.sh -rate 5000 -duration 1m
```

`run.sh` lanza el pod, muestra su salida desde la primera línea y lo borra al terminar (también con Ctrl-C).

| Flag | Default | Qué cambia |
| --- | --- | --- |
| `-profile` | `B` | Cómo se mueve el tráfico en el tiempo (ver abajo) |
| `-rate` | `1000` | Órdenes por segundo; en `E`, el ritmo de base |
| `-peak` | `10000` | Solo `E`: el ritmo del pico |
| `-duration` | `1m` | Cuánto tiempo manda |
| `-wait` | `30s` | Cuánto espera, al terminar, los últimos eventos del engine |

La meta del PDR (RNF-01) es `-rate 5000 -duration 10m` con el veredicto «keeps up».

## Perfiles

Los dos mandan las mismas órdenes; cambia solo el ritmo.

**`B`: ritmo fijo.** Responde si el sistema sostiene un ritmo. `-rate 5000 -duration 30s`:

```text
órdenes/s
  5000 ┤──────────────────────────────
       │
     0 ┼──────────────────────────────▶ tiempo
       0s                           30s
```

**`E`: un pico.** Responde si el sistema absorbe un pico y se recupera después. Manda `-rate` el primer tercio, `-peak` el del medio y `-rate` otra vez el último. `-profile E -rate 1000 -peak 6000 -duration 30s`:

```text
órdenes/s
  6000 ┤          ┌─────────┐
       │          │         │
  1000 ┤──────────┘         └──────────
     0 ┼──────────┬─────────┬─────────▶ tiempo
       0s        10s       20s       30s
```

## Leer el resultado

Una corrida real de `E`:

```text
profile E, 1000/s with a peak of 6000/s, for 30s
funding 5000 people
sending for 30s
   5s  API    999/s  engine    997/s  backlog 9
  10s  API   1000/s  engine    998/s  backlog 19
  15s  API   5955/s  engine   2542/s  backlog 17086
  20s  API   6003/s  engine   2242/s  backlog 35893
  25s  API   1040/s  engine   3528/s  backlog 23457
  30s  API    996/s  engine   4193/s  backlog 7473
waiting up to 1m0s for the last engine events
sent 79999, accepted 79999, failed 0: 2647 accepted/s
API answer    p50=6ms p90=22ms p99=58ms p99.9=159ms max=238ms
engine event  p50=6.029s p90=8.91s p99=9.368s p99.9=9.423s max=9.428s
orders without their engine event: 0
verdict: the engine falls behind, it processed 2534 orders/s
```

**Las líneas de progreso** cuentan los últimos 5 s:

| Columna | Qué dice |
| --- | --- |
| `API` | Órdenes por segundo que aceptó la API |
| `engine` | Órdenes por segundo que procesó el engine |
| `backlog` | Órdenes aceptadas que el engine todavía no procesó |

Mientras `engine` sigue a `API`, el backlog queda cerca de 0. En el ejemplo, a 1.000/s el engine va al día. En el pico la API acepta 6.000/s pero el engine procesa unas 2.500/s, y el backlog sube hasta casi 36.000. Cuando el pico termina, el engine lo va drenando a ~4.000/s.

**El resumen** cubre toda la corrida:

| Línea | Qué dice |
| --- | --- |
| `sent ... accepted/s` | Órdenes enviadas, aceptadas con `201` y fallidas, y cuántas se aceptaron por segundo en promedio |
| `API answer` | Cuánto tardó la API en responder |
| `engine event` | Cuánto tardó el engine en procesar cada orden |
| `orders without their engine event` | Órdenes cuyo evento no llegó dentro de `-wait` |
| `verdict` | `keeps up` si todas tienen su evento y el p99 de `engine event` está bajo 1 s; si no, `falls behind`. En los dos casos dice cuántas órdenes/s procesó el engine en promedio |

`p50` es el tiempo que no superó la mitad de las órdenes, `p99` el que no superó el 99 % y `max` la más lenta.

## Verlo en vivo

Con `<minikube ip> orderbook.local console.orderbook.local` en `/etc/hosts`, la Redpanda Console queda en <http://console.orderbook.local>. En **Consumer Groups** está el lag de `order-service`, `wallet-trades` y `market-service`: si crece durante la corrida, ese lector no da abasto. El engine no aparece ahí porque no usa consumer group; su atraso es el `backlog` de loadgen.

## Ojo

- **Las corridas no están aisladas.** Usan el mismo book que las pruebas manuales, y sus órdenes quedan en el mercado. Cada corrida deja unas 90.000 órdenes por cada 100.000 enviadas, así que las siguientes corren sobre un book y una base más grandes. Para comparar números, parte de un entorno limpio.
- **Después de reiniciar el engine, espera a que se ponga al día.** Relee todo el log al arrancar; una corrida que empieza antes mide ese atraso.

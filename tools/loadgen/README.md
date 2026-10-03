# loadgen

`loadgen` sends orders to the deployed stack at a fixed rate and reports what the system did with them: how many orders the API accepted per second, how long the API took to answer, and how long until the matching engine published its first event about each order. It measures the throughput goal of the PDR (RNF-01, 5,000 orders/s sustained) and is design D4 of [06-container-infrastructure](../../openspec/changes/06-container-infrastructure/design.md). The engine's own speed, without the network, the wallet or the log, is in [docs/benchmark.md](../../docs/benchmark.md).

## Prerequisites

- The stack deployed with Helm on a local minikube cluster: release `orderbook`, namespace `orderbook`, chart [`deploy/helm/orderbook`](../../deploy/helm/orderbook) (see [Related utilities](#related-utilities)).
- `kubectl` pointing at that cluster.

## Build the image

Build it with minikube's Docker daemon, so the image is already in the cluster and needs no `minikube image load`. Run it from the repository root:

```bash
eval $(minikube -p minikube docker-env)   # fish: minikube -p minikube docker-env | source
docker build -t orderbook/loadgen:dev -f tools/loadgen/Dockerfile .
```

## Run it

loadgen runs inside the cluster, as a pod. Redpanda only advertises in-cluster addresses, so it cannot be reached from the laptop.

```bash
kubectl run loadgen -n orderbook --image=orderbook/loadgen:dev --image-pull-policy=Never \
  --restart=Never --attach --rm -- -profile B -rate 1000 -duration 30s -people 5000
```

Everything after `--` is a loadgen flag. The report is printed when the run ends, and `--rm` deletes the pod.

## Flags

| Flag | Default | What it sets |
| --- | --- | --- |
| `-orders` | `http://orderbook-order-api` | OrderService base URL |
| `-wallet` | `http://orderbook-wallet-api` | WalletService base URL, for the deposits |
| `-host` | empty | Host header, to go through the ingress |
| `-brokers` | `redpanda:9093` | Redpanda brokers, comma separated |
| `-book` | `BRL-VIB` | Book to trade |
| `-profile` | `B` | Load profile: `A`, `B`, `C`, `D` or `E` (see [Profiles](#profiles)) |
| `-rate` | `1000` | Requests per second; the base rate of `E` |
| `-peak` | `10000` | Requests per second at the peak of `E` |
| `-people` | `200` | People trading, funded at the start |
| `-workers` | `200` | Requests in flight at once |
| `-duration` | `1m` | How long to send |
| `-wait` | `30s` | How long to wait, after sending, for the last engine events |
| `-seed` | `1` | Seed of the requests: the same seed sends the same requests, so two runs are comparable |

Use `-people 5000` for realistic runs. With the default 200, each person sends about 5 orders/s at 1,000/s, and all of them reserve funds on the same wallet balance rows, which exaggerates lock contention.

`-workers` caps the requests in flight. Latencies count from the moment a worker sends the request, so if every worker is busy the wait for a free one is not in them; it shows instead as `accepted/s` below `-rate`. At 5,000/s, 200 workers are enough while the API answers in under 40 ms.

## What it does

1. **Reads the engine's events.** It reads the book's partition of `orders.events` with `consumer.StartPartition` of [`shared/eventlog`](../../shared/eventlog/consumer), from the start of the partition, and skips the events created before the run.
2. **Funds the people.** Each person (`loadgen-0`, `loadgen-1`, ...) gets a deposit of R$ 100,000,000.00 and 1,000,000 VIB through WalletService, so no order is rejected for its funds. The deposits go one after another, so with 5,000 people it takes a while before the first order is sent.
3. **Sends at a fixed rate.** Every millisecond it hands a pool of workers the requests due since the start ([`pace`](pace/pace.go)). When requests slow down, the ones that came due meanwhile go out as soon as a worker is free, so a slow system cannot lower the rate and make the measurement look better. Only a `201` counts as accepted.
4. **Pairs each order with its first event.** For each created order it keeps the time it was sent and the time it read the engine's first event about it, `OrderAccepted` or `OrderRejected` ([`driver/tracker.go`](driver/tracker.go)).
5. **Waits for the last events.** After sending, it waits up to `-wait`, and stops earlier once every accepted order has its event.
6. **Prints the report.**

## Profiles

Every profile trades around a mid price of R$ 100.00 ([`workload`](workload/workload.go)).

| Profile | What it sends |
| --- | --- |
| `A` | Everything crosses: a buy and a sell of 1 VIB at the mid price, by turns |
| `B` | 90 % rests: limit buys below the mid price and sells above it, within R$ 5.00, of 1 to 5 VIB. The other 10 % are market orders that cross them: a buy of R$ 105.00 or a sell of 1 VIB |
| `C` | Half the requests cancel the oldest open order of the run, sent by its owner; the other half are resting orders as in `B` |
| `D` | A deep book: resting orders of 1 VIB over 5,000 prices on each side, from R$ 50.00 to R$ 150.00 |
| `E` | The orders of `B` in a burst: `-rate` for the first third of `-duration`, `-peak` for the middle third, `-rate` again for the last ([`pace`](pace/pace.go)) |

`B` is the profile of the spec's sustained throughput scenario (5,000 orders/s for 10 minutes).

In `C`, cancellations count in `sent`, `accepted` and `API accepted`, but only created orders wait for an engine event. A cancellation of an order created a moment before can answer `404` (see [Reads after a write](../../docs/api.md#reads-after-a-write)) and counts as failed.

## Reading the report

A real run, profile `B` at 1,000/s for 30 s with 5,000 people:

```text
profile B, 1000/s for 30s
sent 29999, accepted 29999, failed 0, in 30.001s: 1000 accepted/s
API accepted  n=29999 p50=1.8ms p90=3.7ms p99=14.8ms p99.9=23.2ms max=38ms
engine event  n=29999 p50=13.7623s p90=21.4006s p99=22.1179s p99.9=22.3545s max=22.383s
orders without their engine event: 0
```

| Line | What it says |
| --- | --- |
| `profile ...` | The profile, rate and duration of the run (for `E`, the base rate) |
| `sent ... accepted/s` | Requests sent, answered `201`, and failed (any other answer, or none). `accepted/s` is the real accepted throughput: accepted requests over the time from the first request to the last answer |
| `API accepted` | Time until the API answered `201`: OrderService checking the order and writing its command to the log |
| `engine event` | Time until loadgen read the engine's first event about the order: the API, plus the engine applying it, plus WalletService reserving its funds |
| `orders without their engine event` | Accepted orders whose event did not arrive within `-wait` |

`n` is the number of samples, `p50` to `p99.9` are percentiles and `max` is the slowest.

When `engine event` is much larger than `API accepted`, the engine is falling behind: commands pile up in the log faster than it applies them (a backlog). Orders without their event mean the system did not keep up within `-wait`.

In the example the API keeps up (p99 of 14.8 ms) but the engine does not: half the orders waited more than 13 s for their event. That is the known current limit: WalletService's `funds:batch`, with lock contention on the balance rows and about 1,500 queries one after another per batch.

## Proving the goal

RNF-01 asks for 5,000 orders/s sustained:

```bash
kubectl run loadgen -n orderbook --image=orderbook/loadgen:dev --image-pull-policy=Never \
  --restart=Never --attach --rm -- -profile B -rate 5000 -duration 10m -people 5000
```

It passes when:

- `accepted/s` is about 5000 and `failed` is 0;
- the `engine event` p99 stays small and stable: milliseconds, not seconds that grow with the run;
- no order is left without its engine event.

loadgen does not check that BRL and VIB are conserved, which the spec also requires.

## Watching a run live

The Console and the market go through the ingress. Add the cluster's IP (`minikube ip`, here `192.168.85.2`) to `/etc/hosts`:

```text
192.168.85.2 orderbook.local console.orderbook.local
```

- **Redpanda Console**, <http://console.orderbook.local>:
  - *Topics*: the messages arriving in `orders.commands` and `orders.events`.
  - *Consumer Groups*: the lag of `order-service`, `wallet-trades` and `market-service`. A lag that keeps growing means that reader cannot keep up.
  - The engine has no consumer group. Compare the message counts of `orders.commands` and `orders.events`: when commands keep growing and events do not follow, the engine is behind.
- **Market depth**: `curl http://orderbook.local/market/orderbook/BRL-VIB`.

## Related utilities

[`deploy/reset-dev.sh`](../../deploy/reset-dev.sh) recreates the **local docker compose** environment (topics and databases) from scratch. It does not touch the cluster.

Install or upgrade the cluster:

```bash
helm repo add redpanda https://charts.redpanda.com   # first time only
helm dependency build deploy/helm/orderbook
helm upgrade --install orderbook deploy/helm/orderbook -n orderbook --create-namespace
```

Deploy a change to a service: rebuild its image inside minikube and restart what runs it. The tag `dev` does not change, so Kubernetes does not notice the new image otherwise.

```bash
eval $(minikube -p minikube docker-env)
docker build -t orderbook/order-service:dev -f microservices/order-service/Dockerfile .
kubectl rollout restart deployment/orderbook-order-api deployment/orderbook-order-projector -n orderbook
```

| Image | Runs as |
| --- | --- |
| `orderbook/order-service` | `deployment/orderbook-order-api`, `deployment/orderbook-order-projector` |
| `orderbook/wallet-service` | `deployment/orderbook-wallet-api`, `deployment/orderbook-wallet-funds`, `deployment/orderbook-wallet-trades` |
| `orderbook/market-service` | `deployment/orderbook-market` |
| `orderbook/matching-engine` | `statefulset/orderbook-engine-brl-vib` |

Start from a clean cluster environment, which deletes every order, wallet and message, then install again:

```bash
helm uninstall orderbook -n orderbook && kubectl delete namespace orderbook
```

## Caveats

- **Runs are not isolated.** They trade on the same book as manual tests, so the resting orders of a run stay in the market and can cross manual orders.
- **Wait for the engine after a restart.** It rereads the whole log at startup, and Kubernetes marks it ready before it finishes. A run started before it caught up begins on a backlog and measures it.
- **Memory.** loadgen keeps every order in memory until the end: a 10-minute run at 5,000/s is about 3 million orders.

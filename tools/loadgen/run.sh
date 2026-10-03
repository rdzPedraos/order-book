#!/usr/bin/env bash
# Runs loadgen as a pod in the orderbook namespace and follows its whole
# output: kubectl run --attach misses what the pod prints before it attaches.
# Ctrl-C stops the run: the pod is deleted, loadgen stops sending and prints
# its report, and the script ends with it.
# The flags go to loadgen: tools/loadgen/run.sh -rate 5000 -duration 1m
set -euo pipefail

namespace=orderbook

stop_loadgen() {
  kubectl delete pod loadgen -n "$namespace" --ignore-not-found --wait=false >/dev/null
}

kubectl delete pod loadgen -n "$namespace" --ignore-not-found --wait >/dev/null
kubectl run loadgen -n "$namespace" --image=orderbook/loadgen:dev --image-pull-policy=Never \
  --restart=Never -- "$@" >/dev/null
trap stop_loadgen EXIT

until [ "$(kubectl get pod loadgen -n "$namespace" -o jsonpath='{.status.phase}')" != Pending ]; do
  sleep 0.5
done

# A command in the background ignores Ctrl-C, so the output keeps coming
# after it until loadgen exits, report included.
kubectl logs -f loadgen -n "$namespace" &
trap stop_loadgen INT
wait $! || wait $!

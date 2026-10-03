#!/usr/bin/env bash
# Runs loadgen as a pod in the orderbook namespace and follows its whole
# output: kubectl run --attach misses what the pod prints before it attaches.
# The flags go to loadgen: tools/loadgen/run.sh -rate 5000 -duration 1m
set -euo pipefail

namespace=orderbook

kubectl delete pod loadgen -n "$namespace" --ignore-not-found --wait >/dev/null
kubectl run loadgen -n "$namespace" --image=orderbook/loadgen:dev --image-pull-policy=Never \
  --restart=Never -- "$@" >/dev/null
trap 'kubectl delete pod loadgen -n "$namespace" --ignore-not-found --wait=false >/dev/null' EXIT

until [ "$(kubectl get pod loadgen -n "$namespace" -o jsonpath='{.status.phase}')" != Pending ]; do
  sleep 0.5
done

kubectl logs -f loadgen -n "$namespace"

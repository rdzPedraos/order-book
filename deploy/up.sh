#!/usr/bin/env bash
# Builds every image of the repo inside minikube and installs the orderbook
# chart from scratch: it uninstalls the release and deletes its volumes first,
# so every run starts with empty databases and an empty log, and every pod runs
# the images just built.
#
# Usage: deploy/up.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
namespace=orderbook

# minikube's Docker daemon builds them, so the cluster already has the images.
eval "$(minikube docker-env --shell bash)"

echo "building the images"
for dockerfile in "$root"/microservices/*/Dockerfile "$root"/tools/*/Dockerfile; do
  image="orderbook/$(basename "$(dirname "$dockerfile")"):dev"
  echo "building $image"
  docker build -q -t "$image" -f "$dockerfile" "$root" >/dev/null
done

echo "uninstalling the chart"
helm uninstall orderbook -n "$namespace" --wait --ignore-not-found >/dev/null
kubectl delete pvc --all -n "$namespace"

echo "installing the chart"
helm repo add redpanda https://charts.redpanda.com >/dev/null 2>&1 || true
helm dependency build --skip-refresh "$root/deploy/helm/orderbook" >/dev/null
helm upgrade --install orderbook "$root/deploy/helm/orderbook" -n "$namespace" --create-namespace --wait --timeout 10m >/dev/null

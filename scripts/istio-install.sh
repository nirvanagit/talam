#!/usr/bin/env bash
# Installs Istio's "demo" profile into the given kind cluster and waits for
# istiod to be ready. Called by `make istio-install` / `make up`.
set -euo pipefail

CLUSTER="${1:-talam-demo}"
CTX="kind-${CLUSTER}"

if ! command -v istioctl >/dev/null 2>&1; then
  echo "istioctl not found. Install it (e.g. 'brew install istioctl') and re-run." >&2
  exit 1
fi

echo "Installing Istio (demo profile) into context ${CTX}..."
istioctl install --context "${CTX}" --set profile=demo -y

echo "Waiting for istiod to be ready..."
kubectl --context "${CTX}" -n istio-system rollout status deployment/istiod --timeout=180s

echo "Istio installed."

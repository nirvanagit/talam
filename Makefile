SHELL := /bin/bash
GOARCH := $(shell go env GOARCH)
KIND_CLUSTER := talam-demo
NAMESPACE := talam-system
BINARIES := talam-agent talam-server talam-operator talam-mesh-mcp talamctl

.PHONY: build test vet lint \
	kind-up kind-down istio-install \
	deploy demo verify \
	up down logs-server logs-agent \
	clean

## --- Build & test (no cluster required) -----------------------------------

build: ## Compile all binaries to bin/ for linux/$(GOARCH) (what the cluster runs)
	@mkdir -p bin
	@for b in $(BINARIES); do \
		echo "building $$b..."; \
		GOOS=linux GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o bin/$$b ./cmd/$$b; \
	done

test: ## Run unit tests (fixture-based, no cluster required)
	go test ./...

vet:
	go vet ./...

lint: vet test

## --- Local cluster lifecycle -----------------------------------------------

kind-up: ## Create the local kind cluster (idempotent)
	@if kind get clusters 2>/dev/null | grep -qx $(KIND_CLUSTER); then \
		echo "kind cluster $(KIND_CLUSTER) already exists"; \
	else \
		kind create cluster --name $(KIND_CLUSTER) --config scripts/kind-config.yaml; \
	fi
	kubectl cluster-info --context kind-$(KIND_CLUSTER)

istio-install: ## Install Istio (demo profile) into the kind cluster
	./scripts/istio-install.sh $(KIND_CLUSTER)

kind-down: ## Delete the local kind cluster
	kind delete cluster --name $(KIND_CLUSTER)

## --- Deploy talam + demo workload -------------------------------------------

kind-load: build ## Build container images from bin/ and load them into kind
	docker build -f docker/Dockerfile --build-arg BINARY=talam-agent    -t talam-agent:local    .
	docker build -f docker/Dockerfile --build-arg BINARY=talam-server   -t talam-server:local   .
	docker build -f docker/Dockerfile --build-arg BINARY=talam-operator -t talam-operator:local .
	docker build -f docker/Dockerfile --build-arg BINARY=talam-mesh-mcp -t talam-mesh-mcp:local .
	kind load docker-image talam-agent:local talam-server:local talam-operator:local talam-mesh-mcp:local --name $(KIND_CLUSTER)

deploy: kind-load ## Install CRDs and deploy operator + server + mesh-mcp (agent comes from the operator reconciling MeshDiagnostics)
	kubectl apply -f deploy/crds/
	kubectl apply -f deploy/namespace.yaml
	kubectl apply -f deploy/operator/rbac.yaml
	kubectl apply -f deploy/operator/deployment.yaml
	kubectl apply -f deploy/server/rbac.yaml
	kubectl apply -f deploy/server/deployment.yaml
	kubectl apply -f deploy/mesh-mcp/rbac.yaml
	kubectl apply -f deploy/mesh-mcp/deployment.yaml
	@if [ -n "$$ANTHROPIC_API_KEY" ]; then \
		kubectl -n $(NAMESPACE) create secret generic talam-llm-credentials \
			--from-literal=apiKey=$$ANTHROPIC_API_KEY --dry-run=client -o yaml | kubectl apply -f -; \
		kubectl apply -f deploy/server/modelbinding-sample.yaml; \
	else \
		echo ""; \
		echo "!! ANTHROPIC_API_KEY not set. The in-cluster talam-server image has no claude CLI"; \
		echo "!! (that fallback only works when talam-server runs on the host — see README), so"; \
		echo "!! LLM calls will fail until you either:"; \
		echo "!!   export ANTHROPIC_API_KEY=... && make deploy   # re-run with a real key, or"; \
		echo "!!   kubectl apply -f deploy/server/modelbinding-sample.yaml  # after creating"; \
		echo "!!     the talam-llm-credentials secret yourself against a running cluster."; \
		echo ""; \
	fi
	kubectl -n $(NAMESPACE) rollout status deployment/talam-server --timeout=120s
	kubectl -n $(NAMESPACE) rollout status deployment/talam-operator --timeout=120s
	kubectl -n $(NAMESPACE) rollout status deployment/talam-mesh-mcp --timeout=120s
	kubectl apply -f deploy/mesh-mcp/mcpserver-sample.yaml
	kubectl apply -f deploy/operator/meshdiagnostics-sample.yaml

demo: ## Deploy the intentionally-broken demo mesh (docs: orphaned subset + dangling host)
	kubectl apply -f deploy/demo/httpbin.yaml
	kubectl -n demo rollout status deployment/httpbin-v1 --timeout=120s
	kubectl apply -f deploy/demo/broken-mesh.yaml

verify: ## Poll talam-server until it reports incidents with a real LLM explanation
	./scripts/verify.sh

up: kind-up istio-install deploy demo verify ## Full local stand-up: cluster, istio, talam, broken demo mesh, verify end-to-end

down: kind-down ## Tear down the local kind cluster

## --- Convenience ------------------------------------------------------------

logs-server:
	kubectl -n $(NAMESPACE) logs deploy/talam-server -f

logs-agent:
	kubectl -n $(NAMESPACE) logs deploy/talam-agent-kind-local -f

port-forward: ## Expose the dashboard at http://localhost:8443
	kubectl -n $(NAMESPACE) port-forward svc/talam-server 8443:8443

clean:
	rm -rf bin talam-server-store.json

help:
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | sed 's/:.*## /|/' | sort | awk -F'|' '{printf "  %-16s %s\n", $$1, $$2}'

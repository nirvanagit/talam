# Getting Started

talam detects mesh misconfigurations automatically and proposes fixes. This guide walks you through a local setup.

## Prerequisites

- Kubernetes 1.26+ (local cluster via kind, minikube, or Docker Desktop)
- Istio 1.17+ installed in the cluster
- kubectl configured to access your cluster
- Go 1.21+ (to build from source)

## 1. Set up a test cluster

Create a local Kubernetes cluster with Istio:

```bash
kind create cluster --name talam-demo
istioctl install --set profile=demo -y
```

Deploy a sample application:

```bash
kubectl create namespace demo
kubectl label namespace demo istio-injection=enabled
kubectl apply -n demo -f https://raw.githubusercontent.com/istio/istio/release-1.17/samples/bookinfo/platform/kube/bookinfo.yaml
```

## 2. Deploy talam

Clone the repository and deploy talam-operator:

```bash
git clone https://github.com/nirvanagit/talam.git
cd talam
kubectl apply -k deploy/crds
kubectl apply -k deploy/operator
```

Verify the operator is running:

```bash
kubectl get deployment -n talam-system talam-operator
```

## 3. Deploy talam-server and talam-agent

Create a MeshDiagnostics object to configure talam:

```yaml
apiVersion: talam.io/v1alpha1
kind: MeshDiagnostics
metadata:
  name: default
  namespace: talam-system
spec:
  serverEndpoint: "http://talam-server:8080"
  scanInterval: 5m
  enabledAnalyzers:
    - "istio-mismatch-routes"
    - "destination-rule-unused"
    - "mtls-policy-mismatch"
```

Apply it:

```bash
kubectl apply -f meshdiagnostics.yaml
```

Deploy the server and agent:

```bash
kubectl apply -k deploy/server
kubectl apply -k deploy/agent
```

Check their status:

```bash
kubectl get pods -n talam-system
kubectl logs -n talam-system deployment/talam-server
kubectl logs -n talam-system deployment/talam-agent
```

## 4. Trigger your first scan

Port-forward to the server:

```bash
kubectl port-forward -n talam-system svc/talam-server 8080:8080
```

Manually trigger a scan (or wait for the next interval):

```bash
curl -X POST http://localhost:8080/v1/scan \
  -H "Content-Type: application/json" \
  -d '{"namespace": "demo"}'
```

Watch incidents appear:

```bash
kubectl get meshincidents -n demo -w
```

## 5. Approve and apply a remediation

List pending resolutions:

```bash
kubectl get meshresolutions -n demo
```

View a resolution's details:

```bash
kubectl get meshresolution <name> -n demo -o yaml
```

The `spec.proposal` contains the dry-run output. When ready, approve by setting `spec.approved: true`:

```bash
kubectl patch meshresolution <name> -n demo -p '{"spec":{"approved":true}}'
```

Watch the agent apply it:

```bash
kubectl logs -n talam-system deployment/talam-agent -f
```

Check the incident status:

```bash
kubectl get meshincident <incident-name> -n demo -o yaml
```

## Next steps

- **[Read the docs](README.md)** — understand the architecture and design decisions
- **[Explore the API](api/crds.md)** — all CRD schemas and configuration options
- **[Set up MCP](components/mesh-mcp/README.md)** — enable evidence enrichment for smarter proposals
- **[Contribute](contribute.md)** — add analyzers, fix bugs, write docs

## Troubleshooting

**No incidents showing up?**
- Check server logs: `kubectl logs -n talam-system deployment/talam-server`
- Verify the cluster has actual misconfigurations (talam detects real issues, not test noise)
- Check MeshDiagnostics is applied and scanInterval has passed

**Resolution won't apply?**
- Verify `spec.approved: true` is set
- Check agent logs for apply errors
- Run `kubectl auth can-i patch destinationrules --as=system:serviceaccount:talam-system:talam-agent` to verify RBAC

**Can't see mesh resources?**
- Verify Istio is installed: `kubectl get ns istio-system`
- Check resources exist: `kubectl get destinationrules -A`

Need help? Open an issue on [GitHub](https://github.com/nirvanagit/talam/issues).

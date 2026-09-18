# Guides

Step-by-step guides for common tasks with talam.

## Quick Reference

### Patch a MeshIncident to mark it as reviewed

```bash
kubectl patch meshincident <name> -n <namespace> \
  -p '{"status":{"reviewedBy":"user@example.com"}}'
```

### Reject a resolution proposal

```bash
kubectl patch meshresolution <name> -n <namespace> \
  -p '{"spec":{"approved":false}}'
```

### View the LLM's explanation

```bash
kubectl get meshincident <name> -n <namespace> -o jsonpath='{.status.explanation}'
```

### Check if a resolution was applied

```bash
kubectl get meshresolution <name> -n <namespace> -o jsonpath='{.status.phase}'
# Output: Applied, Rejected, or Pending
```

### Restart talam-agent to resync from server

```bash
kubectl rollout restart deployment/talam-agent -n talam-system
```

### List all incidents in a namespace

```bash
kubectl get meshincidents -n <namespace>
```

### View incident details

```bash
kubectl get meshincident <name> -n <namespace> -o yaml
```

### Watch for new incidents in real-time

```bash
kubectl get meshincidents -n <namespace> -w
```

### Delete a resolved incident

```bash
kubectl delete meshincident <name> -n <namespace>
```

## Coming Soon

More detailed how-to guides:
- Custom Analyzers — Write a new deterministic check
- MCP Integration — Add a custom MCP server for evidence enrichment
- Multi-Cluster Setup — Run talam across multiple Kubernetes clusters
- Dry-Run Safety — Validate remediation proposals before applying
- Troubleshooting — Common problems and solutions

Open an issue to [request a guide](https://github.com/nirvanagit/talam/issues).

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

### Check if a resolution has been applied

talam never applies a resolution itself ([ADR-0007](decisions/0007-agent-never-applies-remediation.md)) — this checks whether an external system has reported an outcome:

```bash
kubectl get meshresolution <name> -n <namespace> -o jsonpath='{.status.outcome}'
# Empty until an external system reports; then "Applied" or "Failed"
```

### Report an outcome manually (no external system subscribed)

If nothing in the cluster is watching `MeshResolution` objects, apply the patch yourself and report it so talam's incident tracking closes:

```bash
kubectl patch meshresolution <name> -n <namespace> --subresource=status -p \
  '{"status":{"outcome":"Applied","appliedBy":"me","appliedAt":"'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'"}}'
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
- External System Integration — Subscribe a GitOps controller or custom operator to `MeshResolution` objects ([ADR-0007](decisions/0007-agent-never-applies-remediation.md))
- Troubleshooting — Common problems and solutions

Open an issue to [request a guide](https://github.com/nirvanagit/talam/issues).

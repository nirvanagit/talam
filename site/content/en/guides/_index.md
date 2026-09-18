---
title: "Guides"
weight: 8
description: "How-to guides, examples, and recipes for talam."
---

Step-by-step guides for common tasks.

## Available Guides

- **[Custom Analyzers]({{< relref "custom-analyzers" >}})** — Write a new deterministic check
- **[MCP Integration]({{< relref "mcp-integration" >}})** — Add a custom MCP server for evidence enrichment
- **[Multi-Cluster Setup]({{< relref "multi-cluster" >}})** — Run talam across multiple Kubernetes clusters
- **[Dry-Run Safety]({{< relref "dry-run-safety" >}})** — Validate remediation proposals before applying
- **[Troubleshooting]({{< relref "troubleshooting" >}})** — Common problems and solutions

---

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

---

More guides coming soon. [Suggest a guide](https://github.com/nirvanagit/talam/discussions).

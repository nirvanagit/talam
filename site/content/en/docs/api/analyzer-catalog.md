---
title: "Analyzer catalog (v0.1 scope)"
weight: 20
---
**Related:** reads [`../concepts/analyzer-interface.md`](/docs/concepts/analyzer-interface/); read by [`../components/agent/README.md`](/docs/components/agent/), [`crds.md`](/docs/api/crds/)

Ships with a focused set of high-signal Istio checks rather than broad shallow coverage — each one maps to a failure mode that's genuinely hard to spot by eye across dozens of CRDs. Each row is a concrete implementation of the [`Analyzer` interface](/docs/concepts/analyzer-interface/).

| Analyzer ID | Detects | Default severity |
|---|---|---|
| `destinationrule.orphaned-subset` | Subset selector matches zero pods with live endpoints. | Critical |
| `virtualservice.dangling-host` | Host referenced has no backing Service/ServiceEntry in the mesh. | Critical |
| `peerauth.mtls-mismatch` | PeerAuthentication STRICT on a workload a client still calls over plaintext. | Critical |
| `gateway.unbound-selector` | Gateway selector matches no ingress workload. | Warning |
| `sidecar.xds-drift` | Envoy's live xDS config diverges from istiod's last-pushed config. | Warning |
| `authz.deny-all-shadow` | An AuthorizationPolicy silently deny-alls traffic another policy meant to allow. | Critical |
| `sidecar.injection-skew` | Namespace labeled for injection but running pods predate the label / proxy version skew vs. istiod. | Warning |
| `serviceentry.unused` | ServiceEntry defined but no traffic or reference observed — hygiene signal. | Info |

Adding an analyzer means adding a row here plus an implementation registered with the [agent](/docs/components/agent/)'s engine — the catalog is the contract, the Go type is the implementation.

---
title: "Finding and Incident"
weight: 20
---
**Related:** reads [`analyzer-interface.md`](/docs/concepts/analyzer-interface/), [`ADR-0005`](/docs/decisions/0005-crd-native-incidents-and-resolutions/); read by [`../components/agent/README.md`](/docs/components/agent/), [`../components/server/README.md`](/docs/components/server/), [`remediation-flow.md`](/docs/concepts/remediation-flow/); see also [`../glossary/README.md`](/docs/glossary/), [`../api/crds.md`](/docs/api/crds/#meshincident--meshresolution)

## Finding

The output of one [`Analyzer`](/docs/concepts/analyzer-interface/) run. Produced entirely on the [agent](/docs/components/agent/), before anything is sent to the server.

```go
type Finding struct {
    AnalyzerID   string
    Severity     Severity       // Critical | Warning | Info
    Resource     ResourceRef    // kind, namespace, name
    RelatedRefs  []ResourceRef  // e.g. the Service a VirtualService points at
    RawEvidence  map[string]any // structured facts, not prose
    DetectedAt   time.Time
}
```

`RawEvidence` is deliberately structured data, never a pre-written explanation — the prose explanation is the [LLM gateway](/docs/components/server/)'s job, downstream, and only after evidence has been redacted per [`security-model.md`](/docs/concepts/security-model/).

## Incident

An Incident is the server-side correlation of one or more Findings that describe the same underlying problem — for example, the same broken mTLS handshake reported independently from both the client-side and server-side sidecar, or from two different clusters sharing a mesh boundary. Correlation is the [server](/docs/components/server/)'s job; agents never produce Incidents from scratch, only Findings.

An Incident is what actually gets shown to a user and what a [`RemediationProposal`](/docs/concepts/remediation-flow/) is generated against — proposing a fix against every duplicate Finding independently would just be noise.

Per [ADR-0005](/docs/decisions/0005-crd-native-incidents-and-resolutions/), the agent *does* see Incidents as of v0.1: it mirrors each one relevant to its own cluster into a [`MeshIncident`](/docs/api/crds/#meshincident--meshresolution) CR, which is also where `Complete` — every associated remediation has been performed — is tracked, computed locally from the `MeshResolution` objects referencing it.

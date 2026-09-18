# Finding and Incident

**Related:** reads [`analyzer-interface.md`](analyzer-interface.md), [`ADR-0005`](../decisions/0005-crd-native-incidents-and-resolutions.md); read by [`../components/agent/README.md`](../components/agent/README.md), [`../components/server/README.md`](../components/server/README.md), [`remediation-flow.md`](remediation-flow.md); see also [`../glossary/README.md`](../glossary/README.md), [`../api/crds.md`](../api/crds.md#meshincident--meshresolution)

## Finding

The output of one [`Analyzer`](analyzer-interface.md) run. Produced entirely on the [agent](../components/agent/README.md), before anything is sent to the server.

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

`RawEvidence` is deliberately structured data, never a pre-written explanation — the prose explanation is the [LLM gateway](../components/server/README.md)'s job, downstream, and only after evidence has been redacted per [`security-model.md`](security-model.md).

## Incident

An Incident is the server-side correlation of one or more Findings that describe the same underlying problem — for example, the same broken mTLS handshake reported independently from both the client-side and server-side sidecar, or from two different clusters sharing a mesh boundary. Correlation is the [server](../components/server/README.md)'s job; agents never produce Incidents from scratch, only Findings.

An Incident is what actually gets shown to a user and what a [`RemediationProposal`](remediation-flow.md) is generated against — proposing a fix against every duplicate Finding independently would just be noise.

Per [ADR-0005](../decisions/0005-crd-native-incidents-and-resolutions.md), the agent *does* see Incidents as of v0.1: it mirrors each one relevant to its own cluster into a [`MeshIncident`](../api/crds.md#meshincident--meshresolution) CR, which is also where `Complete` — every associated remediation has a reported outcome, from whichever external system applied it ([ADR-0007](../decisions/0007-agent-never-applies-remediation.md)) — is tracked, computed locally from the `MeshResolution` objects referencing it.

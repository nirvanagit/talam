---
title: "talam"
---

{{% blocks/cover title="talam" image_anchor="top" height="full" color="primary" %}}
<div class="mx-auto">
	<p class="lead mt-4">Take A Look At Mesh.</p>
	<p class="lead-sub">Or just: <strong>tell 'em</strong> — what's actually wrong with your service mesh, before your users have to.</p>
	<a class="btn btn-lg btn-primary me-3 mb-4" href="{{< relref "/docs" >}}">
		Read the docs <i class="fa-solid fa-book-open ms-2"></i>
	</a>
	<a class="btn btn-lg btn-secondary me-3 mb-4" href="https://github.com/nirvanagit/talam">
		GitHub <i class="fa-brands fa-github ms-2"></i>
	</a>
	<p class="lead mt-5" style="opacity:.75; font-size: 1rem;">v0.1 — pre-release</p>
</div>
{{% /blocks/cover %}}

{{% blocks/lead color="white" %}}
talam reads Kubernetes and Istio state, runs deterministic analyzers against it, and hands the results to an LLM to explain in plain English — the same loop [k8sgpt](https://k8sgpt.ai) runs one layer down the stack. talam's failures live one layer up: not "pod won't schedule" but **"pod is healthy, mesh routing is not."**
{{% /blocks/lead %}}

{{% blocks/section color="light" type="row" %}}

{{% blocks/feature icon="fa-solid fa-magnifying-glass" title="Deterministic first" %}}
Every finding starts as a rule-based check against real API/xDS state. The LLM explains and proposes — it never originates a diagnosis.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-solid fa-hand" title="No blind writes" %}}
talam never mutates cluster state without explicit human approval. Every apply is dry-run first, resourceVersion-gated, and fully audited.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-solid fa-diagram-project" title="Fleet-native" %}}
One server, many clusters, many agents. A mesh boundary spanning clusters is a first-class case, not an afterthought.
{{% /blocks/feature %}}

{{% /blocks/section %}}

{{% blocks/section color="dark" type="row" %}}

{{% blocks/feature icon="fa-solid fa-rocket" title="Get started in 5 minutes" url="/getting-started/" %}}
Install on your cluster, trigger a scan, and approve your first remediation. [Quick start →]({{< relref "/getting-started" >}})
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-solid fa-cubes" title="Everything is an object" %}}
Incidents, remediation, LLM provider selection, MCP evidence sources — all Kubernetes CRDs, watched and reconciled by controllers, `kubectl`-visible end to end. See the [object model]({{< relref "/docs/concepts/object-model" >}}).
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-solid fa-robot" title="LLM does the talking, not the deciding" %}}
Detection is rule-based and testable. The LLM's job is exactly two calls per finding — explain, then propose — never origination. See [ADR-0002]({{< relref "/docs/decisions/0002-deterministic-analyzers-then-llm" >}}).
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-brands fa-github" title="Contributions welcome!" url="https://github.com/nirvanagit/talam" %}}
talam is early — v0.1 ships a focused Istio analyzer catalog, manual-approval remediation, and a Kubernetes-native object model end to end. [Learn how to contribute →]({{< relref "/contribute" >}})
{{% /blocks/feature %}}

{{% /blocks/section %}}

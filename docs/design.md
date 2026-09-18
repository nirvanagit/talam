# Design System

talam's identity reflects its purpose: diagnostic clarity in a complex mesh.

## Brand

**Name:** talam (backronym)
- **Take A Look At Mesh** — what the tool does
- **Tell Them** — what it's for (tell them what's wrong with the mesh)

**Tagline:** "What's actually wrong with your service mesh, before your users have to."

## Visual Identity

### Logo

The talam logo is a **teal triangle mesh** — three nodes (clusters/services) connected by lines (network paths), representing mesh diagnostics at a glance.

**Colors:**
- Primary node (top-left): `#0f766e` (deep teal)
- Secondary node (top-right): `#0891b2` (cyan)
- Tertiary node (bottom): `#075985` (dark blue)

**Usage:**
- Keep minimum 30px for readability
- Maintain aspect ratio (square icon)
- Use on light and dark backgrounds (contrast ≥ 4.5:1)
- Single color: use `#0f766e`

See [site/static/icons/logo.svg](../site/static/icons/logo.svg) for the SVG.

### Color Palette

| Name | Hex | Usage |
|---|---|---|
| Primary | `#0f766e` | Buttons, links, headings |
| Secondary | `#0891b2` | Accents, highlights |
| Tertiary | `#075985` | Backgrounds, dark elements |
| Success | `#10b981` | Passing checks, applied resolutions |
| Warning | `#f59e0b` | Pending approvals, warnings |
| Danger | `#ef4444` | Failed checks, errors |
| Light | `#f3f4f6` | Light backgrounds |
| Dark | `#0b1220` | Text, dark backgrounds |

### Typography

- **Display:** Inter, 32–48px, 600–700 weight (headings)
- **Body:** Inter, 14–16px, 400 weight (paragraphs)
- **Monospace:** Monaco or Courier New, 12–14px (code, commands)

## Design Principles

### 1. Determinism First
Every visual should reinforce that talam's findings are rule-based, not guessed. Use checkmarks for confirmed states, not approximations.

### 2. Clarity Over Prettiness
Show the mesh state clearly. Color conveys status (green = healthy, yellow = caution, red = error), not decoration.

### 3. Human-in-the-Loop, and Never talam's Own Hand
Require explicit approval before a proposal is exposed to whatever external system applies it — talam never applies a patch itself ([ADR-0007](decisions/0007-agent-never-applies-remediation.md)). Make the review UX prominent, and make it visually obvious that "Approved" hands off to another system rather than completing the fix.

### 4. Audit Trail
Every action leaves a trace. Logs, proposals, outcomes are all visible and searchable in CRDs.

## UI Components

### Status Indicators

```
✓ Applied         (green, checkmark — reported by an external system)
⏳ Pending        (yellow, hourglass)
✗ Failed          (red, X)
→ Proposed        (blue, arrow)
🔌 Approved, awaiting external system (violet, plug — approved but no outcome reported yet)
```

### Incident Card

```
[Incident Title]
Namespace: default | Severity: High | Time: 2m ago

Rule: destination-rule-unused
Resource: bookinfo/reviews
Impact: Traffic not reaching v3 backend

Resolutions: 1 (1 approved, awaiting external system)
```

### Resolution Panel

```
Proposed by: LLM (v0.1)
Approved by: [user@example.com] ✓

talam does not apply this patch — approving exposes it to whatever
system (GitOps, config pipeline, kubectl) is subscribed in this cluster.

[Approve] [Reject] [View Patch YAML] [Copy kubectl command]
```

No "Apply" button exists anywhere in the UI — the strongest signal that talam never performs the write itself. A reviewer approves a *proposal*, not an *action*.

## Content Tone

- **Informative:** Explain *why* a check failed, not just *that* it failed
- **Direct:** "Pod is healthy, mesh routing is not" — avoid vague terms
- **Technical but accessible:** Assume kubectl familiarity, not Istio mastery
- **Action-oriented:** Every message should point to next steps (approve, investigate, check logs)

## Contributing

Maintain consistency:
- Use the color palette for all new UI components
- Follow the status indicator conventions
- Write copy in the voice above
- Get design review before major UI changes

Open an issue or PR on [GitHub](https://github.com/nirvanagit/talam) to discuss.

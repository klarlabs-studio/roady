# Roady Core – Product Requirements Document

## Product Mission

Enable developers and AI agents to **plan once, execute many times, and detect drift continuously**.

---

## Target Users

### Primary
- Individual developers
- Senior engineers using AI tools
- Open-source maintainers

### Secondary
- Tech leads
- Consultants hopping between repos

---

## Key Jobs To Be Done

### JTBD 1 – Planning
“When I start a project, I want a clear plan derived from intent, not a blank slate.”

### JTBD 2 – Continuity
“When I come back later (or someone else does), I want to know exactly where we are.”

### JTBD 3 – Drift Awareness
“When reality changes, I want to know *what* drifted and *why*.”

### JTBD 4 – AI Efficiency
“When using AI, I don’t want to keep re-explaining context.”

---

## Core Functional Areas

---

### 1. Spec Lifecycle

#### Inputs
- Existing PRDs
- Markdown docs
- README files
- Guided prompts
- AI-assisted inference (optional)

#### Capabilities
- Generate initial ProductSpec
- Normalize & validate
- Version and hash features
- Explain spec back to user

#### Non-goals
- No speculative requirements
- No silent mutation

---

### 2. Plan Lifecycle

#### Plan Definition
- DAG of tasks
- Each task:
  - references a feature
  - pins a feature hash
  - declares dependencies
  - declares skill + model hint

#### Capabilities
- Deterministic generation
- AI-assisted suggestion
- Human approval required
- Diffable plans

---

### 3. Drift Detection

#### Drift Types
- Spec drift
- Plan drift
- Code drift
- Policy drift

#### Output
- Structured findings
- SARIF output
- Human-readable summaries

#### Philosophy
Drift ≠ failure  
Undetected drift = failure

---

### 4. Status & Querying

Roady must answer instantly:
- What phase are we in?
- What tasks are active?
- What’s blocked?
- What drift exists?
- What changed recently?

---

### 5. CLI & MCP

#### CLI
- Scriptable
- Explicit verbs
- No magic defaults

#### MCP
- Enables AI tools to:
  - read spec
  - read plan
  - ask status
  - propose changes

---

## Collaboration Model (Core)

- Git-based async
- `.roady/` committed optionally
- No real-time locking
- Conflicts resolved like code

---

## Out of Scope (Core)

> Corrected 2026-08. This list previously named ownership tracking, team
> dashboards, forecasting, billing, and compliance as out of scope. All of
> them shipped in Core, so the list described the opposite of reality.

- Real-time multi-user presence (state syncs through git, not a server)
- A hosted control plane — see `ROADMAP.md` for the Roady Cloud boundary
- Authenticated identity: actors are asserted, never verified
  (see `docs/audit-grc.md`)

---

## Success Metrics

- Reduced AI token usage
- Reduced replanning frequency
- Faster onboarding
- Earlier drift detection
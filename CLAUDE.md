# AI-DLC and Spec-Driven Development

Kiro-style Spec Driven Development implementation on AI-DLC (AI Development Life Cycle)

## Project Context

### Paths

- Steering: `.kiro/steering/`
- Specs: `.kiro/specs/`

### Steering vs Specification

**Steering** (`.kiro/steering/`) - Guide AI with project-wide rules and context
**Specs** (`.kiro/specs/`) - Formalize development process for individual features

### Active Specifications

- Check `.kiro/specs/` for active specifications
- Use `/kiro:spec-status [feature-name]` to check progress

## Development Guidelines

- Think in English, generate responses in Simplified Chinese. All Markdown content written to project files (e.g., requirements.md, design.md, tasks.md, research.md, validation reports) MUST be written in the target language configured for this specification (see spec.json.language).
- All output text—including specification-level Markdown—must be clear, concise, and audience-friendly. Use short sentences and active voice. Avoid jargon unless essential; when technical terms are necessary, immediately clarify them in plain language (e.g., in parentheses). Before finalizing, pause and ask: "Can it quickly capture the main point for those unfamiliar with the field? " If not, revise for clarity—then save or output.

## Minimal Workflow

- Phase 0 (optional): `/kiro:steering`, `/kiro:steering-custom`
- Phase 1 (Specification):
  - `/kiro:spec-init "description"`
  - `/kiro:spec-requirements {feature}`
  - `/kiro:validate-gap {feature}` (optional: for existing codebase)
  - `/kiro:spec-design {feature} [-y]`
  - `/kiro:validate-design {feature}` (optional: design review)
  - `/kiro:spec-tasks {feature} [-y]`
- Phase 2 (Implementation): `/kiro:spec-impl {feature} [tasks]`
  - `/kiro:validate-impl {feature}` (optional: after implementation)
- Progress check: `/kiro:spec-status {feature}` (use anytime)

## Development Rules

- 3-phase approval workflow: Requirements → Design → Tasks → Implementation
- Human review required each phase; use `-y` only for intentional fast-track
- Keep steering current and verify alignment with `/kiro:spec-status`
- Follow the user's instructions precisely, and within that scope act autonomously: gather the necessary context and complete the requested work end-to-end in this run, asking questions only when essential information is missing or the instructions are critically ambiguous.

## Steering Configuration

- Load entire `.kiro/steering/` as project memory
- Default files: `product.md`, `tech.md`, `structure.md`
- Custom files are supported (managed via `/kiro:steering-custom`)

## Development SOP (Documentation-First, Mandatory)

### 1) Authority and Traceability

- Treat `docs/*.md` as authority-level design sources for implementation details.
- Keep steering concise. Add `Authority Anchors` in each relevant steering file to point to exact `docs/*.md` sections for details.
- When there is a conflict: `docs/`* authority > active spec docs > steering summary > existing code shape.

### 2) Pre-Coding Dry Run (Required Before Any Task Implementation)

- For each task, run a dry run and record:
  - `Impact Surface`: modules/contracts/tests affected.
  - `Design Delta`: whether implementation differs from authority design.
  - `Execution Decision`: follow design directly, or request deviation handling.
- Do not start coding if this dry run is missing.

### 3) Deviation Control (Major vs Minor)

- **Major deviation**: any change that violates MUST constraints, breaks public contract stability, or introduces safety/data-integrity risk.
  - Action: STOP and wait for explicit confirmation before proceeding.
- **Minor deviation**: does not violate MUST constraints, is reversible, and does not break external contracts.
  - Action: allowed to proceed with explicit deviation record and PR disclosure.

### 4) Deviation Reconciliation (Design and Code Must Converge)

- Deviation records are temporary controls, not permanent truth.
- After deviation approval, select one path and complete it:
  - Align code back to authority design, or
  - Update authority design to reflect the approved new truth.
- Keep design and code synchronized. Avoid long-lived "known deviation" drift.

“For every /kiro:spec-impl run, the agent MUST output the following sections in order:

Authority Anchors

Pre-Coding Dry Run

Implementation Plan (Bound to Docs)

Deviation Report

Post-Implementation Conformance Check”

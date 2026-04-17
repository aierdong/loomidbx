# Deviation Governance Guide

This folder stores deviation records between authority design and implementation.

## 1) When a Deviation Record Is Required

Create a deviation record before implementation if any of the following is true:

- Implementation cannot fully follow authority design in `docs/*`.
- A MUST-level constraint in steering/spec may be violated.
- Public contract behavior may change (FFI/API/JSON/schema semantics).
- Safety or data-integrity risk may be introduced.

Use `docs/deviation-record-template.md` as the template.

## 2) Deviation Classification

### Major Deviation (Stop-the-line)

A deviation is **Major** when at least one of these conditions is met:

- Violates MUST constraints.
- Breaks stable external/public contracts.
- Introduces safety or data-integrity risk.

Required action:

- Stop implementation and request explicit confirmation.
- Do not continue coding until approval is granted.

### Minor Deviation (Allowed with Traceability)

A deviation is **Minor** when all of the following are true:

- Does not violate MUST constraints.
- Does not break external/public contracts.
- Is reversible with bounded impact.

Required action:

- Record deviation details and proceed.
- Disclose the deviation in PR checklist.

## 3) Reconciliation Policy (Design and Code Must Converge)

Deviation records are temporary controls. After approval, choose one path and complete it:

- Align code back to authority design, or
- Update authority design to reflect approved implementation truth.

Do not keep long-lived unresolved drift between design and code.

## 4) TTL Policy (Time-to-Live)

Each deviation record must include:

- Target version or due date
- Owner
- Closure criteria

If TTL is reached and record is still unresolved, escalate it to blocking status for subsequent related changes.

## 5) Suggested Workflow

1. Complete pre-coding dry run (`docs/dry-run-template.md`).
2. Detect design delta.
3. Classify as Major/Minor.
4. Create deviation record if needed.
5. Implement with explicit PR disclosure.
6. Reconcile by code or authority design update.
7. Close deviation after validation.
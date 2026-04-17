# Deviation Record Template

Use this template when implementation cannot fully follow authority design.

## 1) Metadata
- Record ID:
- Spec:
- Task ID / Title:
- Date:
- Author:
- Approver:
- Status: Draft / Approved / Rejected / Resolved

## 2) Deviation Summary
- Authority baseline (expected):
- Actual implementation (current):
- Deviation type: Architecture / Contract / Data Model / File Structure / Process / Other

## 3) Classification
- Level: Major / Minor
- Why this level:

Major means at least one of:
- Violates MUST constraints
- Breaks stable public contract
- Introduces safety or data-integrity risk

## 4) Root Cause
- Why deviation happened:
- Constraints (time/dependency/performance/compatibility):
- Alternative options considered:

## 5) Impact and Risk
- Affected modules:
- Affected contracts:
- User/business impact:
- Risk level: Low / Medium / High

## 6) Decision and Approval
- Decision:
  - [ ] Stop and wait for confirmation
  - [ ] Continue with approved minor deviation
- Approval notes:

## 7) Reconciliation Plan (Mandatory)
- Final direction:
  - [ ] Align code back to authority design
  - [ ] Update authority design to become new source of truth
- Target version / due date:
- Owner:
- Validation checks:

## 8) Linked Changes
- Related doc updates:
- Related code changes:
- Related tests:
- Related PR/commit:

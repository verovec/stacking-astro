---
id: NNNN
title: <imperative, ≤ 60 chars>
epic: —            # or ENN
status: todo       # todo | claimed | in-review | done | blocked | dropped
depends_on: []     # card IDs that must be done first
branch: card/NNNN-slug
---

# NNNN · <title>

## Context
<!-- WHY this card exists + the few pointers a zero-context worker needs. Pointers,
     not prose: `internal/pkg/file.go:123`, `stacking-doc.md §N`, `tasks-context/x.md`,
     `docs/x.md`. 5–10 lines. Never paste code, never restate what a pointer says. -->

## Approach
<!-- HOW: what existing code to REUSE (exact packages/functions, verified via gitnexus),
     what to add and where, each rejected alternative in one line. Honour the repo
     invariants (defaults stay byte-identical; no soft-fail to a different algorithm;
     single-sourced catalogues). If this section needs a weird pattern or special-case
     cascade, the card is mis-scoped — fix the scoping, not the pattern. -->

## Acceptance criteria
<!-- Verifiable and test-shaped: each criterion names the test that proves it
     (TestType_Method_Scenario + file, per conventions/testing-conventions.md). -->
- [ ] …
- [ ] Existing suite stays green: `just check` passes; no existing test modified or
      deleted without justification in the PR body.

## Todos (TDD order)
<!-- Red → green → refactor per slice; the first todo is always a failing test.
     The worker ticks these in place while implementing. -->
- [ ] Red: …
- [ ] Green: …
- [ ] Refactor: …

---
name: new-card
description: "Use when the PO asks for a new integration card or epic — 'write a card for X', 'new card:', 'plan X', 'add X to the roadmap'. Authors a TDD-oriented card in cards/ (or an epic + child cards), registers it in roadmap.md, and bootstraps roadmap.md on first use. For implementing cards use pick-card instead."
---

# new-card — author an integration card

You are writing for a future worker session that has zero context beyond the repo and
the card. Every sentence must either aim the worker at the right code or constrain the
solution — anything else is wasted context. Cut it.

## 0 · Bootstrap (first use only)

If `roadmap.md` is missing at the repo root: copy `bootstrap/roadmap.md` (next to this
skill) there, `mkdir -p cards/`, and copy the other `bootstrap/*.md` files into
`cards/` — they seed the fork's first epic (E01, prune to the stacking core) and its
scoping card 0001. Roadmap and cards are gitignored on purpose — local planning state,
like `tasks-context/`. Never commit them; never "fix" the gitignore.

## 1 · Understand before writing

A card written without reading code is guesswork. Before drafting:

- **gitnexus first** (`repo:"astronomy"`): `query` the concept, `context` every symbol
  the card will name, `impact` anything it changes. Run `just gitnexus-sync` first if
  the graph may be stale.
- **Point at the maps, don't restate them**: `stacking-doc.md` (repo root, gitignored)
  is THE technical map — every stack path, the `stackalg` catalogue, the stretch chain;
  its §7 is the checklist for integrating a new algorithm/stretch/mode/preset, §8.3 a
  ready backlog, Appendix B the file map. `tasks-context/*.md` holds operating
  procedures; `docs/` the architecture and per-mode pages.
- **Scope guard**: this fork only cares about the stacking/processing core — inspect →
  calib → register/stack (`siril` | `stacknative` | mode-owned) → stretch/finish chain,
  plus the minimal job/API/store/UI needed to run and watch a run (see stacking-doc.md
  Appendix B). Asked for a card clearly outside that (capture hardware, mount, S3,
  non-stacking UI)? Ask the PO one question, then act on the answer.

## 2 · Card or epic?

A **card** = one branch, one PR, all tests green, ≤ ~1 focused day of work. If honest
scoping needs more, create an **epic** (`cards/ENN-slug.epic.md` from
`templates/epic.md`) plus child cards. Split rules:

- Vertical slices — each child independently mergeable and testable — not layers.
- First child = the seam: types/interfaces plus the failing tests that pin the contract.
- A child that cannot state a testable acceptance criterion is not a card; merge it
  into its neighbour or rethink the split.
- When later children depend on earlier findings, specify only what is honest today and
  end the card map with a planning child that carves the rest.

## 3 · Write it — from `templates/card.md`

ID = next free 4-digit number in the roadmap Queue (never reused); file
`cards/NNNN-slug.md`. Section by section:

- **Context** — why the card exists + the pointers needed to work it
  (`internal/pkg/file.go:123`, `stacking-doc.md §N`, `tasks-context/x.md`). 5–10 lines,
  no pasted code, nothing a pointer already says.
- **Approach** — what to REUSE (exact packages/functions, verified via gitnexus — never
  from memory), what to add and where, each rejected alternative in one line. Honour
  the repo invariants: un-configured runs stay byte-identical, no soft-fail to a
  different algorithm, single-sourced catalogues (`stackalg`, `filters`). If the
  approach needs a weird pattern or a special-case cascade, the scoping is wrong —
  rescope now, not at build time.
- **Acceptance criteria** — verifiable and test-shaped: each names the Go test that
  proves it (`TestType_Method_Scenario`, table-driven, per
  `conventions/testing-conventions.md`). Always keep the template's final criterion:
  existing suite green, no test weakened without justification.
- **Todos** — strict TDD order, red → green → refactor per slice; the first todo is
  always a failing test. If the card adds a wire knob, include its `paramDocs` glossary
  entry (en + fr) and doc row as todos.

## 4 · Register

Every card gets a Queue row (row position = priority: default bottom; ask the PO if it
should jump the line) and a Journal line
`YYYY-MM-DD · NNNN · created (PO: <one-line ask>)`. An epic gets a Journal line and its
`.epic.md` file; only its children get Queue rows.

## 5 · Report

Reply with each card's path + one-line summary, and (for a split) the rationale in ≤ 2
lines. Do not paste card contents back into the conversation.

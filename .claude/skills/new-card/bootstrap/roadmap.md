# Roadmap — AstroStack fork (local, gitignored like `cards/`)

Single source of truth for card ordering, dependencies and status. Cards live in
`cards/`; this file decides what gets worked on next. The PO creates and orders; the
worker claims and releases. **Every status change = one Queue update + one Journal
line — no silent transitions.** Keep every entry to one line; this file is read at the
start of every card session.

## Protocol

**Card** — the only unit of work: one branch (`card/NNNN-slug`), one PR, mergeable on
its own with all tests green. Scope ≤ ~1 focused day / one reviewable diff. File:
`cards/NNNN-slug.md`, 4-digit global sequence, IDs never reused.

**Epic** — a feature too big for one card: overview file `cards/ENN-slug.epic.md`
listing its child cards; children are ordinary cards carrying `epic: ENN`. An epic is
never claimed or merged — only its cards are. An epic is done when all its children are.

**Statuses** — `todo → claimed → in-review → done`, plus `blocked` (worker hit a wall —
reason in Journal) and `dropped` (PO cancelled). `in-review` = PR open, CI + code
review running. On conflict between a card's frontmatter and this file, **this file
wins**.

**Pick rule (worker)** — topmost Queue row with status `todo` whose `Depends on` are
all `done`. Row order IS priority: the PO owns it; the worker never reorders, never
skips ahead, and holds at most one claimed card at a time.

**Claim / release** — claim: Queue row → `claimed`, Journal line, card frontmatter
updated, branch created. Release: same three writes with the outcome (`done` + PR
number, `blocked` + one-line reason, or back to `todo`).

## Queue

| ID   | Card | Epic | Depends on | Status | Notes |
|------|------|------|------------|--------|-------|
| 0001 | Map the stacking core and carve the prune epic | E01 | — | todo | planning card — outputs child cards, no code change |

## Journal

<!-- newest first · `YYYY-MM-DD · ID · event (detail)` -->
- 2026-09-20 · 0001 · created (PO: inventory keep/drop before any pruning; carves E01's children)
- 2026-09-20 · E01 · created (PO: prune the fork to the stacking/processing core — drop capture/mount/S3/non-stacking UI/docker extras)
- 2026-09-20 · — · workspace bootstrapped (roadmap + cards/ + new-card/pick-card skills + tests.yml CI gate)

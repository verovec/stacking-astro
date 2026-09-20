---
name: pick-card
description: "Use when asked to work the roadmap — 'pick a card', 'work the next card', 'implement card 0004', 'continue the roadmap'. Claims the next eligible card from roadmap.md, implements it strictly TDD reusing existing code, gets the diff code-reviewed, opens a PR and merges to main only when CI is green. For writing cards use new-card instead."
---

# pick-card — claim, build (TDD), review, merge

The roadmap protocol (statuses, pick rule, claim/release) lives in `roadmap.md` at the
repo root — it is authoritative. This skill is the worker's execution procedure.

## 1 · Sync & pick

- `git switch main && git pull`, then `just gitnexus-sync` and let it finish.
- Read `roadmap.md`. Pick per its Pick rule: topmost Queue row with status `todo`
  whose deps are all `done`. If the PO named a card, take that one — after verifying
  its deps; unmet deps → report and stop. Nothing eligible → report what blocks and stop.
- One claimed card at a time. Never reorder the Queue.

## 2 · Claim

Three writes together, before any code: card frontmatter `status: claimed`; Queue row
→ `claimed`; Journal line `YYYY-MM-DD · NNNN · claimed`. Then
`git switch -c card/NNNN-slug`.

## 3 · Load context

Read exactly what the card's Context points at, plus the conventions for the areas
touched (`testing` + `golang` always; others per the CLAUDE.md list). Do not re-explore
what the card already answers; do not skip what it points at.

## 4 · Implement — strict TDD, reuse first

- Per todo: **red** — write the test, run it, watch it fail for the right reason;
  **green** — minimal change to pass; **refactor** — with the suite green. Tick todos
  in the card file as you go.
- Before writing any new function: gitnexus `context`/`query` (`repo:"astronomy"`) for
  an existing one to reuse or extend. Duplicating existing logic is a review blocker.
- All new backend code ships with tests (table-driven, colocated, per
  `conventions/testing-conventions.md`). Never weaken or delete an existing test
  without justification recorded in the PR body.
- **Sanity valve** — if the card's approach forces a weird pattern, a special-case
  cascade, or a fight with the existing architecture (`impact` will show it): STOP.
  Do not force it. Write what you learned into the card's Approach, set `blocked`
  (card + Queue + Journal, one-line reason), leave main untouched, and report a
  proposed rescope to the PO.

## 5 · Verify & review

- `just check` (lint + full host test suite) and `just build` must pass.
- Run the code-review skill on the working diff (`/code-review`). Fix every finding you
  agree with; record the ones you reject and why (they go in the PR body). Re-run
  `just check` after fixes.

## 6 · Ship

- Commit per `conventions/commit-conventions.md` (scope = the card's area; no AI
  attribution anywhere in history).
- Push and `gh pr create` — title = the card title in commit style, body = card ID,
  the acceptance criteria as a checklist, and the review notes.
- CI (`.github/workflows/tests.yml`) runs the hermetic unit suite on the PR. Merge
  only when green: `gh pr merge --auto --squash`; if the repo refuses auto-merge, watch
  with `gh pr checks --watch` and `gh pr merge --squash` once green. CI red → fix on
  the branch; never merge red, never bypass the review.

## 7 · Release

After the merge: card frontmatter `done` (acceptance boxes ticked), Queue row `done`,
Journal line `YYYY-MM-DD · NNNN · done (PR #N)`. `git switch main && git pull`, delete
the branch. If the session must end before the merge, leave a Journal line saying
exactly where the card stands — a claim without a journal trail is a protocol
violation.

## Report

One short paragraph: what merged (PR link), the tests that now pin the behaviour, and
anything the next card should know.

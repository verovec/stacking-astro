---
id: 0001
title: Map the stacking core and carve the prune epic
epic: E01
status: todo
depends_on: []
branch: card/0001-map-stacking-core
---

# 0001 · Map the stacking core and carve the prune epic

## Context

This fork keeps only the stacking/processing core (E01 Goal). `internal/` holds ~60
packages; many exist for features the fork drops (capture, device, mount, polar
alignment, guiding, S3/backup, dark-sky planning…). The keep-set seed is
`stacking-doc.md` Appendix B (file map of every stacking concern) + its §6 finish
chain; CLAUDE.md lists the subsystems and their coupling. Deleting by guesswork would
strand hidden importers — the map must come from the actual import graph. **Planning
card: the deliverable is the validated keep/drop map + E01's child cards, no
production code changes.**

## Approach

From the entry points that must survive — `pipeline.Process`, each kept
`internal/pipeline/<mode>mode.go`, `job.Manager`, `cmd/astrostack` (serve/process),
`store.Migrate` — build the import closure (`go list -deps ./...` cross-checked with
gitnexus `impact`, `repo:"astronomy"`). Classify every `internal/` package, `cmd/`
binary, `frontend/src` route/page/store, `compose.yaml` service, justfile recipe and
`go.mod`/`package.json` dependency as **keep / drop / trim** (trim = kept file loses
its dropped-feature branches). PO validates the map, then carve one child card per
severable subtree, ordered so each removal leaves the build green (leaf features
first, shared plumbing last).

Open PO decisions to resolve at validation, with proposed defaults: which modes stay
(`livestack` imports capture — propose drop; `sun`/`eclipse` are stacking — propose
keep); Postgres + job/API/web monitoring stays (propose yes — CLAUDE.md mandates
processing runs as monitorable jobs); S3 mirroring (propose drop); capture/mount/
device/polar subtrees go entirely (propose yes).

## Acceptance criteria

- [ ] E01's Card map lists every dropped/trimmed unit with its import-closure evidence
      (who imported it, why it is severable) — no "probably unused" entries.
- [ ] Child cards 0002+ exist in `cards/` and the roadmap Queue, in an order where
      every prefix of removals keeps `just build` + `just check` green; each child's
      acceptance includes a deepsky smoke job (`POST /api/jobs`) completing.
- [ ] PO sign-off on the keep/drop map recorded as a Journal line.
- [ ] `git status` clean of production-code changes — this card ships cards, not code.

## Todos

<!-- planning card — red/green/refactor does not apply; the TDD gate lives in the
     children's acceptance criteria -->
- [ ] Build the import closure from the surviving entry points (`go list -deps` +
      gitnexus `impact`); dump keep/drop/trim tables into E01's Card map.
- [ ] Sweep the non-Go surfaces against the map: `frontend/src` routes/stores,
      `compose.yaml`, `justfile`, CI, `go.mod`/`package.json` deps.
- [ ] Present the map + open decisions to the PO; record the sign-off in the Journal.
- [ ] Write the child cards (via the new-card skill) and register them in the Queue.

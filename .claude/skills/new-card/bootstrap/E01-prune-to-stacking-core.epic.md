---
id: E01
title: Prune the fork to the stacking core
status: open
cards: [0001]
---

# E01 · Prune the fork to the stacking core

## Goal

The fork builds, tests and runs with only the stacking/processing core: inspect →
calib → register/stack (`siril` | `stacknative` | mode-owned engines) → stretch/finish
(GraXpert/StarNet/GIMP/supervisor), plus the minimal job/API/store/frontend needed to
submit a run and watch it. Everything serving only dropped features goes with them:
capture/device/mount/polar/guide/focus, S3/backup mirroring, non-stacking frontend
pages, Docker services, justfile recipes and Go/npm dependencies. OUT of scope:
rewriting or "improving" anything kept — every child card is a pure removal/trim that
leaves `just check` green and a smoke stacking job passing.

## Card map

- 0001 Map the stacking core and carve the prune epic — dependency-closure keep/drop
  map over `internal/`, `cmd/`, `frontend/src`, `compose.yaml`, `justfile`, validated
  by the PO; carves the removal children below it. (deps: —)
- <!-- children 0002+ are carved by 0001; do not guess them here -->

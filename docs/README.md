# Documentation

| Topic | Where |
|-------|-------|
| Start here: clone to first stacked image, with the failure modes | [`getting-started.md`](getting-started.md) |
| Recipes, prerequisites, host mode, the web UI, configuration pointers | [`usage.md`](usage.md) |
| Every environment variable | [`configuration.md`](configuration.md) |
| System shape, components, containerized `stack` mode, provenance and tool health | [`architecture.md`](architecture.md) |
| How stacking is made, stage by stage | [`pipeline.md`](pipeline.md) |
| Combination methods, rejection algorithms, normalization and weighting | [`stacking.md`](stacking.md) |
| Master library, cross-session pools, dark defect maps, matching rules | [`calibration.md`](calibration.md) |
| Per-mode deep dives, one page each: deepsky, nebula, milkyway, nightpano, planetary, comet, mosaic, sun, eclipse | [`modes/`](modes/README.md) |
| Worked example: NGC 7000 from 100 duo-band subs to the finished image, every number measured | [`examples/ngc7000-coloured-detailed.md`](examples/ngc7000-coloured-detailed.md) |
| The HTTP API reference | [`api.md`](api.md) |
| The web UI, page by page | [`ui.md`](ui.md) |
| The local AI: finish supervisor, supervised conversations, series | [`agent.md`](agent.md) |
| Every external tool, catalogue, data service and library, with its licence | [`third-party.md`](third-party.md) |
| End-to-end verification recipes with pass criteria | [`verification.md`](verification.md) |
| Parked code (the dense planetary aligner), not compiled, kept for reference | [`attic/`](attic/README.md) |
| README artwork | [`assets/`](assets/) |

The native macOS successor of this engine is
[kappa-sigma](https://github.com/verovec/kappa-sigma); the Go rules documented here are the
reference while they are ported to Swift.

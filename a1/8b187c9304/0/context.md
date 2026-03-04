# Session Context

## User Prompts

### Prompt 1

Implement docs/plans/instance-management-cli.md using the `kasmos-coder` skill. Execute all tasks sequentially.

Reviewer feedback from previous round:
round 1 — changes required.

## critical

- `cmd/instance.go:229-244` — **data loss on state write-back in removeInstanceFromState**: `instanceRecord` is missing ~15 fields present in `session.InstanceData` (`Height`, `Width`, `UpdatedAt`, `AutoYes`, `SkipPermissions`, `TaskNumber`, `WaveNumber`, `PeerCount`, `IsReviewer`, `ImplementationCompl...


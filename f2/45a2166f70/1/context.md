# Session Context

## User Prompts

### Prompt 1

Implement Task 6: Rewrite FormOverlay, PermissionOverlay, and TmuxBrowserOverlay

Load the `kasmos-coder` skill before starting. Also load `cli-tools` for the tool selection reference.

## Plan Context

**Goal:** Clean-room rewrite of `ui/overlay/` to replace 8 ad-hoc overlay structs (each with different APIs) with a unified `Overlay` interface and `OverlayManager`. Eliminates the 27-state switch explosion in `app/app_input.go` and the 20-case render switch in `app/app.go:View()` by making ov...


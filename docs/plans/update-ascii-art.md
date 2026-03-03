# Update ASCII Art: KLIQUE → KASMOS

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the KLIQUE banner art with KASMOS using the same ANSI Shadow figlet font.

**Architecture:** Single constant swap in `ui/consts.go`. The new art is the same height (6 rows) so the frame builder and `blockPeriod` glyph are unchanged.

**Tech Stack:** Go, ANSI Shadow figlet font

---

## Wave 1

### Task 1: Replace banner art and update comments

**Files:**
- Modify: `ui/consts.go`

**Step 1: Replace `fallbackBannerRaw` content**

Change the raw string literal from KLIQUE to KASMOS (generated via `figlet -f "ANSI Shadow" KASMOS`):

```go
// The base KASMOS banner — 6 rows tall.
var fallbackBannerRaw = `██╗  ██╗ █████╗ ███████╗███╗   ███╗ ██████╗ ███████╗
██║ ██╔╝██╔══██╗██╔════╝████╗ ████║██╔═══██╗██╔════╝
█████╔╝ ███████║███████╗██╔████╔██║██║   ██║███████╗
██╔═██╗ ██╔══██║╚════██║██║╚██╔╝██║██║   ██║╚════██║
██║  ██╗██║  ██║███████║██║ ╚═╝ ██║╚██████╔╝███████║
╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝╚═╝     ╚═╝ ╚═════╝ ╚══════╝`
```

**Step 2: Update suffix comments**

Replace the four `// KLIQUE` comments with `// KASMOS`:

```go
		{},                                      // KASMOS
		{blockPeriod},                           // KASMOS.
		{blockPeriod, blockPeriod},              // KASMOS..
		{blockPeriod, blockPeriod, blockPeriod}, // KASMOS...
```

**Step 3: Verify build**

Run: `go build ./ui/...`
Expected: clean build, no errors.

**Step 4: Commit**

```bash
git add ui/consts.go
git commit -m "feat: replace KLIQUE ascii art with KASMOS"
```

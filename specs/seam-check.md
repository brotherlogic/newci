# Feature Spec: `seam-check`

## 1. Overview
`seam-check` uses Go's `go/parser` and `go/ast` to enforce two structural invariants on the Crystal Plane:
1. **Strict 1:1 Pairing:** Every `specs/<feature>.md` must have a matching `tests/<feature>_test.go` (and vice versa), with no orphan specs, orphan tests, or empty spec files.
2. **Public Seam Boundary:** Black-box tests in `tests/<feature>_test.go` must have valid Go syntax and are forbidden from importing any `internal/` implementation package (`<module>/internal/...`, `internal/...`, or `.../internal/...`).

## 2. CLI Commands & Trigger Conditions

`seam-check` runs both as a standalone CLI command (`newci seam-check [feature]`) and automatically as a precondition gate inside `newci lock [feature]`.

### `newci [--dir <path>] seam-check [feature]`
- **Single-Feature Mode (`newci seam-check <feature>`):**
  - Verifies `specs/<feature>.md` exists and is non-empty (`> 0` bytes).
  - Verifies `tests/<feature>_test.go` exists and parses cleanly via `go/parser.ParseFile`.
  - Inspects all AST `import` declarations in `tests/<feature>_test.go` against `<module>/internal` (read from `go.mod`) and any `/internal` path segment.
  - Exits `0` with `Seam check OK for feature "<feature>"` if valid; exits `1` with a descriptive `seam violation` message otherwise.
- **Repository-Wide Mode (`newci seam-check`):**
  - Scans `specs/` and `tests/` for all files.
  - Rejects non-`.md` files in `specs/` and non-`_test.go` files in `tests/`.
  - Rejects any `specs/<feature>.md` missing `tests/<feature>_test.go` or any `tests/<feature>_test.go` missing `specs/<feature>.md`.
  - Runs single-feature AST seam verification across every discovered feature pair.
  - Exits `0` with `Seam check OK (<N> feature pair(s) verified)` if all pass; exits `1` otherwise.

### Gate in `newci [--dir <path>] lock [feature]`
- Before sealing a feature capsule into `.newci/crystal/<feature>.json`, `newci lock` executes both `seam.CheckFeature` and `seam.CheckAll`.
- If any 1:1 pairing error, syntax error, or forbidden `internal/` import exists, `newci lock` aborts with exit code `1` and leaves the feature unsealed.

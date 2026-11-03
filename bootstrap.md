# `newci` Bootstrap Strategy

`newci` is a native **Dual-Plane Version Control & Verification Engine** designed for the **Liquid-to-Crystal (Feature-Locking Ratchet)** workflow (see [`../dcrmux/WORKFLOW.md`](../dcrmux/WORKFLOW.md)).

Instead of wrapping Git branches, Pull Requests, and slow GitHub Actions workflows (like `speculate`), `newci` replaces day-to-day `git` and CI/CD ceremony with a fast local engine and treats GitHub `origin/main` purely as a **one-way publish target** when a feature crystallizes.

---

## 1. Core Architecture: Dual-Plane Engine + One-Way Git Emitter

1. **The Flight Recorder (Liquid Plane):**
   - Watches `cmd/` and `internal/` continuously during Liquid Mode.
   - Automatically records content-addressed **Liquid Frames** in `.newci/liquid/` whenever `go build` (and existing locked tests) succeed.
   - Supports instant local rewind/scrubbing (`newci rewind`, `newci frames`) without commit messages, branch ceremony, or touching locked specs/tests.

2. **The Crystal Vault (Contract Plane):**
   - Owns `specs/<feature>.md` and `tests/<feature>_test.go`.
   - Keeps locked specs and tests **read-only on disk (`0444`)** during Liquid Mode so agents cannot weaken tests or alter ratified specs without an explicit human `unlock`.

3. **The `lock` Transaction (Local Adversarial CI):**
   - Running `newci lock <feature>` executes the full ratification pipeline locally in seconds:
     1. **Regression Check:** Runs all existing locked black-box tests in `tests/`.
     2. **1:1 & Seam Check (`go/ast`):** Verifies `specs/<feature>.md` and `tests/<feature>_test.go` exist as a pair and that `tests/` never couples to forbidden deep `internal/` packages.
     3. **Adversarial Teeth Check (Mutation Engine):** Automatically mutates AST nodes in the modified `internal/` code paths and runs `tests/<feature>_test.go` against each mutant. If the new test still passes when `internal/` is broken, the lock is rejected.
     4. **Human Ratification:** Presents `specs/<feature>.md` and the Teeth Score (`N/N mutants killed`) for approval.
     5. **Seal & Publish:** Seals the capsule into `.newci/crystal/` (setting `0444` permissions) and emits a single clean commit straight to GitHub `origin/main` so `go get` and `devcontainer-manager` stay in sync.

---

## 2. Avoiding the "Two Repos at Once" Trap (`speculate` vs. `newci`)

`speculate` required developing `speculate-kv` in parallel because it was built out of GitHub Actions YAML and PR webhooks that could only be tested by pushing to a remote GitHub repository.

Because `newci` is a local Go binary, we eliminate that friction using three strategies:

1. **Self-Hosting (Dogfooding `newci` on `newci`):**
   - `newci` is itself a Go project built with the Liquid-to-Crystal workflow.
   - As soon as Feature 1 (`vault-guard`) compiles in Liquid Mode, we use `newci` to lock `newci`'s own features into `/var/home/simon/projects/newci/specs/` and `/var/home/simon/projects/newci/tests/`.
2. **Ephemeral `t.TempDir()` Micro-Repos for Rejection Tests:**
   - Testing harness failure modes (e.g., rejecting a toothless test, catching an illegal `internal/` import, blocking edits to locked files) happens inside `tests/` using tiny 15-line Go modules generated in `t.TempDir()`.
   - Full adversarial verification runs locally in **<500ms** with zero network or GitHub round-trips.
3. **`dcrmux` as the First External Patient:**
   - Once bootstrapped on itself, `newci`'s first real-world target is [`../dcrmux`](../dcrmux/GOAL.md) (starting at 0 `.go` files for its first feature, `three-pane-layout`).

---

## 3. Bootstrap Sequence

1. **Write `GOAL.md` for `newci`:**
   - Define the CLI surface (`newci init`, `newci status`, `newci lock <feature>`, `newci unlock <feature>`, `newci rewind`) and seam rules.
2. **Feature 1 — `vault-guard`:**
   - `newci init` & `newci status`: initializes `.newci/`, enforces read-only (`0444`) permissions on locked `specs/` and `tests/`, and gates modifications behind `newci unlock <feature>`.
   - *Milestone:* Use `newci` to lock `vault-guard` on `newci` itself.
3. **Feature 2 — `seam-check`:**
   - AST verifier enforcing the 1:1 pairing between `specs/<feature>.md` and `tests/<feature>_test.go` and ensuring `tests/` only imports allowed public/harness seams.
   - *Milestone:* Lock `seam-check` using `newci lock seam-check`.
4. **Feature 3 — `teeth-check`:**
   - Automated Go AST mutation runner that mutates `internal/` implementation logic and proves `tests/<feature>_test.go` fails when `internal/` is broken.
   - *Milestone:* Lock `teeth-check` (proving the mutation engine catches mutations in itself).
5. **Feature 4 — `liquid-recorder` & `git-publish`:**
   - Background/on-build Liquid Frame checkpointing (`.newci/liquid/`) with `newci rewind`, plus one-way clean commit synthesis to GitHub `origin/main` whenever a feature locks.

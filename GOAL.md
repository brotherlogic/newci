# `newci` — Goal & Guardrails

`newci` is a native **Dual-Plane Version Control & Adversarial CI Engine** for Go projects built with the **Liquid-to-Crystal (Feature-Locking Ratchet)** workflow. It replaces day-to-day `git` branches, Pull Requests, and slow GitHub Actions pipelines with fast local ratification transactions, emitting clean commits to GitHub `origin/main` only when a feature crystallizes.

---

## 1. Repository Layout & CLI UX

A repository managed by `newci` separates the **Liquid Plane** (mutable implementation) from the **Crystal Plane** (immutable feature contracts):

```text
<repo>/
├── GOAL.md                         # Project constitution & candidate feature list
├── cmd/ & internal/                # LIQUID PLANE: 100% unlocked for rapid hacking & refactoring
├── specs/<feature>.md              # CRYSTAL PLANE: Locked behavioral spec (chmod 0444 when locked)
├── tests/<feature>_test.go         # CRYSTAL PLANE: Locked black-box test (chmod 0444 when locked)
└── .newci/
    ├── crystal/<feature>.json      # Sealed feature capsules (hashes, teeth score, timestamp)
    └── liquid/                     # Ephemeral content-addressed snapshots of cmd/ & internal/
```

### Core CLI Commands (`newci [--dir <path>] <command>`)

- **`newci init`:** Initializes `.newci/`, `specs/`, and `tests/`, and configures the repository for Dual-Plane tracking.
- **`newci status`:** Displays:
  - **Locked Features (Crystal):** Integrity status (`OK` vs. `TAMPERED`), permission check (`0444`), and recorded Teeth Score.
  - **Candidate / Unlocked Features:** Features present in `specs/` or `tests/` not yet sealed, or listed in `GOAL.md`.
  - **Liquid State:** Current working status of `cmd/` & `internal/` and latest Liquid Frame ID.
- **`newci lock <feature> [--yes] [--no-push]`:** Executes the atomic ratification transaction:
  1. **Vault Integrity & Regression:** Verifies no locked capsules were tampered with and runs all existing locked tests (`go test ./tests/...`).
  2. **1:1 & Seam Check:** Verifies `specs/<feature>.md` and `tests/<feature>_test.go` both exist, and uses `go/ast` to ensure `tests/<feature>_test.go` does not import forbidden `internal/` implementation packages.
  3. **Adversarial Teeth Check:** Mutates AST nodes in `internal/` (flipping boolean conditions, altering return values/branches) and runs `go test -run ... ./tests/` against each mutant. **Fails the lock if `tests/<feature>_test.go` passes when `internal/` is broken.**
  4. **Ratification & Seal:** Prompts for human sign-off on `specs/<feature>.md` + Teeth Score (unless `--yes`), writes `.newci/crystal/<feature>.json`, locks `specs/<feature>.md` and `tests/<feature>_test.go` to `0444`, and emits a single clean commit to Git `main` (pushing to `origin/main` unless `--no-push`).
- **`newci unlock <feature>`:** Explicitly opens a locked feature for intentional modification (restoring `0644` permissions on its spec/test and marking the capsule unlocked until re-locked).
- **`newci snapshot` / `newci rewind [frame]`:** Records a lightweight Liquid Frame of `cmd/` and `internal/` whenever `go build ./...` and locked tests pass, and rewinds `cmd/` + `internal/` to a prior frame without touching `specs/` or `tests/`.

---

## 2. Hard Guardrails (The Constitution)

- **Stack:** Pure Go (`github.com/brotherlogic/newci`) using the standard library (`go/ast`, `go/parser`, `go/printer`, `go/token`, `crypto/sha256`). Keep external dependencies minimal.
- **Target Isolation (`--dir <path>`):** Every operation must work against an arbitrary target directory (`--dir`, defaulting to `.`) rather than assuming the current process working directory. This allows `newci`'s own black-box tests in `tests/` to spin up 15-line Go micro-repos inside `t.TempDir()` and verify both passing and failing locks in milliseconds.
- **Public Black-Box Seam:** Black-box tests in `tests/<feature>_test.go` must exercise `newci` strictly through its public CLI entrypoint (or a thin public runner package) — never importing `github.com/brotherlogic/newci/internal/...`.
- **OS-Level & Cryptographic Immutability:** Locked files (`specs/<feature>.md` and `tests/<feature>_test.go`) must be set to `0444` on disk and verified against SHA-256 digests stored in `.newci/crystal/<feature>.json`. Any unauthorized modification or permission weakening is flagged immediately by `newci status` and blocks `newci lock`.
- **Self-Hosting Dogfooding:** `newci` manages itself. Every feature below is prototyped in Liquid Mode (`cmd/newci`, `internal/`) and ratified by running `newci lock <feature>` on `/var/home/simon/projects/newci`.

---

## 3. Candidate Features to Prototype & Lock

Build in Liquid mode on `main`, and lock each behavior into `specs/<feature>.md` + `tests/<feature>_test.go` once it works:

1. `vault-guard` — `newci init`, `newci status`, and `newci unlock <feature>`: manages `.newci/crystal/`, enforces `0444` read-only permissions + SHA-256 integrity checks on locked `specs/` and `tests/`, and reports Liquid vs. Crystal feature status.
2. `seam-check` — `go/ast` verifier enforcing 1:1 pairing between `specs/<feature>.md` and `tests/<feature>_test.go`, and rejecting any test file in `tests/` that imports `<module>/internal/...`.
3. `teeth-check` — Automated Go AST mutation engine that generates single-site mutants in `internal/` `.go` files and verifies `tests/<feature>_test.go` kills the mutants (fails when `internal/` is broken).
4. `crystal-lock` — End-to-end `newci lock <feature>` transaction uniting regression checks, `seam-check`, `teeth-check`, capsule sealing (`.newci/crystal/<feature>.json` + `0444`), and one-way Git commit/push emission.
5. `liquid-recorder` — `newci snapshot` and `newci rewind`: content-addressed checkpointing of `cmd/` and `internal/` in `.newci/liquid/` that restores implementation state without disturbing locked Crystal files.

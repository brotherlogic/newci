# Feature Spec: `vault-guard`

## 1. Overview
`vault-guard` manages the Crystal Vault (`.newci/crystal/`) and enforces OS-level (`0444` read-only permissions) and cryptographic (SHA-256 digest) immutability on locked feature specifications (`specs/<feature>.md`) and black-box tests (`tests/<feature>_test.go`).

## 2. CLI Commands & Trigger Conditions

All commands accept `--dir <path>` (defaulting to `.`) via the public `pkg/cli.Run` entrypoint.

### `newci [--dir <path>] init`
- **Action:** Idempotently creates `.newci/crystal/`, `.newci/liquid/`, `specs/`, and `tests/` (`0755`).
- **Output:** Prints `Initialized newci Dual-Plane workspace in <dir>` to stdout and exits `0`.

### `newci [--dir <path>] status`
- **Precondition:** Requires `.newci/crystal/` to exist; otherwise exits `1` with `repository at <dir> is not initialized (run 'newci init')`.
- **Crystal Vault Verification:**
  - For each `.newci/crystal/<feature>.json` where `unlocked == false`:
    - Verifies `specs/<feature>.md`, `tests/<feature>_test.go`, and `.newci/crystal/<feature>.json` exist and have no write permission bits (`perm & 0222 == 0`, i.e., `0444`).
    - Verifies the SHA-256 digests of `specs/<feature>.md` and `tests/<feature>_test.go` match `spec_sha256` and `test_sha256` in the capsule.
    - Reports `[OK]` if all checks pass, or `[TAMPERED]` with specific failure reasons (`missing <file>`, `<file> is writable`, `<file> hash mismatch`, or `corrupt capsule JSON`).
- **Candidate & Unlocked Discovery:**
  - Capsules with `unlocked == true` are listed as `[UNLOCKED]`.
  - Unsealed features discovered in `GOAL.md` (numbered backticked items `1. \`<feature>\``) or present in `specs/` or `tests/` are listed in `GOAL.md` order as `[CANDIDATE]` (neither spec nor test exists yet) or `[DRAFT]` (spec `[s]` and/or test `[t]` exists).
- **Liquid Plane Summary:**
  - Reports presence of `cmd/` and `internal/` directories and the latest frame in `.newci/liquid/`.
- **Exit Code:** Exits `0` if all locked capsules are `OK`; exits `1` if any locked capsule is `TAMPERED`.

### `newci [--dir <path>] lock <feature> [--yes] [--no-push]`
- **Preconditions:**
  - Repository must be initialized and have zero `TAMPERED` capsules in `status`.
  - Both `specs/<feature>.md` and `tests/<feature>_test.go` must exist; otherwise exits `1` with `missing spec file specs/<feature>.md` or `missing test file tests/<feature>_test.go`.
- **Action:**
  - Computes SHA-256 hashes of `specs/<feature>.md` and `tests/<feature>_test.go`.
  - Writes `.newci/crystal/<feature>.json` with `unlocked: false` and UTC timestamp.
  - Sets `chmod 0444` on `specs/<feature>.md`, `tests/<feature>_test.go`, and `.newci/crystal/<feature>.json`.
  - Exits `0`.

### `newci [--dir <path>] unlock <feature>`
- **Precondition:** `.newci/crystal/<feature>.json` must exist; otherwise exits `1` with `error: feature "<feature>" is not locked in crystal vault`.
- **Action:**
  - Restores `chmod 0644` on `.newci/crystal/<feature>.json`, `specs/<feature>.md`, and `tests/<feature>_test.go`.
  - Updates `.newci/crystal/<feature>.json` with `"unlocked": true`.
  - Exits `0`.

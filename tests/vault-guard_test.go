package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brotherlogic/newci/pkg/cli"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestVaultGuardLifecycle(t *testing.T) {
	repo := t.TempDir()

	// 1. Status before init should fail
	if code, _, stderr := runCLI(t, "-dir", repo, "status"); code == 0 || !strings.Contains(stderr, "not initialized") {
		t.Fatalf("expected uninitialized error, got code=%d stderr=%q", code, stderr)
	}

	// 2. Init creates Dual-Plane directories
	if code, stdout, stderr := runCLI(t, "-dir", repo, "init"); code != 0 {
		t.Fatalf("init failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, sub := range []string{".newci/crystal", ".newci/liquid", "specs", "tests"} {
		if info, err := os.Stat(filepath.Join(repo, sub)); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %s to exist after init", sub)
		}
	}

	// 3. Write GOAL.md and verify candidate ordering in status
	goalContent := `# Test Goal
## 3. Candidate Features
1. ` + "`zeta-feature`" + ` — First in GOAL
2. ` + "`alpha-feature`" + ` — Second in GOAL
`
	if err := os.WriteFile(filepath.Join(repo, "GOAL.md"), []byte(goalContent), 0644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runCLI(t, "-dir", repo, "status")
	if code != 0 {
		t.Fatalf("status failed: code=%d stderr=%q", code, stderr)
	}
	zetaIdx := strings.Index(stdout, "zeta-feature")
	alphaIdx := strings.Index(stdout, "alpha-feature")
	if zetaIdx == -1 || alphaIdx == -1 || zetaIdx > alphaIdx {
		t.Fatalf("expected GOAL.md candidate order (zeta before alpha) in status output:\n%s", stdout)
	}

	// 4. Unlocking a feature that isn't locked should fail cleanly
	code, _, stderr = runCLI(t, "-dir", repo, "unlock", "zeta-feature")
	if code == 0 || !strings.Contains(stderr, `feature "zeta-feature" is not locked in crystal vault`) {
		t.Fatalf("expected clean not-locked error on unlock, got code=%d stderr=%q", code, stderr)
	}

	// 5. Locking without spec/test files should fail cleanly
	code, _, stderr = runCLI(t, "-dir", repo, "lock", "zeta-feature")
	if code == 0 || !strings.Contains(stderr, "missing spec file specs/zeta-feature.md") {
		t.Fatalf("expected missing spec error, got code=%d stderr=%q", code, stderr)
	}

	specPath := filepath.Join(repo, "specs", "zeta-feature.md")
	testPath := filepath.Join(repo, "tests", "zeta-feature_test.go")
	capPath := filepath.Join(repo, ".newci", "crystal", "zeta-feature.json")

	if err := os.WriteFile(specPath, []byte("# Zeta Spec\nBehavior details.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runCLI(t, "-dir", repo, "lock", "zeta-feature")
	if code == 0 || !strings.Contains(stderr, "missing test file tests/zeta-feature_test.go") {
		t.Fatalf("expected missing test error, got code=%d stderr=%q", code, stderr)
	}

	if err := os.WriteFile(testPath, []byte("package tests\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 6. Lock feature seals capsule and sets 0444 permissions on spec, test, and capsule
	if code, _, stderr := runCLI(t, "-dir", repo, "lock", "zeta-feature"); code != 0 {
		t.Fatalf("lock failed: code=%d stderr=%q", code, stderr)
	}

	for _, p := range []string{specPath, testPath, capPath} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("expected %s to exist after lock: %v", p, err)
		}
		if info.Mode().Perm() != 0444 {
			t.Fatalf("expected %s to have 0444 permissions, got %04o", p, info.Mode().Perm())
		}
	}

	// 7. Status reports [OK] for locked feature
	code, stdout, stderr = runCLI(t, "-dir", repo, "status")
	if code != 0 || !strings.Contains(stdout, "[OK]") || !strings.Contains(stdout, "zeta-feature") {
		t.Fatalf("expected status [OK] for zeta-feature, got code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	// 8. Tampering with permissions or content causes status to report [TAMPERED] and exit 1
	if err := os.Chmod(specPath, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("# Tampered Spec\n"), 0644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ = runCLI(t, "-dir", repo, "status")
	if code == 0 || !strings.Contains(stdout, "[TAMPERED]") || !strings.Contains(stdout, "hash mismatch") {
		t.Fatalf("expected status to detect tampering and exit non-zero, got code=%d stdout=%q", code, stdout)
	}

	// Attempting to lock another feature while vault is tampered must be rejected
	if code, _, stderr := runCLI(t, "-dir", repo, "lock", "alpha-feature"); code == 0 || !strings.Contains(stderr, "TAMPERED") {
		t.Fatalf("expected lock to be blocked while vault is tampered, got code=%d stderr=%q", code, stderr)
	}

	// 9. Explicit unlock restores 0644 permissions and clears TAMPERED state into [UNLOCKED]
	if code, _, stderr := runCLI(t, "-dir", repo, "unlock", "zeta-feature"); code != 0 {
		t.Fatalf("unlock failed: code=%d stderr=%q", code, stderr)
	}
	for _, p := range []string{specPath, testPath, capPath} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0644 {
			t.Fatalf("expected %s to have 0644 permissions after unlock, got %04o", p, info.Mode().Perm())
		}
	}

	code, stdout, stderr = runCLI(t, "-dir", repo, "status")
	if code != 0 || !strings.Contains(stdout, "[UNLOCKED]") {
		t.Fatalf("expected status to exit 0 with [UNLOCKED] after unlock, got code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	// 10. Re-locking seals the updated spec cleanly
	if code, _, stderr := runCLI(t, "-dir", repo, "lock", "zeta-feature"); code != 0 {
		t.Fatalf("re-lock failed: code=%d stderr=%q", code, stderr)
	}
	code, stdout, _ = runCLI(t, "-dir", repo, "status")
	if code != 0 || !strings.Contains(stdout, "[OK]") {
		t.Fatalf("expected [OK] after re-lock, got code=%d stdout=%q", code, stdout)
	}
}

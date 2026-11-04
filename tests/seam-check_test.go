package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeamCheckEnforcesPairingAndForbidsInternalImports(t *testing.T) {
	repo := t.TempDir()

	if code, _, stderr := runCLI(t, "-dir", repo, "init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/sample\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(repo, "specs", "auth.md")
	testPath := filepath.Join(repo, "tests", "auth_test.go")

	// 1. Empty spec file is rejected
	if err := os.WriteFile(specPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testPath, []byte("package tests\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "-dir", repo, "seam-check", "auth"); code == 0 || !strings.Contains(stderr, "empty") {
		t.Fatalf("expected empty spec failure, got code=%d stderr=%q", code, stderr)
	}

	// 2. Spec with forbidden internal import in test is rejected by both seam-check and lock
	if err := os.WriteFile(specPath, []byte("# Auth Spec\n"), 0644); err != nil {
		t.Fatal(err)
	}
	badTest := `package tests

import (
	"testing"
	"example.com/sample/internal/secret"
)

func TestAuth(t *testing.T) {
	_ = secret.Token
}
`
	if err := os.WriteFile(testPath, []byte(badTest), 0644); err != nil {
		t.Fatal(err)
	}

	if code, _, stderr := runCLI(t, "-dir", repo, "seam-check", "auth"); code == 0 || !strings.Contains(stderr, "forbidden internal import") {
		t.Fatalf("expected forbidden internal import error from seam-check, got code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := runCLI(t, "-dir", repo, "lock", "auth"); code == 0 || !strings.Contains(stderr, "forbidden internal import") {
		t.Fatalf("expected lock to reject forbidden internal import, got code=%d stderr=%q", code, stderr)
	}

	// 3. Invalid Go syntax in test file is rejected
	if err := os.WriteFile(testPath, []byte("package tests\nfunc broken(\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "-dir", repo, "seam-check", "auth"); code == 0 || !strings.Contains(stderr, "invalid Go syntax") {
		t.Fatalf("expected invalid Go syntax error, got code=%d stderr=%q", code, stderr)
	}

	// 4. Clean public seam import passes seam-check
	goodTest := `package tests

import (
	"testing"
	"example.com/sample/pkg/api"
)

func TestAuth(t *testing.T) {
	_ = api.Client{}
}
`
	if err := os.WriteFile(testPath, []byte(goodTest), 0644); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := runCLI(t, "-dir", repo, "seam-check", "auth"); code != 0 || !strings.Contains(stdout, "Seam check OK") {
		t.Fatalf("expected seam-check to pass for public import, got code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	// 5. Orphan spec (missing matching test) is rejected by repository-wide seam-check and lock
	orphanSpec := filepath.Join(repo, "specs", "orphan.md")
	if err := os.WriteFile(orphanSpec, []byte("# Orphan\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "-dir", repo, "seam-check"); code == 0 || !strings.Contains(stderr, "no matching tests/orphan_test.go") {
		t.Fatalf("expected orphan spec failure, got code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := runCLI(t, "-dir", repo, "lock", "auth"); code == 0 || !strings.Contains(stderr, "no matching tests/orphan_test.go") {
		t.Fatalf("expected lock to fail when orphan spec exists, got code=%d stderr=%q", code, stderr)
	}
	_ = os.Remove(orphanSpec)

	// 6. Orphan test (missing matching spec) is rejected
	orphanTest := filepath.Join(repo, "tests", "unpaired_test.go")
	if err := os.WriteFile(orphanTest, []byte("package tests\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "-dir", repo, "seam-check"); code == 0 || !strings.Contains(stderr, "no matching specs/unpaired.md") {
		t.Fatalf("expected orphan test failure, got code=%d stderr=%q", code, stderr)
	}
	_ = os.Remove(orphanTest)

	// 7. Repository-wide seam-check and lock succeed once 1:1 pairing and public imports are clean
	if code, stdout, stderr := runCLI(t, "-dir", repo, "seam-check"); code != 0 || !strings.Contains(stdout, "1 feature pair(s) verified") {
		t.Fatalf("expected repo-wide seam-check to pass, got code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if code, _, stderr := runCLI(t, "-dir", repo, "lock", "auth"); code != 0 {
		t.Fatalf("expected lock to succeed on clean seam, got code=%d stderr=%q", code, stderr)
	}
}

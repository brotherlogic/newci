package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Capsule represents a sealed (or explicitly unlocked) feature in .newci/crystal/<feature>.json.
type Capsule struct {
	Feature       string    `json:"feature"`
	SpecSHA256    string    `json:"spec_sha256"`
	TestSHA256    string    `json:"test_sha256"`
	MutantsKilled int       `json:"mutants_killed"`
	MutantsTotal  int       `json:"mutants_total"`
	Unlocked      bool      `json:"unlocked"`
	LockedAt      time.Time `json:"locked_at"`
}

// FeatureStatus captures the live verification state of a locked capsule.
type FeatureStatus struct {
	Feature       string
	Status        string // "OK" or "TAMPERED"
	Reasons       []string
	Permissions   string
	MutantsKilled int
	MutantsTotal  int
}

// CandidateStatus captures an unlocked, draft, or GOAL.md candidate feature.
type CandidateStatus struct {
	Feature     string
	State       string // "UNLOCKED", "DRAFT", "CANDIDATE"
	HasSpec     bool
	HasTest     bool
	Description string
}

// RepoStatus summarizes the Dual-Plane state of a repository.
type RepoStatus struct {
	Dir          string
	Locked       []FeatureStatus
	Candidates   []CandidateStatus
	HasCmd       bool
	HasInternal  bool
	LatestFrame  string
	Tampered     bool
}

// Init creates the Dual-Plane directory structure (.newci/crystal, .newci/liquid, specs, tests).
func Init(dir string) error {
	dirs := []string{
		filepath.Join(dir, ".newci", "crystal"),
		filepath.Join(dir, ".newci", "liquid"),
		filepath.Join(dir, "specs"),
		filepath.Join(dir, "tests"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}
	return nil
}

// Seal writes a feature capsule into .newci/crystal/<feature>.json and locks the spec, test, and capsule to 0444.
func Seal(dir, feature string, mutantsKilled, mutantsTotal int) (*Capsule, error) {
	if err := ensureInitialized(dir); err != nil {
		return nil, err
	}
	specPath := SpecPath(dir, feature)
	testPath := TestPath(dir, feature)

	specHash, err := fileSHA256(specPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("missing spec file specs/%s.md", feature)
		}
		return nil, fmt.Errorf("hashing spec for %q: %w", feature, err)
	}
	testHash, err := fileSHA256(testPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("missing test file tests/%s_test.go", feature)
		}
		return nil, fmt.Errorf("hashing test for %q: %w", feature, err)
	}

	cap := &Capsule{
		Feature:       feature,
		SpecSHA256:    specHash,
		TestSHA256:    testHash,
		MutantsKilled: mutantsKilled,
		MutantsTotal:  mutantsTotal,
		Unlocked:      false,
		LockedAt:      time.Now().UTC(),
	}

	capPath := CapsulePath(dir, feature)
	if _, err := os.Stat(capPath); err == nil {
		_ = os.Chmod(capPath, 0644)
	}
	data, err := json.MarshalIndent(cap, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(capPath, append(data, '\n'), 0644); err != nil {
		return nil, fmt.Errorf("writing capsule %s: %w", capPath, err)
	}

	for _, p := range []string{specPath, testPath, capPath} {
		if err := os.Chmod(p, 0444); err != nil {
			return nil, fmt.Errorf("locking permissions on %s: %w", p, err)
		}
	}
	return cap, nil
}

// PublishGit stages all changes, creates a ratification commit ("Lock feature: <feature>"),
// and pushes to origin/main if a git repository and origin remote are present.
func PublishGit(dir, feature string, noPush bool) (committed bool, pushed bool, err error) {
	if !dirExists(filepath.Join(dir, ".git")) {
		return false, false, nil
	}

	runGit := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	if out, err := runGit("add", "-A"); err != nil {
		return false, false, fmt.Errorf("git add failed: %s: %w", out, err)
	}

	statusOut, err := runGit("status", "--porcelain")
	if err != nil {
		return false, false, fmt.Errorf("git status failed: %w", err)
	}
	if statusOut != "" {
		msg := fmt.Sprintf("Lock feature: %s", feature)
		if out, err := runGit("commit", "-m", msg); err != nil {
			return false, false, fmt.Errorf("git commit failed: %s: %w", out, err)
		}
		committed = true
	}

	if noPush {
		return committed, false, nil
	}

	remotes, err := runGit("remote")
	if err != nil || !strings.Contains(remotes, "origin") {
		return committed, false, nil
	}

	if out, err := runGit("push", "origin", "main"); err != nil {
		return committed, false, fmt.Errorf("git push origin main failed: %s: %w", out, err)
	}
	return committed, true, nil
}

// Unlock opens a sealed feature for intentional modification, restoring 0644 permissions.
func Unlock(dir, feature string) error {
	if err := ensureInitialized(dir); err != nil {
		return err
	}
	capPath := CapsulePath(dir, feature)
	cap, err := readCapsule(capPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("feature %q is not locked in crystal vault", feature)
		}
		return fmt.Errorf("feature %q is not locked in crystal vault: %w", feature, err)
	}

	specPath := SpecPath(dir, feature)
	testPath := TestPath(dir, feature)

	for _, p := range []string{capPath, specPath, testPath} {
		if _, statErr := os.Stat(p); statErr == nil {
			if err := os.Chmod(p, 0644); err != nil {
				return fmt.Errorf("restoring write permissions on %s: %w", p, err)
			}
		}
	}

	cap.Unlocked = true
	data, err := json.MarshalIndent(cap, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(capPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("updating capsule %s: %w", capPath, err)
	}
	return nil
}

// Status inspects the Crystal Vault, specs/, tests/, GOAL.md, and Liquid Plane.
func Status(dir string) (*RepoStatus, error) {
	if err := ensureInitialized(dir); err != nil {
		return nil, err
	}

	st := &RepoStatus{
		Dir:         dir,
		HasCmd:      dirExists(filepath.Join(dir, "cmd")),
		HasInternal: dirExists(filepath.Join(dir, "internal")),
		LatestFrame: latestLiquidFrame(dir),
	}

	crystalDir := filepath.Join(dir, ".newci", "crystal")
	entries, err := os.ReadDir(crystalDir)
	if err != nil {
		return nil, fmt.Errorf("reading crystal directory: %w", err)
	}

	seenFeatures := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		capPath := filepath.Join(crystalDir, entry.Name())
		cap, err := readCapsule(capPath)
		if err != nil {
			featName := strings.TrimSuffix(entry.Name(), ".json")
			st.Locked = append(st.Locked, FeatureStatus{
				Feature: featName,
				Status:  "TAMPERED",
				Reasons: []string{fmt.Sprintf("corrupt capsule JSON: %v", err)},
			})
			st.Tampered = true
			seenFeatures[featName] = true
			continue
		}

		seenFeatures[cap.Feature] = true
		specPath := SpecPath(dir, cap.Feature)
		testPath := TestPath(dir, cap.Feature)

		if cap.Unlocked {
			st.Candidates = append(st.Candidates, CandidateStatus{
				Feature: cap.Feature,
				State:   "UNLOCKED",
				HasSpec: fileExists(specPath),
				HasTest: fileExists(testPath),
			})
			continue
		}

		fs := verifyLockedFeature(cap, capPath, specPath, testPath)
		if fs.Status != "OK" {
			st.Tampered = true
		}
		st.Locked = append(st.Locked, fs)
	}

	// Discover draft features in specs/ or tests/ not yet in crystal
	draftSet := make(map[string]bool)
	if specEntries, err := os.ReadDir(filepath.Join(dir, "specs")); err == nil {
		for _, e := range specEntries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				feat := strings.TrimSuffix(e.Name(), ".md")
				if !seenFeatures[feat] {
					draftSet[feat] = true
				}
			}
		}
	}
	if testEntries, err := os.ReadDir(filepath.Join(dir, "tests")); err == nil {
		for _, e := range testEntries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.go") {
				feat := strings.TrimSuffix(e.Name(), "_test.go")
				if !seenFeatures[feat] {
					draftSet[feat] = true
				}
			}
		}
	}

	goalCandidates := parseGoalCandidates(filepath.Join(dir, "GOAL.md"))
	goalDesc := make(map[string]string)
	var orderedDrafts []string
	addedDraft := make(map[string]bool)

	for _, gc := range goalCandidates {
		goalDesc[gc.Feature] = gc.Description
		if !seenFeatures[gc.Feature] && !addedDraft[gc.Feature] {
			orderedDrafts = append(orderedDrafts, gc.Feature)
			addedDraft[gc.Feature] = true
		}
	}

	var extraDrafts []string
	for feat := range draftSet {
		if !addedDraft[feat] {
			extraDrafts = append(extraDrafts, feat)
		}
	}
	sort.Strings(extraDrafts)
	orderedDrafts = append(orderedDrafts, extraDrafts...)

	activeAssigned := len(st.Candidates) > 0 // true if an UNLOCKED capsule is already at the head
	for _, feat := range orderedDrafts {
		hasSpec := fileExists(SpecPath(dir, feat))
		hasTest := fileExists(TestPath(dir, feat))
		state := "CANDIDATE"
		if hasSpec || hasTest {
			state = "DRAFT"
			activeAssigned = true
		} else if !activeAssigned {
			state = "ACTIVE"
			activeAssigned = true
		}
		st.Candidates = append(st.Candidates, CandidateStatus{
			Feature:     feat,
			State:       state,
			HasSpec:     hasSpec,
			HasTest:     hasTest,
			Description: goalDesc[feat],
		})
	}

	sort.Slice(st.Locked, func(i, j int) bool {
		return st.Locked[i].Feature < st.Locked[j].Feature
	})

	return st, nil
}

func verifyLockedFeature(cap *Capsule, capPath, specPath, testPath string) FeatureStatus {
	fs := FeatureStatus{
		Feature:       cap.Feature,
		Status:        "OK",
		Permissions:   "0444",
		MutantsKilled: cap.MutantsKilled,
		MutantsTotal:  cap.MutantsTotal,
	}

	checkFile := func(label, path, expectedHash string) {
		info, err := os.Stat(path)
		if err != nil {
			fs.Status = "TAMPERED"
			fs.Reasons = append(fs.Reasons, fmt.Sprintf("missing %s", label))
			return
		}
		perm := info.Mode().Perm()
		if perm&0222 != 0 {
			fs.Status = "TAMPERED"
			fs.Permissions = fmt.Sprintf("%04o", perm)
			fs.Reasons = append(fs.Reasons, fmt.Sprintf("%s is writable (%04o != 0444)", label, perm))
		}
		if expectedHash != "" {
			actualHash, err := fileSHA256(path)
			if err != nil || actualHash != expectedHash {
				fs.Status = "TAMPERED"
				fs.Reasons = append(fs.Reasons, fmt.Sprintf("%s hash mismatch", label))
			}
		}
	}

	checkFile("spec", specPath, cap.SpecSHA256)
	checkFile("test", testPath, cap.TestSHA256)
	checkFile("capsule", capPath, "")

	return fs
}

var goalCandidateRe = regexp.MustCompile(`^\s*\d+\.\s+` + "`" + `([a-zA-Z0-9_-]+)` + "`" + `(?:\s*[—-]\s*(.+))?$`)

type goalCandidate struct {
	Feature     string
	Description string
}

func parseGoalCandidates(goalPath string) []goalCandidate {
	data, err := os.ReadFile(goalPath)
	if err != nil {
		return nil
	}
	var out []goalCandidate
	for _, line := range strings.Split(string(data), "\n") {
		if m := goalCandidateRe.FindStringSubmatch(line); m != nil {
			out = append(out, goalCandidate{
				Feature:     m[1],
				Description: strings.TrimSpace(m[2]),
			})
		}
	}
	return out
}

func latestLiquidFrame(dir string) string {
	entries, err := os.ReadDir(filepath.Join(dir, ".newci", "liquid"))
	if err != nil || len(entries) == 0 {
		return "none"
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	if len(names) == 0 {
		return "none"
	}
	sort.Strings(names)
	return names[len(names)-1]
}

// SpecPath returns the path to specs/<feature>.md.
func SpecPath(dir, feature string) string {
	return filepath.Join(dir, "specs", feature+".md")
}

// TestPath returns the path to tests/<feature>_test.go.
func TestPath(dir, feature string) string {
	return filepath.Join(dir, "tests", feature+"_test.go")
}

// CapsulePath returns the path to .newci/crystal/<feature>.json.
func CapsulePath(dir, feature string) string {
	return filepath.Join(dir, ".newci", "crystal", feature+".json")
}

func ensureInitialized(dir string) error {
	crystalDir := filepath.Join(dir, ".newci", "crystal")
	if !dirExists(crystalDir) {
		return fmt.Errorf("repository at %s is not initialized (run 'newci init')", dir)
	}
	return nil
}

func readCapsule(path string) (*Capsule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cap Capsule
	if err := json.Unmarshal(data, &cap); err != nil {
		return nil, err
	}
	return &cap, nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

package seam

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// CheckFeature verifies that a single feature has a 1:1 spec/test pair, valid Go syntax
// in tests/<feature>_test.go, and zero imports of internal implementation packages.
func CheckFeature(dir, feature string) error {
	specPath := filepath.Join(dir, "specs", feature+".md")
	testPath := filepath.Join(dir, "tests", feature+"_test.go")

	specInfo, err := os.Stat(specPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing spec file specs/%s.md", feature)
		}
		return err
	}
	if specInfo.Size() == 0 {
		return fmt.Errorf("seam violation: specs/%s.md is empty", feature)
	}

	if _, err := os.Stat(testPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing test file tests/%s_test.go", feature)
		}
		return err
	}

	fset := token.NewFileSet()
	fileAST, err := parser.ParseFile(fset, testPath, nil, parser.AllErrors)
	if err != nil {
		return fmt.Errorf("seam violation in tests/%s_test.go: invalid Go syntax: %w", feature, err)
	}

	modPath := readModulePath(dir)
	for _, imp := range fileAST.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if isForbiddenInternalImport(importPath, modPath) {
			return fmt.Errorf(
				"seam violation in tests/%s_test.go: forbidden internal import %q (tests/ must only import public seams)",
				feature, importPath,
			)
		}
	}

	return nil
}

// CheckAll verifies 1:1 pairing across all files in specs/ and tests/ and runs AST seam checks on each.
func CheckAll(dir string) ([]string, error) {
	specsDir := filepath.Join(dir, "specs")
	testsDir := filepath.Join(dir, "tests")

	specFeatures := make(map[string]bool)
	if entries, err := os.ReadDir(specsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !strings.HasSuffix(e.Name(), ".md") {
				return nil, fmt.Errorf("seam violation: unexpected non-markdown file in specs/: %s", e.Name())
			}
			feat := strings.TrimSuffix(e.Name(), ".md")
			specFeatures[feat] = true
		}
	}

	testFeatures := make(map[string]bool)
	if entries, err := os.ReadDir(testsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !strings.HasSuffix(e.Name(), "_test.go") {
				return nil, fmt.Errorf("seam violation: file in tests/ must match <feature>_test.go: %s", e.Name())
			}
			feat := strings.TrimSuffix(e.Name(), "_test.go")
			testFeatures[feat] = true
		}
	}

	allSet := make(map[string]bool)
	for f := range specFeatures {
		allSet[f] = true
	}
	for f := range testFeatures {
		allSet[f] = true
	}

	var allFeatures []string
	for f := range allSet {
		allFeatures = append(allFeatures, f)
	}
	sort.Strings(allFeatures)

	for _, feat := range allFeatures {
		if !specFeatures[feat] {
			return nil, fmt.Errorf("seam violation: tests/%s_test.go has no matching specs/%s.md", feat, feat)
		}
		if !testFeatures[feat] {
			return nil, fmt.Errorf("seam violation: specs/%s.md has no matching tests/%s_test.go", feat, feat)
		}
		if err := CheckFeature(dir, feat); err != nil {
			return nil, err
		}
	}

	return allFeatures, nil
}

func isForbiddenInternalImport(importPath, modPath string) bool {
	if modPath != "" {
		if importPath == modPath+"/internal" || strings.HasPrefix(importPath, modPath+"/internal/") {
			return true
		}
	}
	if importPath == "internal" ||
		strings.HasPrefix(importPath, "internal/") ||
		strings.HasPrefix(importPath, "./internal") ||
		strings.HasPrefix(importPath, "../internal") ||
		strings.HasSuffix(importPath, "/internal") ||
		strings.Contains(importPath, "/internal/") {
		return true
	}
	return false
}

func readModulePath(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

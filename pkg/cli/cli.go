package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/brotherlogic/newci/internal/seam"
	"github.com/brotherlogic/newci/internal/vault"
)

// Run executes the newci CLI with the provided arguments and output streams.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("newci", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", ".", "Target repository directory")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	rest := fs.Args()
	if len(rest) == 0 {
		printUsage(stderr)
		return 2
	}

	cmd := rest[0]
	cmdArgs := rest[1:]

	switch cmd {
	case "init":
		if err := vault.Init(*dir); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Initialized newci Dual-Plane workspace in %s\n", *dir)
		return 0

	case "status":
		st, err := vault.Status(*dir)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		renderStatus(stdout, st)
		if st.Tampered {
			return 1
		}
		return 0

	case "unlock":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(stderr, "error: usage: newci [--dir <path>] unlock <feature>")
			return 2
		}
		feature := cmdArgs[0]
		if err := vault.Unlock(*dir, feature); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Unlocked feature %q (specs/%s.md and tests/%s_test.go restored to 0644)\n", feature, feature, feature)
		return 0

	case "lock":
		lockFS := flag.NewFlagSet("lock", flag.ContinueOnError)
		lockFS.SetOutput(stderr)
		_ = lockFS.Bool("yes", false, "Skip interactive confirmation")
		noPush := lockFS.Bool("no-push", false, "Skip git push")

		var feature string
		var flagArgs []string
		for _, a := range cmdArgs {
			if strings.HasPrefix(a, "-") {
				flagArgs = append(flagArgs, a)
			} else if feature == "" {
				feature = a
			} else {
				flagArgs = append(flagArgs, a)
			}
		}
		if err := lockFS.Parse(flagArgs); err != nil {
			return 2
		}

		st, err := vault.Status(*dir)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		if st.Tampered {
			fmt.Fprintln(stderr, "error: crystal vault integrity check failed (TAMPERED capsules detected); fix or unlock before locking")
			return 1
		}

		if feature == "" {
			for _, c := range st.Candidates {
				if c.State == "DRAFT" || c.State == "UNLOCKED" || c.State == "ACTIVE" {
					feature = c.Feature
					break
				}
			}
		}
		if feature == "" {
			fmt.Fprintln(stderr, "error: usage: newci [--dir <path>] lock [feature] [--yes] [--no-push]")
			return 2
		}

		if err := seam.CheckFeature(*dir, feature); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		if _, err := seam.CheckAll(*dir); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}

		cap, err := vault.Seal(*dir, feature, 0, 0)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Locked feature %q into .newci/crystal/%s.json (0444)\n", cap.Feature, cap.Feature)

		committed, pushed, err := vault.PublishGit(*dir, cap.Feature, *noPush)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		if pushed {
			fmt.Fprintf(stdout, "Committed and pushed %q to origin/main\n", "Lock feature: "+cap.Feature)
		} else if committed {
			fmt.Fprintf(stdout, "Committed %q to local git branch\n", "Lock feature: "+cap.Feature)
		}

		if nextSt, err := vault.Status(*dir); err == nil {
			for _, c := range nextSt.Candidates {
				if c.State == "ACTIVE" || c.State == "DRAFT" {
					fmt.Fprintf(stdout, "Active feature is now: %s [%s]\n", c.Feature, c.State)
					break
				}
			}
		}
		return 0

	case "seam-check":
		if len(cmdArgs) > 0 {
			feature := cmdArgs[0]
			if err := seam.CheckFeature(*dir, feature); err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return 1
			}
			fmt.Fprintf(stdout, "Seam check OK for feature %q\n", feature)
			return 0
		}
		features, err := seam.CheckAll(*dir)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Seam check OK (%d feature pair(s) verified)\n", len(features))
		return 0

	default:
		fmt.Fprintf(stderr, "error: unknown command %q\n", cmd)
		printUsage(stderr)
		return 2
	}
}

func renderStatus(w io.Writer, st *vault.RepoStatus) {
	fmt.Fprintf(w, "=== Crystal Vault (Locked Features) ===\n")
	if len(st.Locked) == 0 {
		fmt.Fprintln(w, "  (none locked yet)")
	} else {
		for _, f := range st.Locked {
			if f.Status == "OK" {
				fmt.Fprintf(w, "  [OK]       %-20s perm=%s  teeth=%d/%d\n", f.Feature, f.Permissions, f.MutantsKilled, f.MutantsTotal)
			} else {
				fmt.Fprintf(w, "  [TAMPERED] %-20s perm=%s  (%s)\n", f.Feature, f.Permissions, strings.Join(f.Reasons, "; "))
			}
		}
	}

	fmt.Fprintf(w, "\n=== Candidate / Unlocked Features ===\n")
	if len(st.Candidates) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for _, c := range st.Candidates {
			specMark := " "
			if c.HasSpec {
				specMark = "s"
			}
			testMark := " "
			if c.HasTest {
				testMark = "t"
			}
			desc := ""
			if c.Description != "" {
				desc = " — " + c.Description
			}
			tag := fmt.Sprintf("[%s]", c.State)
			fmt.Fprintf(w, "  %-11s [%s%s] %s%s\n", tag, specMark, testMark, c.Feature, desc)
		}
	}

	fmt.Fprintf(w, "\n=== Liquid Plane ===\n")
	fmt.Fprintf(w, "  cmd/: %v | internal/: %v | latest frame: %s\n", st.HasCmd, st.HasInternal, st.LatestFrame)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: newci [--dir <path>] <init|status|lock|unlock> [args]")
}

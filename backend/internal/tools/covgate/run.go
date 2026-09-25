package covgate

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
)

// Run executes the gate CLI: reads the profile and ignore file from fsys and
// returns the process exit code.
func Run(args []string, fsys fs.FS, stdout io.Writer) int {
	fl := flag.NewFlagSet("coverage_gate", flag.ContinueOnError)
	fl.SetOutput(stdout)
	profile := fl.String("profile", "coverage.out", "coverage profile path")
	ignorePath := fl.String("ignore", ".coverageignore", "exclusion list path")
	module := fl.String("module", "github.com/labibtajremin/agrihub_bd/backend", "module path")
	threshold := fl.Float64("threshold", 100, "minimum per-package percentage")
	verbose := fl.Bool("v", false, "list uncovered blocks")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	pf, err := fsys.Open(*profile)
	if err != nil {
		fmt.Fprintln(stdout, "open profile:", err)
		return 2
	}
	defer pf.Close()
	blocks, err := ParseProfile(pf)
	if err != nil {
		fmt.Fprintln(stdout, "parse profile:", err)
		return 2
	}
	igf, err := fsys.Open(*ignorePath)
	if err != nil {
		fmt.Fprintln(stdout, "open ignore:", err)
		return 2
	}
	defer igf.Close()
	ignore, err := ParseIgnore(igf)
	if err != nil {
		fmt.Fprintln(stdout, "parse ignore:", err)
		return 2
	}
	failed := Check(stdout, Summarize(blocks, *module, ignore), *threshold)
	if len(failed) == 0 {
		return 0
	}
	if *verbose {
		for _, b := range UncoveredBlocks(blocks, *module, ignore) {
			fmt.Fprintln(stdout, "  uncovered", b)
		}
	}
	return 1
}

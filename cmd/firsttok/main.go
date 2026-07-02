// Package main is the firsttok CLI entry point.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/firsttok/firsttok/internal/config"
	"github.com/firsttok/firsttok/internal/report"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "firsttok",
		Short: "Measure time-to-first-token (TTFT) for LLM API providers and proxies",
	}
	root.AddCommand(newRunCmd(), newBenchCmd(), newCompareCmd(), newReportCmd())
	if err := root.Execute(); err != nil {
		// Exit-code-aware error handling.
		var ec *exitCodeError
		if errors.As(err, &ec) {
			os.Exit(ec.code)
		}
		if config.IsConfigError(err) {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// exitCodeError carries a process exit code from a subcommand.
type exitCodeError struct {
	code int
	msg  string
}

func (e *exitCodeError) Error() string { return e.msg }

func parseFormat(s string) (report.Format, error) {
	switch s {
	case "", "default", "snake":
		return report.FormatDefault, nil
	case "perftest":
		return report.FormatPerftest, nil
	}
	return report.FormatDefault, fmt.Errorf("unknown format %q (want default|perftest)", s)
}

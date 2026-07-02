package main

import (
	"fmt"

	"github.com/firsttok/firsttok/internal/report"
	"github.com/firsttok/firsttok/internal/runset"
	"github.com/spf13/cobra"
)

func newReportCmd() *cobra.Command {
	var dir, format string
	c := &cobra.Command{
		Use:   "report",
		Short: "Regenerate CSVs and report.txt from an existing result directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				return fmt.Errorf("report requires --dir")
			}
			f, err := parseFormat(format)
			if err != nil {
				return err
			}
			if err := runset.RunReport(dir, f); err != nil {
				return err
			}
			cmd.Printf("Report regenerated in %s\n", dir)
			return nil
		},
	}
	c.Flags().StringVar(&dir, "dir", "", "result directory containing ttft_*_round*.jsonl + metadata.jsonl")
	c.MarkFlagRequired("dir")
	c.Flags().StringVar(&format, "format", "default", "CSV field contract (default|perftest)")
	return c
}

var _ = report.FormatDefault

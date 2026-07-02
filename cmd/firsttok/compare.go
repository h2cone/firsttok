package main

import (
	"fmt"
	"path/filepath"

	"github.com/firsttok/firsttok/internal/config"
	"github.com/firsttok/firsttok/internal/report"
	"github.com/firsttok/firsttok/internal/runset"
	"github.com/spf13/cobra"
)

func newCompareCmd() *cobra.Command {
	s := &settingsFlags{}
	c := &cobra.Command{
		Use:   "compare [configs...]",
		Short: "Run multi-target TTFT comparison with delta reports",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("compare requires at least one config path or glob")
			}
			settings, err := s.toSettings()
			if err != nil {
				return err
			}
			paths := expandConfigs(args)
			if len(paths) < 2 {
				return fmt.Errorf("compare requires at least two configs (got %d)", len(paths))
			}
			used := map[string]bool{}
			var targets []runset.Target
			for _, p := range paths {
				cfg, err := config.Load(p, &config.CLIOverrides{NoValidateStream: s.noValidateStream})
				if err != nil {
					return err
				}
				base := config.EndpointKeyFromName(filepath.Base(p))
				key := config.UniqueKey(base, used)
				targets = append(targets, runset.Target{
					Target: report.Target{Key: key, Label: key, Config: p},
					Config: cfg,
				})
			}
			dir, err := runset.RunCompare(targets, settings)
			if err != nil {
				return mapRunsetError(err)
			}
			cmd.Printf("TTFT compare report written to %s\n", dir)
			return nil
		},
	}
	addSettingsFlags(c, s)
	return c
}

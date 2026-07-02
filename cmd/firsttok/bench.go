package main

import (
	"path/filepath"
	"sort"

	"github.com/firsttok/firsttok/internal/config"
	"github.com/firsttok/firsttok/internal/report"
	"github.com/firsttok/firsttok/internal/runset"
	"github.com/spf13/cobra"
)

func addSettingsFlags(c *cobra.Command, s *settingsFlags) {
	c.Flags().IntVar(&s.rounds, "rounds", runset.DefaultRounds, "number of rounds")
	c.Flags().IntVar(&s.warmup, "warmup", runset.DefaultWarmup, "warmup requests per round (excluded from stats)")
	c.Flags().IntVar(&s.repeat, "repeat", runset.DefaultRepeat, "measured requests per round")
	c.Flags().IntVarP(&s.timeoutSec, "timeout-sec", "t", runset.DefaultTimeoutSec, "per-request timeout in seconds")
	c.Flags().Float64Var(&s.pauseSeconds, "pause-seconds", 0, "pause between target invocations")
	c.Flags().BoolVar(&s.noValidateStream, "no-validate-stream", false, "skip stream=true validation")
	c.Flags().Int64Var(&s.seed, "seed", -1, "random seed for target order (-1 = system random)")
	c.Flags().BoolVar(&s.fixedOrder, "fixed-order", false, "do not randomize target order per round")
	c.Flags().StringVar(&s.format, "format", "default", "CSV field contract (default|perftest)")
	c.Flags().StringVar(&s.outputDir, "output-dir", "", "output directory (default ttft_runs/<timestamp>)")
	c.Flags().BoolVar(&s.stopOnFailure, "stop-on-failure", false, "stop on first non-zero invocation exit code")
	c.Flags().BoolVar(&s.failOnRunFailure, "fail-on-run-failure", false, "exit 2 if any non-warmup probe fails (CI-friendly)")
	c.Flags().BoolVar(&s.progress, "progress", false, "print per-target progress")
}

type settingsFlags struct {
	rounds, warmup, repeat, timeoutSec int
	pauseSeconds                       float64
	noValidateStream                   bool
	seed                               int64
	fixedOrder                         bool
	format, outputDir                  string
	stopOnFailure, failOnRunFailure    bool
	progress                           bool
}

func (s *settingsFlags) toSettings() (runset.Settings, error) {
	f, err := parseFormat(s.format)
	if err != nil {
		return runset.Settings{}, err
	}
	return runset.Settings{
		Rounds:           s.rounds,
		Warmup:           s.warmup,
		Repeat:           s.repeat,
		TimeoutSec:       s.timeoutSec,
		PauseSeconds:     s.pauseSeconds,
		FixedOrder:       s.fixedOrder,
		Seed:             s.seed,
		NoValidateStream: s.noValidateStream,
		Format:           f,
		OutputDir:        s.outputDir,
		OutputDirSet:     s.outputDir != "",
		FailOnRunFailure: s.failOnRunFailure,
		StopOnFailure:    s.stopOnFailure,
	}, nil
}

func newBenchCmd() *cobra.Command {
	var configPath string
	s := &settingsFlags{}
	c := &cobra.Command{
		Use:   "bench",
		Short: "Run multiple rounds against one config for stability assessment",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := s.toSettings()
			if err != nil {
				return err
			}
			cfg, err := config.Load(configPath, &config.CLIOverrides{NoValidateStream: s.noValidateStream, Insecure: false})
			if err != nil {
				return err
			}
			key := config.EndpointKeyFromName(filepath.Base(configPath))
			target := runset.Target{
				Target: report.Target{Key: key, Label: key, Config: configPath},
				Config: cfg,
			}
			dir, err := runset.RunBench(target, settings)
			if err != nil {
				return mapRunsetError(err)
			}
			cmd.Printf("TTFT bench report written to %s\n", dir)
			return nil
		},
	}
	c.Flags().StringVarP(&configPath, "config", "c", "", "config file path (required)")
	c.MarkFlagRequired("config")
	addSettingsFlags(c, s)
	return c
}

// expandConfigs resolves positional args (paths and globs) into a sorted,
// de-duplicated list of config file paths.
func expandConfigs(args []string) []string {
	var paths []string
	seen := map[string]bool{}
	for _, a := range args {
		var matches []string
		if hasGlob(a) {
			matches, _ = filepath.Glob(a)
		}
		if matches == nil {
			matches = []string{a}
		}
		for _, m := range matches {
			abs, err := filepath.Abs(m)
			if err != nil {
				abs = m
			}
			if !seen[abs] {
				seen[abs] = true
				paths = append(paths, m)
			}
		}
	}
	sort.Strings(paths)
	return paths
}

func hasGlob(s string) bool {
	for _, c := range s {
		switch c {
		case '*', '?', '[', '{':
			return true
		}
	}
	return false
}

// mapRunsetError translates runset errors into exit-code-aware errors.
func mapRunsetError(err error) error {
	if runset.IsFailOnRunError(err) {
		return &exitCodeError{code: 2, msg: err.Error()}
	}
	return err
}

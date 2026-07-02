package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/firsttok/firsttok/internal/config"
	"github.com/firsttok/firsttok/internal/probe"
	"github.com/firsttok/firsttok/internal/report"
	"github.com/firsttok/firsttok/internal/result"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	o := &config.CLIOverrides{}
	var (
		configPath      string
		timeoutSec      int
		repeat          int
		warmup          int
		outputJSONL     string
		format          string
		firstTokenPaths []string
	)
	c := &cobra.Command{
		Use:   "run",
		Short: "Run a single config for quick TTFT probing",
		RunE: func(cmd *cobra.Command, args []string) error {
			o.TimeoutSec = timeoutSec
			o.Repeat = repeat
			o.Warmup = warmup
			o.FirstTokenPaths = firstTokenPaths
			return runRun(configPath, o, repeat, warmup, timeoutSec, outputJSONL, format)
		},
	}
	c.Flags().StringVarP(&configPath, "config", "c", "", "config file path (required)")
	c.MarkFlagRequired("config")
	c.Flags().StringVar(&o.Provider, "provider", "", "credential provider key")
	c.Flags().StringVar(&o.API, "api", "", "API form (auto|openai-responses|openai-chat-completions|anthropic-messages|google-generative-ai)")
	c.Flags().StringVar(&o.BaseURL, "base-url", "", "base url")
	c.Flags().StringVar(&o.URL, "url", "", "full request url")
	c.Flags().StringVar(&o.Path, "path", "", "request path")
	c.Flags().StringVar(&o.APIKey, "api-key", "", "API key (plaintext)")
	c.Flags().StringVar(&o.APIKeyEnv, "api-key-env", "", "environment variable name holding the API key")
	c.Flags().StringArrayVar(&o.Headers, "header", nil, "extra header \"Name: Value\" (repeatable)")
	c.Flags().StringVar(&o.RequestFile, "request-file", "", "read request body from file")
	c.Flags().StringVar(&o.RequestJSON, "request-json", "", "request body JSON string")
	c.Flags().StringArrayVar(&firstTokenPaths, "first-token-json-paths", nil, "custom first-token JSON path (repeatable)")
	c.Flags().IntVar(&timeoutSec, "timeout-sec", 120, "per-request timeout in seconds")
	c.Flags().IntVarP(&timeoutSec, "timeout", "t", 120, "alias for --timeout-sec")
	c.Flags().IntVar(&repeat, "repeat", 1, "number of measured (non-warmup) requests")
	c.Flags().IntVar(&warmup, "warmup", 0, "number of warmup requests")
	c.Flags().BoolVar(&o.NoValidateStream, "no-validate-stream", false, "skip stream=true validation")
	c.Flags().BoolVar(&o.Insecure, "insecure", false, "skip TLS certificate verification")
	c.Flags().StringVar(&outputJSONL, "output-jsonl", "", "write raw results JSONL (with summary tail) to this path")
	c.Flags().StringVar(&format, "format", "table", "output format (table|json)")
	return c
}

func runRun(configPath string, o *config.CLIOverrides, repeat, warmup, timeoutSec int, outputJSONL, format string) error {
	cfg, err := config.Load(configPath, o)
	if err != nil {
		return err
	}
	if err := config.ValidateStream(cfg.API, cfg.Request, o.NoValidateStream); err != nil {
		return err
	}
	url, err := config.BuildURL(cfg)
	if err != nil {
		return err
	}
	opts := probe.Options{
		URL:         url,
		Headers:     cfg.Headers,
		Body:        cfg.BodyBytes(),
		API:         cfg.API,
		CustomPaths: cfg.FirstTokenPaths,
		VerifySSL:   cfg.VerifySSL,
		Timeout:     time.Duration(timeoutSec) * time.Second,
	}

	total := warmup + repeat
	runs := make([]result.Single, 0, total)
	for i := 1; i <= total; i++ {
		w := i <= warmup
		runs = append(runs, probe.Run(opts, i, w, cfg.Provider, cfg.API))
	}

	summary := report.ComputeSummary(runs)

	if outputJSONL != "" {
		if err := report.WriteJSONL(outputJSONL, runs, summary); err != nil {
			return err
		}
	}

	switch format {
	case "json":
		out := map[string]interface{}{"runs": runs, "summary": summary}
		data, _ := json.Marshal(out)
		fmt.Println(string(data))
	default:
		printRunTable(runs)
		fmt.Printf("\nsummary: runs=%d successful=%d failed=%d\n", summary.Runs, summary.SuccessfulRuns, summary.FailedRuns)
		if summary.TTFTMS != nil {
			fmt.Printf("ttft_ms: min=%.3f p50=%.3f avg=%.3f p95=%.3f max=%.3f\n",
				summary.TTFTMS.Min, summary.TTFTMS.P50, summary.TTFTMS.Avg, summary.TTFTMS.P95, summary.TTFTMS.Max)
		}
	}

	if summary.FailedRuns > 0 || summary.SuccessfulRuns != summary.Runs {
		return &exitCodeError{code: 2, msg: "one or more non-warmup probes failed"}
	}
	return nil
}

func printRunTable(runs []result.Single) {
	cols := []string{"run", "warmup", "ok", "status", "headers_ms", "ttfb_ms", "first_event_ms", "ttft_ms", "events_read", "bytes_read", "first_token", "error"}
	rows := make([][]string, 0, len(runs))
	for _, r := range runs {
		src := ""
		if r.FirstToken != nil {
			src = r.FirstToken.Source
		}
		status := ""
		if r.Status != 0 {
			status = fmt.Sprintf("%d", r.Status)
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", r.Run),
			fmt.Sprintf("%t", r.Warmup),
			fmt.Sprintf("%t", r.OK),
			status,
			msStr(r.HeadersMS), msStr(r.TTFBMS), msStr(r.FirstEventMS), msStr(r.TTFTMS),
			fmt.Sprintf("%d", r.EventsRead), fmt.Sprintf("%d", r.BytesRead),
			src, r.Error,
		})
	}
	fmt.Print(renderSimpleTable(cols, rows))
}

func msStr(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.3f", *v)
}

// renderSimpleTable renders a header + divider + rows table (2-space columns).
func renderSimpleTable(cols []string, rows [][]string) string {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	for _, r := range rows {
		for i := 0; i < len(cols) && i < len(r); i++ {
			if len(r[i]) > widths[i] {
				widths[i] = len(r[i])
			}
		}
	}
	var b strings.Builder
	for i, c := range cols {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(c + strings.Repeat(" ", widths[i]-len(c)))
	}
	b.WriteString("\n")
	for i := range cols {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(strings.Repeat("-", widths[i]))
	}
	b.WriteString("\n")
	for _, r := range rows {
		for i := range cols {
			if i > 0 {
				b.WriteString("  ")
			}
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			b.WriteString(cell + strings.Repeat(" ", widths[i]-len(cell)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

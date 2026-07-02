// Package report builds CSV/text reports and reads/writes the JSONL result
// files. It owns the firsttok snake_case field contract and the perftest
// PascalCase compatibility format.
package report

// Field order constants for the default firsttok snake_case CSV contract.

var AllRunsFields = []string{
	"endpoint", "label", "round", "run", "warmup", "ok", "status",
	"ttfb_ms", "headers_ms", "first_event_ms", "ttft_ms",
	"events_read", "bytes_read", "first_token_source", "first_token_preview",
	"provider", "api", "url", "started_at", "error", "source_file",
}

var SummaryFields = []string{
	"endpoint", "label", "round", "rounds_included", "runs", "successful_runs",
	"failed_runs", "failure_rate_pct", "ttft_min_ms", "ttft_p25_ms",
	"ttft_p50_ms", "ttft_p75_ms", "ttft_p90_ms", "ttft_p95_ms", "ttft_p99_ms",
	"ttft_max_ms", "ttft_avg_ms", "ttft_stddev_ms", "ttfb_p50_ms",
	"headers_p50_ms", "first_event_p50_ms", "events_read_p50", "bytes_read_p50",
}

var DeltaByRoundFields = []string{
	"round", "comparison", "meaning", "left_endpoint", "right_endpoint",
	"p50_delta_ms", "p90_delta_ms", "p95_delta_ms", "avg_delta_ms",
	"min_delta_ms", "max_delta_ms",
}

var DeltaSummaryFields = []string{
	"comparison", "meaning", "rounds_compared", "pooled_p50_delta_ms",
	"pooled_p95_delta_ms", "pooled_avg_delta_ms", "round_p50_delta_median_ms",
	"round_p50_delta_min_ms", "round_p50_delta_max_ms",
	"round_p95_delta_median_ms", "round_avg_delta_median_ms",
}

var FailuresFields = AllRunsFields

var ConfigProfileFields = []string{
	"endpoint", "label", "provider", "api", "base_url_host", "path", "model",
	"stream", "max_tokens", "request_sha256", "config_path",
}

var InvocationsFields = []string{
	"round", "endpoint", "label", "exit_code", "started_at", "ended_at",
	"duration_sec", "command", "jsonl_path", "probe_log_path",
}

// perftest PascalCase field orders (compatibility format).

var PerftestAllRunsFields = []string{
	"Endpoint", "Label", "Round", "Run", "Warmup", "Ok", "Status",
	"TtfbMs", "HeadersMs", "FirstEventMs", "TtftMs", "EventsRead", "BytesRead",
	"FirstTokenSource", "FirstTokenPreview", "Provider", "Url", "StartedAt",
	"Error", "SourceFile",
}

var PerftestSummaryFields = []string{
	"Endpoint", "Label", "Round", "RoundsIncluded", "Runs", "SuccessfulRuns",
	"FailedRuns", "FailureRatePct", "TtftMinMs", "TtftP25Ms", "TtftP50Ms",
	"TtftP75Ms", "TtftP90Ms", "TtftP95Ms", "TtftP99Ms", "TtftMaxMs",
	"TtftAvgMs", "TtftStdDevMs", "TtfbP50Ms", "HeadersP50Ms",
	"FirstEventP50Ms", "EventsReadP50", "BytesReadP50",
}

var PerftestDeltaByRoundFields = []string{
	"Round", "Comparison", "Meaning", "LeftEndpoint", "RightEndpoint",
	"P50DeltaMs", "P90DeltaMs", "P95DeltaMs", "AvgDeltaMs", "MinDeltaMs",
	"MaxDeltaMs",
}

var PerftestDeltaSummaryFields = []string{
	"Comparison", "Meaning", "RoundsCompared", "PooledP50DeltaMs",
	"PooledP95DeltaMs", "PooledAvgDeltaMs", "RoundP50DeltaMedianMs",
	"RoundP50DeltaMinMs", "RoundP50DeltaMaxMs", "RoundP95DeltaMedianMs",
	"RoundAvgDeltaMedianMs",
}

var PerftestConfigProfileFields = []string{
	"Endpoint", "Label", "Provider", "BaseUrlHost", "Path", "Model", "Stream",
	"MaxTokens", "RequestSha256", "ConfigPath",
}

var PerftestInvocationsFields = []string{
	"Round", "Endpoint", "Label", "ExitCode", "StartedAt", "EndedAt",
	"DurationSec", "Command", "JsonlPath", "StdoutPath",
}

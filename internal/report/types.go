package report

import (
	"github.com/firsttok/firsttok/internal/result"
)

// RunRecord is a single probe result enriched with aggregation context derived
// from the JSONL filename and target metadata.
type RunRecord struct {
	result.Single
	Endpoint   string
	Label      string
	Round      int // 0 means "no round" (single run command)
	HasRound   bool
	SourceFile string
}

// Target describes one compare/bench target.
type Target struct {
	Key    string
	Label  string
	Config string
	Chain  string
}

// Comparison defines a left-minus-right delta pairing.
type Comparison struct {
	Name    string
	Meaning string
	Left    string // endpoint key
	Right   string
}

// InvocationRow records one target's execution within a round.
type InvocationRow struct {
	Round        int
	Endpoint     string
	Label        string
	ExitCode     int
	StartedAt    string
	EndedAt      string
	DurationSec  *float64
	Command      string
	JSONLPath    string
	ProbeLogPath string
}

// ConfigProfileRow captures the static profile of one target.
type ConfigProfileRow struct {
	Endpoint      string
	Label         string
	Provider      string
	API           string
	BaseURLHost   string
	Path          string
	Model         string
	Stream        *bool
	MaxTokens     interface{}
	RequestSHA256 string
	ConfigPath    string
}

// Metadata is the single-line metadata.jsonl object.
type Metadata struct {
	GeneratedAt   string             `json:"generated_at"`
	OutputDir     string             `json:"output_dir"`
	AggregateOnly bool               `json:"aggregate_only"`
	Rounds        int                `json:"rounds"`
	Repeat        int                `json:"repeat"`
	Warmup        int                `json:"warmup"`
	TimeoutSec    int                `json:"timeout_sec"`
	PauseSeconds  float64            `json:"pause_seconds"`
	FixedOrder    bool               `json:"fixed_order"`
	Seed          int                `json:"seed"`
	TTFTScript    string             `json:"ttft_script"`
	Endpoints     []MetadataEndpoint `json:"endpoints,omitempty"`
	Target        *MetadataTarget    `json:"target,omitempty"` // bench single-target
}

// MetadataEndpoint is one entry in the compare metadata endpoints[] array.
type MetadataEndpoint struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Config string `json:"config"`
	Chain  string `json:"chain,omitempty"`
}

// MetadataTarget is the bench single-target metadata object.
type MetadataTarget struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Config string `json:"config"`
}

// IsCompare reports whether metadata describes a multi-endpoint compare run.
func (m *Metadata) IsCompare() bool {
	return len(m.Endpoints) > 1
}

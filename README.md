# firsttok

`firsttok` is a command-line tool for measuring **time-to-first-token (TTFT)** of
large-model API providers and relay/proxy stations. It speaks the raw wire
format of OpenAI Responses, OpenAI Chat Completions, Anthropic Messages, and
Google Generative AI directly — no vendor SDK — so the timing boundaries of
`headers_ms`, `ttfb_ms`, `first_event_ms`, and `ttft_ms` stay clean, testable,
and reproducible.

It provides a clear snake_case data contract by default and an explicit
`--format perftest` compatibility mode for PascalCase CSVs.

## Build

```bash
go build -o firsttok ./cmd/firsttok
```

## Commands

### `firsttok run`

Run a single config for quick probing.

```bash
firsttok run --config examples/ttft.openai-chat.example.json --warmup 1 --repeat 5
firsttok run -c config.json --output-jsonl out.jsonl --format json
```

Flags override config-file fields: `--provider`, `--api`, `--base-url`, `--url`,
`--path`, `--api-key`, `--api-key-env`, `--header` (repeatable),
`--request-file`, `--request-json`, `--first-token-json-paths` (repeatable),
`--timeout-sec` (alias `--timeout`), `--no-validate-stream`, `--insecure`.

Exit code: `0` success, `1` config/file error, `2` any non-warmup probe failed
or did not measure `ttft_ms`.

### `firsttok bench`

Multiple rounds against one config for stability assessment.

```bash
firsttok bench -c config.json --rounds 5 --warmup 2 --repeat 20
firsttok bench -c config.json --output-dir ./my-run --format perftest
```

Default output dir: `ttft_runs/<YYYYMMDD_HHMMSS>` (local time). Writes
`all_runs.csv`, `endpoint_summary.csv`, `round_summary.csv`, `failures.csv`,
`config_profile.csv`, `invocations.csv`, `metadata.jsonl`, `report.txt`, and one
`ttft_<endpoint>_round<N>.jsonl` per round.

### `firsttok compare`

Multi-target comparison with delta reports. Accepts multiple config paths and/or
globs.

```bash
firsttok compare ttft.claude.dmxapi.json ttft.claude.dcs.json ttft.claude.dcs-no-plugin-proxy.json
firsttok compare 'ttft.claude.*.json' --rounds 10 --seed 42
```

Adds `delta_by_round.csv` and `delta_summary.csv`. When the endpoint keys
`dmxapi`, `dcs`, and `dcs-no-plugin-proxy` are all present, the three legacy
business comparisons are generated in addition to the automatic pairwise
comparisons. A warning is printed if `request_sha256` differs across targets.

Per-round target order is randomized by default (use `--seed` for
reproducibility, `--fixed-order` to disable).

### `firsttok report`

Regenerate all CSVs and `report.txt` from an existing result directory. No
network requests are made — it reads the raw `ttft_*_round*.jsonl` files,
`metadata.jsonl`, and the config profile. The report type (bench vs compare) is
determined from `metadata.jsonl`.

```bash
firsttok report --dir ttft_runs/20260701_143022
firsttok report --dir ttft_runs/20260701_143022 --format perftest
```

## Configuration

JSON config with case-insensitive field aliases:

| Field | Aliases |
|---|---|
| `provider` | `PROVIDER` — credential namespace |
| `api` | `API` — `auto` (default) \| `openai-responses` \| `openai-chat-completions` \| `anthropic-messages` \| `google-generative-ai` |
| `base_url` | `BASE_URL` |
| `url` | `URL` — full URL, highest priority |
| `path` | `PATH`, `endpoint`, `ENDPOINT` |
| `api_key` | `API_KEY`, `token`, `TOKEN` — `env:NAME` reads from env |
| `api_key_env` | `API_KEY_ENV`, `key_env` |
| `headers` | `HEADERS` |
| `request` | `REQUEST`, `request_json`, `REQUEST_JSON`, `body`, `BODY` |
| `request_file` | `REQUEST_FILE` |
| `first_token_json_paths` | `FIRST_TOKEN_JSON_PATHS`, `token_paths` |
| `verify_ssl` | bool |

API-key resolution order: CLI `--api-key` → CLI `--api-key-env` / config
`api_key_env` → config `api_key` (with `env:` prefix support).

`provider` only names the credential namespace and the auth-provider key in
reports; token-extraction rules are driven by `api`. Auth headers are added
automatically by `api` form unless an equivalent header is already present:
OpenAI → `Authorization: Bearer`, Anthropic → `x-api-key`, Google →
`x-goog-api-key`.

See [`examples/`](examples/) for one config per API form.

## Measurement model

Per probe (clock = monotonic `time.Now`):

1. record `start`;
2. send request, on response headers record `headers_ms`;
3. read the body in raw chunks (transport `DisableCompression: true`, sends
   `Accept-Encoding: identity`) — first byte → `ttfb_ms`;
4. incrementally parse SSE/NDJSON — first completed non-empty event →
   `first_event_ms`;
5. extract the first non-empty token per the `api` rules → `ttft_ms`.

Warmup requests are kept in the raw JSONL but excluded from summary, delta, and
failure-rate stats. Each JSONL file ends with a `{"summary": {...}}` tail record;
readers skip it.

## Statistics

- Percentiles use linear interpolation: `rank = (n-1) * p`.
- Standard deviation is the sample stddev (n-1); `nil` for n ≤ 1.
- All millisecond values round to 3 decimals; `-0.0` normalizes to `0.0`.
- Empty values render as empty CSV cells.

## Output formats

- **default** (snake_case): minimal quoting, LF terminators, fixed field order —
  the firsttok contract.
- **perftest** (`--format perftest`): PascalCase field names/order, all fields
  double-quoted, LF terminators.

## Exit codes

- `0` — success.
- `1` — config, file, or scheduling error.
- `2` — `run`: a non-warmup probe failed; `bench`/`compare` with
  `--fail-on-run-failure`: any non-warmup probe failed (default `0` for compat).
- `bench`/`compare` with `--stop-on-failure`: stop and exit `1` on the first
  non-zero invocation.

## Development

```bash
go test ./...
go vet ./...
gofmt -l .
```

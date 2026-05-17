# tfplanctx

**tfplanctx** is a production-oriented Terraform plan compressor for coding agents. The CLI command is **`tpc`**. It converts either a saved binary Terraform plan or Terraform JSON plan output into a compact, deterministic, grepable representation that agents can inspect without burning context on raw plan noise.

## Quick install

```bash
go install github.com/piblokto/tfplanctx/cmd/tpc@latest
```

Make sure your Go bin directory is on `PATH`:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

Then use it directly:

```bash
terraform plan -out=tfplan
tpc tfplan
```

## Token reduction

Review mode is designed to be materially smaller than both Terraform JSON and the human-readable Terraform plan engineers usually paste into agents:

| Baseline | Observed review-mode reduction |
| --- | ---: |
| Terraform JSON plan | **92–95% fewer approximate tokens** |
| Human-readable Terraform plan | **55–62% fewer approximate tokens** |

Human-readable plans are already much smaller than Terraform JSON, so the second row is the harder comparison and the more practical one for day-to-day agent use. Results vary with plan shape, but the default output is optimized for repetitive real plans rather than only synthetic fixtures.

To measure your own plan:

```bash
tpc plan.json --benchmark --txt-plan plan.txt
```

The benchmark uses the deterministic heuristic `ceil(chars / 4)`, so it is best read as a stable comparison metric rather than a model-specific tokenizer count.

## What it does

- Reads a binary plan file created by `terraform plan -out=tfplan`
- Reads Terraform JSON plan output created by `terraform show -json tfplan`
- Reads Terraform JSON plan content from stdin with `-`
- Emits concise TFP2 resource-scoped output by default, plus legacy TFP1 line, JSONL, and Markdown formats
- Applies deterministic redaction, replacement markers, risk annotations, and budget-aware compression

## What it does not do

- It does **not** replace Terraform's own planning or apply workflow
- It does **not** parse human-readable `terraform plan` output
- It does **not** modify Terraform state
- It does **not** apply infrastructure changes
- It does **not** require Terraform to be installed when the input is already JSON

## Build from source

```bash
make build
```

This creates the CLI binary at `./tpc`.

## Basic usage

```bash
terraform plan -out=tfplan
tpc tfplan
```

### Example Terraform workflow

```bash
terraform plan -out=tfplan
tpc tfplan --summary
tpc tfplan --risk-only
tpc tfplan --budget 4000
```

### Stdin example

```bash
terraform show -json tfplan | tpc -
```

### Risk-only example

```bash
tpc tfplan --risk-only
```

### Budget example

```bash
tpc tfplan --budget 4000
```

## Output format

The default format is **TFP2 review mode**, a resource-scoped compact protocol optimized for plan review rather than raw JSON fidelity:

```text
TFP2 C=1 U=1 R=1 D=1 Q=0 OUT=1 RISK=2 DRIFT=0 OMITTED=1
OMIT|computed=1
COMPRESS|templates=1;dict_values=1
D|aws_db_instance.old|identifier="old-db"|risk=data_loss
R|aws_instance.app|ami="ami-old"->"ami-new"|acts=delete,create;repl=ami
C|aws_s3_bucket.logs|bucket="prod-logs-example"|
U|aws_security_group.web|ingress[0].cidr_blocks=["10.0.0.0/8"]->["0.0.0.0/0"]|risk=public_ingress
O|endpoint|"old"->"new"|
```

For the full grammar, invariants, and escaping rules, see [`docs/TFP.md`](docs/TFP.md).

The header reports changed-resource counts:

- `C`: creates
- `U`: updates
- `R`: replacements
- `D`: deletes
- `Q`: reads
- `OUT`: changed outputs
- `RISK`: changed resources with at least one deterministic risk flag
- `DRIFT`: resources reported in Terraform `resource_drift`

Simple TFP2 resource records are:

```text
<ACTION>|<ADDRESS>|<ATTRS>|<META>
```

TFP2 avoids repeating the resource address for every attribute. Creates show only after values, deletes show only before values, and updates/replacements use `before->after`.

Outputs have their own compact form and are counted separately in `OUT`:

```text
O|artifact_registry_repository|"old"->"new"|
O|created|+"value"|
O|deleted|-"value"|
O|computed_after|"old"->unknown|unk=value
O|secret_output|sensitive->sensitive|sens=value
```

Changed outputs are always represented in compact review/detail output; they are not inferred from resource changes.

Review mode uses stable short metadata keys to save tokens:

| Key | Meaning |
| --- | --- |
| `unk` | unknown after apply |
| `sens` | sensitive/redacted paths |
| `repl` | replacement paths |
| `why` | Terraform action reason |
| `def` | omitted empty/default paths |
| `comp` | omitted provider-computed fields |
| `summ` | values summarized in review mode; exact value is available in `--detail` |
| `omit` | omitted attribute count |
| `acts` | raw Terraform actions when useful |

`--detail` switches back to expanded resource records with long metadata names such as `unknown`, `sensitive`, `replace_path`, and `reason`.

If Terraform reports a changed resource with no material attribute details, TFP2 still emits a resource record, for example:

```text
C|module.foo.terraform_data.validation||type=terraform_data;unk=id;attrs=none;no_material_attrs=true
```

For homogeneous resources, review mode uses groups and address templates when they reduce output size:

```text
TPL|P1|module.a.google_project_iam_member.
REASON_CODES|no_config=delete_because_no_resource_config
G|G1|C|google_project_iam_member|n=3;cols=addr,member;common=project:"project-a",role:"roles/viewer";unk=etag,id;def=condition;omit=1
|$P1:create_0|"group:0@example.com"
|$P1:create_1|"group:1@example.com"
|$P1:create_2|"group:2@example.com"
```

Group rows still represent every changed resource address; common attributes and metadata are lifted into the group header. `REASON_CODES` shortens repeated Terraform action reasons without losing their original meaning.

When one scalar column varies across a homogeneous group, review mode can collapse it into a positional list group:

```text
GL|G1|C|google_project_iam_member|n=3;col=member;vals="group:a","group:b","group:c";refs=$P1:a,$P1:b,$P1:c;common=project:"project-a",role:"roles/viewer"
```

Repeated long scalar values may be lifted into a dictionary:

```text
VAL|V1|"group:backend-developers@example.com"
```

Records then reference `$V1` exactly; dictionary entries are emitted only when the cost model says they save output.

Review mode also has deterministic semantic lenses. The first lens targets IAM-like access-member resources when the resource shape is confidently `scope + principal + role`:

```text
L|IAM|I1|C|google_project_iam_member|n=3;proj="deepsearch-dev";mem=$V1;roles="roles/run.admin","roles/secretmanager.admin","roles/viewer";refs=$P1:a,$P1:b,$P1:c
```

The IAM lens still preserves exact addresses, action, type, unknowns, sensitive markers, reasons, replacement metadata, and risks. If the shape is not safe, rendering falls back to generic grouped or resource records.

For correlated create/delete sets, review mode may add an explicitly heuristic summary:

```text
MIGRATION?|type=google_project_iam_member;C=4;D=3;same_scope=project:"deepsearch-dev";common_roles="roles/viewer";confidence=structural
```

`MIGRATION?` is review assistance, not a Terraform fact, and never replaces the exact represented resources.

Low-signal drift is summarized instead of expanded:

```text
DRIFT|total=52;risk=0;summ=52;detail=0
DRIFT_GROUP|type=google_project_iam_member;count=43;fields=etag;class=provider_cache
```

Risk-relevant, identity, IAM, network, sensitive, unknown, or planned-change-related drift stays detailed. `META|...` summarizes checks, imports, generated config, and relevant attributes when present.

Delimiters in paths or values are percent-escaped so fields containing `|`, `;`, `=`, commas, newlines, or `->` round-trip safely. Address template references use `$P<n>:<suffix>` and resolve through preceding `TPL|P<n>|<prefix>` lines. Value references use `$V<n>` and resolve through preceding `VAL|V<n>|<value>` lines.

The legacy attribute-per-line protocol remains available with `--format line` and uses a `TFP1` header.

Actions are normalized as follows:

| Terraform action | tfplanctx action |
| --- | --- |
| `create` | `C` |
| `update` | `U` |
| `delete` | `D` |
| `delete,create` or `create,delete` | `R` |
| `read` | `Q` |
| output change | `O` |

`no-op` resources are ignored unless `--include-noop` is used together with `--summary`.

## CLI flags

| Flag | Purpose |
| --- | --- |
| `--format compact` | Default TFP2 resource-scoped protocol |
| `--detail` | Expanded TFP2 records instead of default review-mode compaction |
| `--format line` | Legacy TFP1 attribute-per-line protocol |
| `--format jsonl` | One compact JSON object per emitted record |
| `--format markdown` | Concise Markdown for PR comments and human review |
| `--summary` | Resource-only summary records without normal attribute detail |
| `--risk-only` | Emit only resources with risk annotations |
| `--resource <address>` | Emit only the exact Terraform resource address |
| `--type <resource_type>` | Emit only the exact Terraform resource type |
| `--budget <chars>` | Fit output to an approximate character budget |
| `--benchmark` | Print approximate token savings to stderr |
| `--txt-plan <path>` | With `--benchmark`, also compare against a human-readable Terraform plan baseline |
| `--include-read` | Compatibility flag; reads are represented in TFP2 as `Q` records |
| `--include-noop` | Include no-op resource addresses in summary mode |
| `--max-value-len <n>` | Default `160` |
| `--max-list-items <n>` | Default `20` |
| `--max-object-keys <n>` | Default `30` |
| `--unsafe-show-sensitive` | Allow Terraform-marked sensitive values to print |
| `--unsafe-disable-secret-heuristics` | Disable heuristic path-based secret redaction |
| `--no-color` | Compatibility flag; output is always colorless |
| `--detailed-exitcode` | Terraform-like exit codes |

## Safety and sensitive values

Sensitive values are hidden by default. Terraform-marked sensitive fields render as `sensitive`, and tfplanctx also applies conservative path-based redaction when a path contains terms such as `password`, `secret`, `token`, `api_key`, `private_key`, `client_secret`, `authorization`, `cookie`, `session`, or `credential`.

`--unsafe-show-sensitive` only disables Terraform-marked sensitive redaction. Heuristic secret-path redaction still remains active unless the additional explicit `--unsafe-disable-secret-heuristics` flag is also supplied.

Unknown post-apply values render as `unknown`. Oversized values are summarized deterministically, for example:

```text
long_string(len=420,sha256=1a2b3c4d5e6f)
list(len=42,sha256=1a2b3c4d5e6f)
object(len=912,sha256=1a2b3c4d5e6f,keys=a,b,c)
json(len=912,sha256=1a2b3c4d5e6f)
```

Review mode may intentionally summarize low-signal provider-computed fields, default-empty fields, low-signal drift, or long values. The header adds `OMITTED=<count>` and `OMIT|...` explains only lossy/discarded categories, for example `computed`, `default_empty`, `drift_low_signal`, `summarized`, or `budget`. Non-lossy transforms such as groups, templates, value dictionaries, and lenses are tracked separately in `COMPRESS|...`. If a displayed value is summarized, the row carries `summary=true;detail_required=true` or `summ=<path>;detail_required=true`; `--detail` emits exact long values for audit/debugging. Resource address/action presence, outputs, unknown summaries, sensitive summaries, replacement paths, reasons, and risk metadata are retained even when compression is active.

`--benchmark` prints a separate stderr line comparing compact review and compact detail modes:

```text
BENCH json_tokens=2210 json_chars=8837 review_tokens=284 review_chars=1134 detail_tokens=390 detail_chars=1558 review_vs_json_reduction=87.1% detail_vs_json_reduction=82.4% omitted=12 grouped_resources=20 groups=2 lens_resources=0 templates=2 dict_values=0 drift_summarized=8
```

By default, the baseline is the Terraform JSON input that tfplanctx parses. If you also have the human-readable Terraform plan, add `--txt-plan` to compare tfplanctx against both JSON and text:

```bash
tpc plan.json --benchmark --txt-plan plan.txt
```

```text
BENCH json_tokens=2210 json_chars=8837 txt_tokens=740 txt_chars=2958 review_tokens=284 review_chars=1134 detail_tokens=390 detail_chars=1558 review_vs_json_reduction=87.1% detail_vs_json_reduction=82.4% review_vs_txt_reduction=61.6% detail_vs_txt_reduction=47.3% omitted=12 grouped_resources=20 groups=2 lens_resources=0 templates=2 dict_values=0 drift_summarized=8
```

The `review_vs_txt_reduction` field answers the question "how much smaller is the default review output than the normal human-readable Terraform plan an engineer might otherwise paste into an agent.". While json comparison also exists, it shouldn't be seen as the primary indicator since json plans are significantly more verbose than human-readable plans.

For a fixture-wide token-efficiency table, run:

```bash
make bench
```

This compares each JSON fixture with both TFP2 review and TFP2 detail output. For text-plan comparisons, use `--txt-plan` with the exact matching human-readable plan rather than the abbreviated semantic fixture references.

For developer runtime benchmarks of the parse/render pipeline, run:

```bash
make perf
```

Those Go microbenchmarks live in `internal/benchmark/transform_benchmark_test.go` and are separate from the user-facing `--benchmark` report.

## Risk annotations

Initial deterministic risk rules include:

- `risk=public_ingress`
- `risk=data_loss`
- `risk=iam_wildcard`
- `risk=privileged_kubernetes`
- `risk=helm_release_changed`

These are conservative rule-based hints, not AI judgements.

## Agent usage examples

Start broad, then narrow:

```bash
tpc tfplan --summary
tpc tfplan --risk-only
tpc tfplan --budget 4000
tpc tfplan --resource aws_security_group.web
```

This keeps raw Terraform JSON as a fallback instead of the first thing an agent reads.

## Exit codes

Default behavior:

- `0`: success
- `1`: error

With `--detailed-exitcode`:

- `0`: no changes
- `1`: error
- `2`: changes present
- `3`: risky changes present

When risky changes are present, `3` takes precedence over `2`.

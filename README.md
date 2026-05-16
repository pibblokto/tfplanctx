# tfplanctx

tfplanctx is a production-oriented CLI that reduces Terraform plan token usage for coding agents. It converts either a saved binary Terraform plan or Terraform JSON plan output into a compact, deterministic, grepable representation that is easier for agents to inspect without spending context on raw plan noise.

## What it does

- Reads a binary plan file created by `terraform plan -out=tfplan`
- Reads Terraform JSON plan output created by `terraform show -json tfplan`
- Reads Terraform JSON plan content from stdin with `-`
- Emits concise line, JSONL, or Markdown output for changed attributes only
- Applies deterministic redaction, replacement markers, risk annotations, and budget-aware compression

## What it does not do

- It does **not** replace Terraform's own planning or apply workflow
- It does **not** parse human-readable `terraform plan` output
- It does **not** modify Terraform state
- It does **not** apply infrastructure changes
- It does **not** require Terraform to be installed when the input is already JSON

## Build

```bash
make build
```

This creates the CLI binary at `./tpc`.

## Basic usage

```bash
terraform plan -out=tfplan
tfplanctx tfplan
```

If you build from this repository directly, use the generated binary name:

```bash
./tpc tfplan
```

### Example Terraform workflow

```bash
terraform plan -out=tfplan
tfplanctx tfplan --summary
tfplanctx tfplan --risk-only
tfplanctx tfplan --format line --budget 4000
```

### Stdin example

```bash
terraform show -json tfplan | tfplanctx -
```

### Risk-only example

```bash
tfplanctx tfplan --risk-only
```

### Budget example

```bash
tfplanctx tfplan --budget 4000
```

## Output format

The default format is the compact line protocol:

```text
TFP1 C=2 U=1 R=1 D=0 OUT=1 RISK=1
U|aws_security_group.web|ingress[0].cidr_blocks|["10.0.0.0/8"]|["0.0.0.0/0"]|risk=public_ingress
C|aws_s3_bucket.logs|bucket|null|"prod-logs-example"|
R|aws_instance.app|ami|"ami-old"|"ami-new"|replace_path
D|aws_db_instance.old|self|exists|null|risk=data_loss
```

The header reports changed-resource counts:

- `C`: creates
- `U`: updates
- `R`: replacements
- `D`: deletes
- `OUT`: changed outputs
- `RISK`: changed resources with at least one risk flag

Each attribute record is:

```text
<ACTION>|<ADDRESS>|<PATH>|<BEFORE>|<AFTER>|<FLAGS>
```

Actions are normalized as follows:

| Terraform action | tfplanctx action |
| --- | --- |
| `create` | `C` |
| `update` | `U` |
| `delete` | `D` |
| `delete,create` or `create,delete` | `R` |
| output change | `O` |

`read` changes are ignored by default and included as update-style records only with `--include-read`. `no-op` resources are ignored unless `--include-noop` is used together with `--summary`.

## CLI flags

| Flag | Purpose |
| --- | --- |
| `--format line` | Default compact line protocol |
| `--format jsonl` | One compact JSON object per emitted record |
| `--format markdown` | Concise Markdown for PR comments and human review |
| `--summary` | One line per changed resource instead of one per changed attribute |
| `--risk-only` | Emit only resources with risk annotations |
| `--resource <address>` | Emit only the exact Terraform resource address |
| `--type <resource_type>` | Emit only the exact Terraform resource type |
| `--budget <chars>` | Fit output to an approximate character budget |
| `--benchmark` | Print approximate token savings to stderr |
| `--include-read` | Include read/data-source style changes |
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
list(len=42)
object(keys=55)
json(len=912,sha256=1a2b3c4d5e6f)
```

When `--budget` forces records to be dropped, the line-protocol header adds `OMITTED=<count>` so agents can see that compression changed the level of detail.

`--benchmark` prints a separate stderr line such as:

```text
BENCH approx_tokens_in=2210 approx_tokens_out=284 tokens_saved=1926 reduction=87.1% chars_in=8837 chars_out=1134
```

The benchmark uses the deterministic heuristic `ceil(chars / 4)` rather than a model-specific tokenizer, so it is useful for comparing transformations but should be treated as approximate.

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
tfplanctx tfplan --summary
tfplanctx tfplan --risk-only
tfplanctx tfplan --format line --budget 4000
tfplanctx tfplan --resource aws_security_group.web
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

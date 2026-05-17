---
name: terraform-plan-context
description: Use when analyzing Terraform plan files or reviewing Terraform infrastructure changes. Converts plans into compact agent-readable context with tfplanctx.
---

Prefer `tpc` before raw Terraform JSON or human plan output.

Workflow:

1. `tpc <planfile> --summary`
2. `tpc <planfile> --risk-only`
3. `tpc <planfile> --budget 4000`
4. `tpc <planfile> --resource <address>` for one resource
5. `tpc <planfile> --detail` only when review mode is insufficient

Read TFP2 like this:

- Header: `TFP2 C U R D Q OUT RISK DRIFT [OMITTED]`
- Actions: `C` create, `U` update, `R` replace, `D` delete, `Q` read, `O` output
- Resource row: `<ACTION>|<ADDRESS>|<ATTRS>|<META>`
- Create shows after values only; delete shows before values only; update/replace uses `before->after`
- Paths use dot notation and `[n]`; delimiters are percent-escaped
- Key metadata: `unk`, `sens`, `repl`, `why`, `risk`, `def`, `comp`, `summ`, `omit`
- `O|...` rows are output changes
- `G|...` plus `|...` rows, `GL|...`, and `L|IAM|...` are compressed resource forms; expand `refs`, inherited common fields, and metadata before reasoning
- `TPL|P...` defines address prefixes; `$P...` expands addresses
- `VAL|V...` defines repeated values; `$V...` expands exact values
- `REASON_CODES|...` expands short `why=` codes
- `OMIT|...` is lossy review summarization; `COMPRESS|...` is non-lossy factoring
- `DRIFT|...` / `DRIFT_GROUP|...` summarize low-signal drift; detailed drift remains explicit
- `MIGRATION?|...` is a structural hint, not Terraform proof

If `OMITTED`, `summ`, or `detail_required=true` matters to the task, use `--detail`.
Avoid raw `terraform show -json` unless `tpc` is insufficient.
Never apply Terraform changes unless the user explicitly asks.

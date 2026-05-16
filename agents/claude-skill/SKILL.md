---
name: terraform-plan-context
description: Use when analyzing Terraform plan files or reviewing Terraform infrastructure changes. Converts plans into compact agent-readable context with tfplanctx.
---

When reviewing Terraform changes, prefer tpc output before inspecting raw Terraform JSON or human plan output.

Recommended workflow:

1. If a saved plan exists, run `tpc <planfile> --summary` first.
2. Then run `tpc <planfile> --risk-only` for risky changes.
3. Then run `tpc <planfile> --budget 4000` for compact review context.
4. For a specific resource, run `tpc <planfile> --resource <address>`.
5. Use `tpc <planfile> --detail` only when review mode is not enough for troubleshooting.

How to read the compact format:

- `TFP2 C=... U=... R=... D=... Q=... OUT=... RISK=... DRIFT=...` is the header. It counts changed resources, reads, changed outputs, risky resources, and drifted resources.
- Action letters are semantic shortcuts: `C` create, `U` update, `R` replace, `D` delete, `Q` read, `O` output change.
- Simple resource lines are `<ACTION>|<ADDRESS>|<ATTRS>|<META>`, so one resource address is not repeated once per changed attribute.
- Creates omit implied `before=null`; deletes omit implied `after=null`; updates and replacements use `before->after`.
- Paths use dot notation for object keys and `[n]` for list positions, so `ingress[0].cidr_blocks` means the first ingress rule's CIDR list. Delimiter characters are percent-escaped when needed.
- Review mode abbreviates metadata: `unk` unknown, `sens` sensitive, `repl` replacement path, `why` action reason, `def` default-empty omission, `comp` provider-computed omission, `omit` omitted count.
- `G|...` group headers plus following `|...` rows compact homogeneous resources while still representing every changed address. Common values live in the group header; row columns hold only varying fields.
- `GL|...` is a list-compressed group used when one scalar column varies and positional `refs` + `vals` are smaller than row-per-resource output.
- `TPL|P1|...` defines address prefixes; `$P1:<suffix>` rows expand back to full resource addresses.
- `VAL|V1|...` defines repeated long scalar values; `$V1` references resolve back to those exact values.
- `L|IAM|...` is a deterministic IAM lens for access-member resources when tfplanctx can safely summarize `scope + principal + roles` while still preserving exact `refs`.
- `MIGRATION?|...` is a structural create/delete correlation hint, not a Terraform fact; use it for review orientation, not as proof of a move.
- `REASON_CODES|...` preserves verbose Terraform action reasons once and lets rows use short stable codes such as `no_config`.
- `DRIFT|...` and `DRIFT_GROUP|...` summarize low-signal provider/cache drift; risk-relevant drift remains detailed. `META|...` carries checks/import/generated-config summaries.
- `risk=...` flags are conservative deterministic hints, not model judgement. `OMITTED=<n>` and `OMIT|...` mean detail was summarized intentionally, not silently lost.

Avoid reading raw `terraform show -json` output unless tpc output is insufficient.
Never apply Terraform changes unless explicitly requested by the user.

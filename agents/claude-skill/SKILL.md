---
name: terraform-plan-context
description: Use when analyzing Terraform plan files or reviewing Terraform infrastructure changes. Converts plans into compact agent-readable context with tfplanctx.
---

When reviewing Terraform changes, prefer tfplanctx output before inspecting raw Terraform JSON or human plan output.

Recommended workflow:

1. If a saved plan exists, run `tfplanctx <planfile> --summary` first.
2. Then run `tfplanctx <planfile> --risk-only` for risky changes.
3. Then run `tfplanctx <planfile> --format line --budget 4000` for compact full context.
4. For a specific resource, run `tfplanctx <planfile> --resource <address>`.

How to read the compact format:

- `TFP1 C=... U=... R=... D=... OUT=... RISK=...` is the header. It counts changed resources, changed outputs, and resources with at least one deterministic risk flag.
- Action letters are semantic shortcuts: `C` create, `U` update, `R` replace, `D` delete, `O` output change.
- Attribute lines are `<ACTION>|<ADDRESS>|<PATH>|<BEFORE>|<AFTER>|<FLAGS>`.
- Paths use dot notation for object keys and `[n]` for list positions, so `ingress[0].cidr_blocks` means the first ingress rule's CIDR list.
- `unknown` means Terraform says the after value is only known after apply. `sensitive` means the raw value was intentionally redacted.
- `replace_path` marks an attribute Terraform identified as forcing replacement.
- `risk=...` flags are conservative deterministic hints, not model judgement. `OMITTED=<n>` in the header means budget compression dropped detail.

Avoid reading raw `terraform show -json` output unless tfplanctx output is insufficient.
Never apply Terraform changes unless explicitly requested by the user.

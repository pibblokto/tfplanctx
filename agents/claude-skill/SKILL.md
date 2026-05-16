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

Avoid reading raw `terraform show -json` output unless tfplanctx output is insufficient.
Never apply Terraform changes unless explicitly requested by the user.

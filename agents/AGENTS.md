# Agent instructions

When analyzing Terraform infrastructure changes, prefer `tfplanctx` output before reading raw Terraform JSON or human-readable Terraform plans.

Recommended workflow:

1. `tfplanctx <planfile> --summary`
2. `tfplanctx <planfile> --risk-only`
3. `tfplanctx <planfile> --format line --budget 4000`
4. `tfplanctx <planfile> --resource <address>` when a specific resource needs inspection

Use raw `terraform show -json` output only when the compressed context is insufficient.
Never apply Terraform changes unless the user explicitly requests it.

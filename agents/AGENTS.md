# Agent instructions

When analyzing Terraform infrastructure changes, prefer `tpc` output before reading raw Terraform JSON or human-readable Terraform plans.

Recommended workflow:

1. `tpc <planfile> --summary`
2. `tpc <planfile> --risk-only`
3. `tpc <planfile> --budget 4000`
4. `tpc <planfile> --resource <address>` when a specific resource needs inspection

Use `tpc <planfile> --detail` when compact review mode is insufficient for troubleshooting.
Use raw `terraform show -json` output only when the compressed context is insufficient.
Never apply Terraform changes unless the user explicitly requests it.

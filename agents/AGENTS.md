# Agent instructions

When analyzing Terraform infrastructure changes, prefer `tpc` output before reading raw Terraform JSON or human-readable Terraform plans.

Recommended workflow:

1. `tpc <planfile> --summary`
2. `tpc <planfile> --risk-only`
3. `tpc <planfile> --budget 4000`
4. `tpc <planfile> --resource <address>` when a specific resource needs inspection

Read TFP2 as resource-scoped records: `C/U/R/D/Q/O` are actions; create/delete imply the omitted null side; update/replace uses `before->after`; `unk`, `sens`, `repl`, `why`, and `risk` preserve important semantics. Group/lens/template/dictionary records are compressed forms, not missing resources.

Use `tpc <planfile> --detail` when compact review mode is insufficient for troubleshooting, especially when `OMITTED`, `summ`, or `detail_required=true` appears.
Use raw `terraform show -json` output only when the compressed context is insufficient.
Never apply Terraform changes unless the user explicitly requests it.

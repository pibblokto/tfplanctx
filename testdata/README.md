# Test fixtures

Each fixture family has two representations:

- `*.json`: Terraform JSON plan output that `tpc` is allowed to parse
- `*.tfplan.txt`: paired human-readable Terraform plan output kept as a semantic reference only

The text fixtures are intentionally **not** parsed by the tool. They exist so tests and reviewers can compare the compact representation with the kind of Terraform plan humans usually see.

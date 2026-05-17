# TFP: Terraform Plan Format

TFP is the compact, line-oriented interchange format emitted by `tpc`. It is designed for deterministic plan review by coding agents, not as a replacement for Terraform's own plan or apply semantics.

The current default format is **TFP2**. TFP1 is the older attribute-per-line format still available with `--format line`.

## Design goals

- Deterministic ordering and rendering
- Compact review-oriented output
- Safe handling of unknown and sensitive values
- Explicit accounting when review mode summarizes or omits detail
- Enough structure to verify that changed resources and outputs were not silently lost

## TFP2 document structure

A TFP2 document is UTF-8 text with one record per line:

```text
TFP2 <header fields>
[OMIT|...]
[COMPRESS|...]
[TPL|...]
[VAL|...]
[REASON_CODES|...]
[META|...]
[MIGRATION?|...]
[resource/group/lens/output records]
[DRIFT|...]
[DRIFT_GROUP|...]
[DRIFT_DETAIL|...]
```

The first line is always the header:

```text
TFP2 C=<n> U=<n> R=<n> D=<n> Q=<n> OUT=<n> RISK=<n> DRIFT=<n> [OMITTED=<n>]
```

Header fields:

| Field | Meaning |
| --- | --- |
| `C` | created managed resources |
| `U` | updated managed resources |
| `R` | replaced managed resources |
| `D` | deleted managed resources |
| `Q` | read/data-source style changes |
| `OUT` | changed outputs |
| `RISK` | changed resources with at least one deterministic risk flag |
| `DRIFT` | resources present in Terraform `resource_drift` |
| `OMITTED` | lossy review-mode omissions/summaries only |

`R` is one replacement in TFP2 even though Terraform's text plan may count the same resource as both one add and one destroy.

## Action codes

| Code | Meaning |
| --- | --- |
| `C` | create |
| `U` | update |
| `R` | replace |
| `D` | delete |
| `Q` | read |
| `O` | output change |

## Simple resource records

```text
<ACTION>|<ADDRESS>|<ATTRS>|<META>
```

Examples:

```text
C|aws_s3_bucket.logs|bucket="prod-logs"|
D|aws_db_instance.old|identifier="old-db"|risk=data_loss
U|aws_security_group.web|ingress[0].cidr_blocks=["10.0.0.0/8"]->["0.0.0.0/0"]|risk=public_ingress
R|aws_instance.app|ami="ami-old"->"ami-new"|acts=delete,create;repl=ami
```

Attribute encoding:

- create: `path=after`
- delete: `path=before`
- update/replace: `path=before->after`

The action implies omitted null sides: creates do not print `before=null`, and deletes do not print `after=null`.

If Terraform reports a changed resource but no material review attributes survive compression, the resource is still represented:

```text
C|module.foo.terraform_data.validation||type=terraform_data;unk=id;attrs=none;no_material_attrs=true
```

## Output records

Outputs use a dedicated compact form:

```text
O|<NAME>|<VALUE CHANGE>|<META>
```

Examples:

```text
O|artifact_registry_repository|"old"->"new"|
O|created|+"value"|
O|deleted|-"value"|
O|computed_after|"old"->unknown|unk=value
O|secret_output|sensitive->sensitive|sens=value
```

Changed outputs are counted in `OUT` and must always be represented.

## Metadata keys

Review mode uses short stable keys:

| Key | Meaning |
| --- | --- |
| `unk` | unknown-after paths |
| `sens` | sensitive/redacted paths |
| `repl` | replacement paths |
| `why` | Terraform action reason, possibly using `REASON_CODES` |
| `def` | omitted empty/default paths |
| `comp` | omitted provider-computed paths |
| `summ` | displayed values summarized in review mode |
| `omit` | omitted attribute count for the record |
| `acts` | raw Terraform action list when useful |
| `risk` | deterministic risk annotations |

`--detail` uses longer spellings such as `unknown`, `sensitive`, `replace_path`, and `reason`.

## Review-mode helper records

### Lossy omission accounting

```text
OMIT|computed=91;default_empty=96;drift_low_signal=9;summarized=1
```

The values in `OMIT|` are disjoint and sum to header `OMITTED`.

### Non-lossy compression accounting

```text
COMPRESS|grouped_common=4;groups=2;templates=12;dict_values=24;lens_resources=90
```

`COMPRESS|` records substitutions or inherited values that remain recoverable and therefore do not count toward `OMITTED`.

### Address templates

```text
TPL|P1|module.team.google_project_iam_member.roles
C|$P1:["roles/viewer-group:x@example.com"]|...
```

`$P<n>:<suffix>` expands to the corresponding template prefix plus suffix.

### Value dictionaries

```text
VAL|V1|"group:backend@example.com"
```

`$V<n>` expands to the exact value declared by the matching `VAL|` line.

### Groups

```text
G|G1|C|google_project_iam_member|n=3;cols=addr,member;common=project:"p",role:"roles/viewer"
|addr1|"group:a"
|addr2|"group:b"
|addr3|"group:c"
```

Group rows inherit common attributes and metadata from the group header. Every row still represents one resource address.

### Scalar list groups

```text
GL|G1|C|example_role|n=3;col=role;vals="a","b","c";refs=addr1,addr2,addr3;common=namespace:"shared"
```

`refs[i]` paired with `vals[i]` represents one resource.

### Deterministic semantic lenses

```text
L|IAM|I1|C|google_project_iam_member|n=3;proj="p";mem="group:x";roles="roles/a","roles/b","roles/c";refs=addr1,addr2,addr3
```

Lenses are deterministic, provider-field-based renderings. A lens may compress a resource family only when exact references and required metadata remain present.

### Reason codes

```text
REASON_CODES|no_config=delete_because_no_resource_config
```

Rows may then use `why=no_config`; the full Terraform reason remains recoverable from the legend.

### Drift

```text
DRIFT|total=52;risk=0;summ=52;detail=0
DRIFT_GROUP|type=google_project_iam_member;count=43;fields=etag;class=provider_cache
DRIFT_DETAIL|U|aws_security_group.web|ingress[0].cidr_blocks=...|risk=public_ingress
```

Low-signal drift may be summarized. Risk-relevant, identity, IAM, network, sensitive, or planned-change-related drift remains detailed.

## Value semantics

Rendered values are JSON-compatible where possible:

| Value | Meaning |
| --- | --- |
| `null` | known null |
| `unknown` | Terraform `after_unknown` marker |
| `sensitive` | redacted value |
| JSON string/number/bool/list/object | known value |

Review mode may summarize large values:

```text
long_string(len=420,sha256=...)
list(len=42,sha256=...)
object(len=912,sha256=...,keys=a,b,c)
json(len=912,sha256=...)
```

When that happens, the record marks `summary=true;detail_required=true` or `summ=<path>;detail_required=true`. Detail mode emits the exact value where policy allows it.

## Escaping

TFP2 is delimiter-safe. Within protocol fields, the renderer percent-escapes:

- `%`
- `|`
- `;`
- `=`
- newlines
- carriage returns
- the token `->`

Comma-separated metadata lists additionally escape commas. Consumers must decode escapes before comparing semantic values.

## Required invariants

A valid TFP2 review document must preserve:

1. Every non-no-op Terraform `resource_changes` entry as a resource row, group row, lens reference, or explicit represented summary.
2. Every changed output as one `O|...` record.
3. Header counts matching the expanded representation.
4. Replacement paths, action reasons, sensitive paths, unknown paths, deterministic risks, and drift accounting.
5. Exact or explicitly summarized value semantics.
6. `OMITTED` equal to the sum of lossy `OMIT|` categories.

The verifier in `internal/verify` checks these invariants against the normalized Terraform JSON model.

## TFP1 compatibility

TFP1 remains available through `--format line`:

```text
TFP1 C=<n> U=<n> R=<n> D=<n> Q=<n> OUT=<n> RISK=<n>
<ACTION>|<ADDRESS>|<PATH>|<BEFORE>|<AFTER>|<FLAGS>
```

TFP1 is simpler but less compact because it repeats the resource address for every changed attribute.

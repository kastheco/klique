# yq Reference

Binary: `yq`
Version: 3.4.x (kislyuk/yq — Python-based jq wrapper)

YAML/JSON processor. Wraps `jq` to add YAML support — transcodes YAML to JSON, applies jq filters, and optionally transcodes back. Uses standard jq filter syntax.

**Important:** This is the Python `yq` (kislyuk/yq), not the Go `yq` (mikefarah/yq). Syntax differs significantly between the two.

## Basic Usage

```bash
yq 'FILTER' file.yaml                 # output as JSON (default)
yq -y 'FILTER' file.yaml              # output as YAML
yq -Y 'FILTER' file.yaml              # YAML roundtrip (preserves tags/styles)
yq -i -y 'FILTER' file.yaml           # in-place YAML edit
```

## Key Flags

| Flag | Effect |
|------|--------|
| `-y`, `--yaml-output` | Output as YAML instead of JSON |
| `-Y`, `--yaml-roundtrip` | YAML roundtrip preserving tags and styles |
| `-i`, `--in-place` | Edit file in place |
| `-r` | Raw string output (from jq — no quotes) |
| `-e` | Exit with error on `false` or `null` |

## Common Operations

### Read a field
```bash
yq '.metadata.name' file.yaml
yq -r '.metadata.name' file.yaml      # raw string (no quotes)
```

### Read nested field
```bash
yq '.spec.containers[0].image' pod.yaml
```

### Modify a field (YAML output)
```bash
yq -y '.version = "2.0.0"' file.yaml
```

### Add a new field
```bash
yq -y '.metadata.labels.env = "production"' file.yaml
```

### Delete a field
```bash
yq -y 'del(.metadata.annotations)' file.yaml
```

### Iterate array elements
```bash
yq '.items[].name' file.yaml
yq '.items[] | select(.status == "active")' file.yaml
```

### Merge objects
```bash
yq -y '. * {"newKey": "newValue"}' file.yaml
```

### Convert YAML to JSON
```bash
yq '.' file.yaml                       # YAML in, JSON out
```

### Convert JSON to YAML
```bash
yq -y '.' file.json                    # JSON in, YAML out
```

### Multiple files
```bash
yq -y '.name' file1.yaml file2.yaml
```

### In-place edit
```bash
yq -i -y '.version = "3.0"' config.yaml
```

## jq Filter Cheat Sheet

Since yq uses jq syntax, common jq patterns apply:

```bash
.                          # identity (whole document)
.key                       # access field
.key.nested                # nested field
.[0]                       # array index
.[]                        # iterate array
.[] | select(.x == "y")   # filter
keys                       # object keys
length                     # count
map(. + 1)                 # transform array elements
to_entries                 # {k:v} -> [{key:k, value:v}]
del(.key)                  # remove field
. + {"new": "field"}       # add field
if .x then .y else .z end # conditional
```

## When to Use yq vs Alternatives

- **yq**: Reading/modifying YAML, JSON config files with structural awareness
- **jq**: JSON-only operations (yq delegates to jq internally)
- **sd/sed**: Only for trivial string replacements where structure doesn't matter
- **manual edit**: When the change is too complex for a filter expression

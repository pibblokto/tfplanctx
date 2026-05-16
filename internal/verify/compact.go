package verify

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/piblokto/tfplanctx/internal/codec"
	"github.com/piblokto/tfplanctx/internal/plan"
)

type compactRecord struct {
	Action plan.Action
	Addr   string
	Attrs  []string
	Meta   map[string]string
}

type compactOutput struct {
	Header              map[string]int
	Resources           map[string]compactRecord
	Outputs             map[string]compactRecord
	Omit                map[string]int
	Compress            map[string]int
	DriftTotal          int
	DriftSumm           int
	DriftDetail         int
	DriftGroups         int
	DriftGroupResources int
	DriftDetailRecords  int
	Templates           map[string]string
	Values              map[string]string
	ReasonLegend        map[string]string
	Groups              int
	Lenses              int
}

type parsedGroup struct {
	Action      plan.Action
	CommonAttrs []string
	Columns     []string
	Meta        map[string]string
}

// Compact checks that rendered TFP2 output still represents normalized semantics.
func Compact(p *plan.Plan, output string) error {
	parsed, err := parseCompact(output)
	if err != nil {
		return err
	}
	wantHeader := map[string]int{
		"C": p.Summary.Creates, "U": p.Summary.Updates, "R": p.Summary.Replaces, "D": p.Summary.Deletes,
		"Q": p.Summary.Reads, "OUT": p.Summary.OutputChanges, "RISK": p.Summary.RiskResources, "DRIFT": p.DriftSummary.Total,
	}
	for key, want := range wantHeader {
		if got := parsed.Header[key]; got != want {
			return fmt.Errorf("header %s=%d, want %d", key, got, want)
		}
	}
	if len(parsed.Resources) != len(p.Resources) {
		return fmt.Errorf("resource record count=%d, want %d", len(parsed.Resources), len(p.Resources))
	}
	for _, resource := range p.Resources {
		record, ok := parsed.Resources[resourceRecordKey(resource)]
		if !ok {
			return fmt.Errorf("missing resource %s", resource.Address)
		}
		if record.Action != resource.Action {
			return fmt.Errorf("resource %s action=%s, want %s", resource.Address, record.Action, resource.Action)
		}
		if err := requireMetadata(record, resource); err != nil {
			return fmt.Errorf("resource %s: %w", resource.Address, err)
		}
		if len(record.Attrs) < len(resource.Attributes) && record.Meta["omitted_attrs"] == "" && record.Meta["attrs"] != "none" {
			return fmt.Errorf("resource %s omitted details without marker", resource.Address)
		}
		if err := requireMaterialFields(record, resource); err != nil {
			return fmt.Errorf("resource %s: %w", resource.Address, err)
		}
	}
	if len(parsed.Outputs) != len(p.Outputs) {
		return fmt.Errorf("output record count=%d, want %d", len(parsed.Outputs), len(p.Outputs))
	}
	for _, output := range p.Outputs {
		record, ok := parsed.Outputs[output.Address]
		if !ok {
			return fmt.Errorf("missing output %s", output.Address)
		}
		if record.Action != plan.ActionOutput {
			return fmt.Errorf("output %s action=%s, want %s", output.Address, record.Action, plan.ActionOutput)
		}
		if len(output.UnknownPaths) > 0 && !sameCSV(record.Meta["unknown"], output.UnknownPaths) {
			return fmt.Errorf("output %s unknown=%q, want %v", output.Address, record.Meta["unknown"], output.UnknownPaths)
		}
		if len(output.SensitivePaths) > 0 && !sameCSV(record.Meta["sensitive"], output.SensitivePaths) {
			return fmt.Errorf("output %s sensitive=%q, want %v", output.Address, record.Meta["sensitive"], output.SensitivePaths)
		}
		if err := requireOutputValue(record, output); err != nil {
			return fmt.Errorf("output %s: %w", output.Address, err)
		}
	}
	if p.DriftSummary.Total > 0 {
		if parsed.DriftTotal != p.DriftSummary.Total {
			return fmt.Errorf("drift total=%d, want %d", parsed.DriftTotal, p.DriftSummary.Total)
		}
		if parsed.DriftSumm+parsed.DriftDetail != parsed.DriftTotal {
			return fmt.Errorf("drift accounting summ=%d detail=%d total=%d", parsed.DriftSumm, parsed.DriftDetail, parsed.DriftTotal)
		}
		if parsed.DriftGroupResources != parsed.DriftSumm {
			return fmt.Errorf("drift grouped resources=%d, want summarized=%d", parsed.DriftGroupResources, parsed.DriftSumm)
		}
		if parsed.DriftDetailRecords != parsed.DriftDetail {
			return fmt.Errorf("drift detail records=%d, want detail=%d", parsed.DriftDetailRecords, parsed.DriftDetail)
		}
	}
	omitted := sumCounts(parsed.Omit)
	if parsed.Header["OMITTED"] != omitted {
		return fmt.Errorf("omitted header=%d, categories=%d", parsed.Header["OMITTED"], omitted)
	}
	if parsed.Header["OMITTED"] > 0 && len(parsed.Omit) == 0 {
		return fmt.Errorf("header reports omitted details without OMIT summary")
	}
	return nil
}

func requireMetadata(record compactRecord, resource plan.ResourceChange) error {
	if resource.Action == plan.ActionReplace || resource.Action == plan.ActionRead {
		if !sameCSV(record.Meta["actions"], resource.RawActions) {
			return fmt.Errorf("actions=%q, want %v", record.Meta["actions"], resource.RawActions)
		}
	}
	if resource.ActionReason != "" && decodedMeta(record.Meta["reason"]) != resource.ActionReason {
		return fmt.Errorf("reason=%q, want %q", decodedMeta(record.Meta["reason"]), resource.ActionReason)
	}
	if len(resource.ReplacePaths) > 0 && !sameCSV(record.Meta["replace_path"], resource.ReplacePaths) {
		return fmt.Errorf("replace_path=%q, want %v", record.Meta["replace_path"], resource.ReplacePaths)
	}
	if len(resource.UnknownPaths) > 0 && !sameCSV(record.Meta["unknown"], resource.UnknownPaths) {
		return fmt.Errorf("unknown=%q, want %v", record.Meta["unknown"], resource.UnknownPaths)
	}
	if len(resource.SensitivePaths) > 0 && !sameCSV(record.Meta["sensitive"], resource.SensitivePaths) {
		return fmt.Errorf("sensitive=%q, want %v", record.Meta["sensitive"], resource.SensitivePaths)
	}
	if len(resource.Risks) > 0 && !sameCSV(record.Meta["risk"], resource.RiskNames()) {
		return fmt.Errorf("risk=%q, want %v", record.Meta["risk"], resource.RiskNames())
	}
	if resource.PreviousAddress != "" && decodedMeta(record.Meta["previous"]) != resource.PreviousAddress {
		return fmt.Errorf("previous=%q, want %q", decodedMeta(record.Meta["previous"]), resource.PreviousAddress)
	}
	if resource.Deposed != "" && decodedMeta(record.Meta["deposed"]) != resource.Deposed {
		return fmt.Errorf("deposed=%q, want %q", decodedMeta(record.Meta["deposed"]), resource.Deposed)
	}
	if resource.Importing && record.Meta["import"] != "true" {
		return fmt.Errorf("import=%q, want true", record.Meta["import"])
	}
	if resource.GeneratedConfig && record.Meta["generated_config"] != "true" {
		return fmt.Errorf("generated_config=%q, want true", record.Meta["generated_config"])
	}
	return nil
}

func requireMaterialFields(record compactRecord, resource plan.ResourceChange) error {
	represented := representedPaths(record.Attrs)
	for _, path := range resource.ReplacePaths {
		if _, ok := represented[path]; !ok {
			return fmt.Errorf("replace_path %q missing value detail", path)
		}
	}
	if resource.Action != plan.ActionUpdate && resource.Action != plan.ActionReplace {
		return nil
	}
	summarized, err := decodeCSV(record.Meta["summarized"])
	if err != nil {
		return err
	}
	summarySet := toSet(summarized)
	computed, err := decodeCSV(record.Meta["computed"])
	if err != nil {
		return err
	}
	computedSet := toSet(computed)
	for _, attribute := range resource.Attributes {
		if _, ok := represented[attribute.Path]; ok {
			continue
		}
		if _, ok := summarySet[attribute.Path]; ok {
			continue
		}
		if _, ok := computedSet[attribute.Path]; ok {
			continue
		}
		return fmt.Errorf("material %s field %q missing value detail", resource.Action, attribute.Path)
	}
	return nil
}

func requireOutputValue(record compactRecord, output plan.OutputChange) error {
	if len(record.Attrs) != 1 {
		return fmt.Errorf("value record count=%d, want 1", len(record.Attrs))
	}
	expr := record.Attrs[0]
	if record.Meta["summary"] == "true" {
		if record.Meta["detail_required"] != "true" {
			return fmt.Errorf("summary missing detail_required=true")
		}
		return nil
	}
	if len(output.Attributes) == 0 {
		if expr != "attrs=none" {
			return fmt.Errorf("expr=%q, want attrs=none", expr)
		}
		return nil
	}
	attribute := output.Attributes[0]
	switch output.Action {
	case plan.ActionCreate:
		if !strings.HasPrefix(expr, "+") {
			return fmt.Errorf("create expr=%q missing +", expr)
		}
		if got, err := codec.Unescape(strings.TrimPrefix(expr, "+")); err != nil || got != rawLineValue(attribute.After) {
			return fmt.Errorf("create expr=%q does not match after value", expr)
		}
	case plan.ActionDelete:
		if !strings.HasPrefix(expr, "-") {
			return fmt.Errorf("delete expr=%q missing -", expr)
		}
		if got, err := codec.Unescape(strings.TrimPrefix(expr, "-")); err != nil || got != rawLineValue(attribute.Before) {
			return fmt.Errorf("delete expr=%q does not match before value", expr)
		}
	default:
		parts := strings.SplitN(expr, "->", 2)
		if len(parts) != 2 {
			return fmt.Errorf("update expr=%q missing ->", expr)
		}
		before, beforeErr := codec.Unescape(parts[0])
		after, afterErr := codec.Unescape(parts[1])
		if beforeErr != nil || afterErr != nil || before != rawLineValue(attribute.Before) || after != rawLineValue(attribute.After) {
			return fmt.Errorf("update expr=%q does not match before/after values", expr)
		}
	}
	return nil
}

func parseCompact(output string) (compactOutput, error) {
	parsed := compactOutput{
		Resources:    make(map[string]compactRecord),
		Outputs:      make(map[string]compactRecord),
		Omit:         make(map[string]int),
		Compress:     make(map[string]int),
		Templates:    make(map[string]string),
		Values:       make(map[string]string),
		ReasonLegend: make(map[string]string),
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "TFP2 ") {
		return parsed, fmt.Errorf("missing TFP2 header")
	}
	parsed.Header = parseHeader(lines[0])

	var currentGroup *parsedGroup
	for _, line := range lines[1:] {
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "OMIT|"):
			counts, err := parseCountFields(strings.TrimPrefix(line, "OMIT|"))
			if err != nil {
				return parsed, err
			}
			parsed.Omit = counts
			continue
		case strings.HasPrefix(line, "COMPRESS|"):
			counts, err := parseCountFields(strings.TrimPrefix(line, "COMPRESS|"))
			if err != nil {
				return parsed, err
			}
			parsed.Compress = counts
			continue
		case strings.HasPrefix(line, "TPL|"):
			parts := strings.SplitN(line, "|", 3)
			if len(parts) != 3 {
				return parsed, fmt.Errorf("invalid template line %q", line)
			}
			prefix, err := codec.Unescape(parts[2])
			if err != nil {
				return parsed, err
			}
			parsed.Templates[parts[1]] = prefix
			continue
		case strings.HasPrefix(line, "VAL|"):
			parts := strings.SplitN(line, "|", 3)
			if len(parts) != 3 {
				return parsed, fmt.Errorf("invalid value line %q", line)
			}
			value, err := codec.Unescape(parts[2])
			if err != nil {
				return parsed, err
			}
			parsed.Values[parts[1]] = value
			continue
		case strings.HasPrefix(line, "REASON_CODES|"):
			if err := parseReasonCodes(parsed.ReasonLegend, strings.TrimPrefix(line, "REASON_CODES|")); err != nil {
				return parsed, err
			}
			continue
		case strings.HasPrefix(line, "DRIFT|"):
			counts, err := parseCountFields(strings.TrimPrefix(line, "DRIFT|"))
			if err != nil {
				return parsed, err
			}
			parsed.DriftTotal = counts["total"]
			parsed.DriftSumm = counts["summ"]
			parsed.DriftDetail = counts["detail"]
			continue
		case strings.HasPrefix(line, "DRIFT_GROUP|"):
			parsed.DriftGroups++
			counts, err := parseCountFields(strings.TrimPrefix(line, "DRIFT_GROUP|"))
			if err != nil {
				return parsed, err
			}
			parsed.DriftGroupResources += counts["count"]
			continue
		case strings.HasPrefix(line, "DRIFT_DETAIL|"):
			parsed.DriftDetailRecords++
			continue
		case strings.HasPrefix(line, "MIGRATION?|"):
			continue
		case strings.HasPrefix(line, "L|IAM|"):
			records, err := parseIAMLens(line, parsed.Templates, parsed.Values, parsed.ReasonLegend)
			if err != nil {
				return parsed, err
			}
			for _, record := range records {
				if err := addUniqueRecord(parsed.Resources, record); err != nil {
					return parsed, err
				}
			}
			parsed.Lenses++
			continue
		case strings.HasPrefix(line, "GL|"):
			records, err := parseListGroup(line, parsed.Templates, parsed.Values, parsed.ReasonLegend)
			if err != nil {
				return parsed, err
			}
			for _, record := range records {
				if err := addUniqueRecord(parsed.Resources, record); err != nil {
					return parsed, err
				}
			}
			parsed.Groups++
			continue
		case strings.HasPrefix(line, "G|"):
			group, err := parseGroup(line, parsed.Values, parsed.ReasonLegend)
			if err != nil {
				return parsed, err
			}
			currentGroup = &group
			parsed.Groups++
			continue
		case strings.HasPrefix(line, "|"):
			if currentGroup == nil {
				return parsed, fmt.Errorf("group row without group header %q", line)
			}
			record, err := parseGroupRow(line, *currentGroup, parsed.Templates, parsed.Values)
			if err != nil {
				return parsed, err
			}
			if err := addUniqueRecord(parsed.Resources, record); err != nil {
				return parsed, err
			}
			continue
		}
		currentGroup = nil
		parts := strings.Split(line, "|")
		if len(parts) != 4 {
			continue
		}
		action := plan.Action(parts[0])
		if !isRecordAction(action) {
			continue
		}
		addr, err := resolveAddress(parts[1], parsed.Templates)
		if err != nil {
			return parsed, err
		}
		attrs, err := expandAttrs(splitNonEmpty(parts[2]), parsed.Values)
		if err != nil {
			return parsed, err
		}
		record := compactRecord{
			Action: action,
			Addr:   addr,
			Attrs:  attrs,
			Meta:   normalizeMeta(parseMeta(parts[3]), parsed.ReasonLegend),
		}
		if action == plan.ActionOutput {
			if !strings.HasPrefix(addr, "output.") {
				addr = "output." + addr
				record.Addr = addr
			}
			if err := addUniqueRecord(parsed.Outputs, record); err != nil {
				return parsed, err
			}
		} else {
			if err := addUniqueRecord(parsed.Resources, record); err != nil {
				return parsed, err
			}
		}
	}
	return parsed, nil
}

func parseGroup(line string, values map[string]string, legend map[string]string) (parsedGroup, error) {
	parts := strings.SplitN(line, "|", 5)
	if len(parts) != 5 {
		return parsedGroup{}, fmt.Errorf("invalid group line %q", line)
	}
	action := plan.Action(parts[2])
	if !isRecordAction(action) || action == plan.ActionOutput {
		return parsedGroup{}, fmt.Errorf("invalid group action %q", parts[2])
	}
	meta := parseMeta(parts[4])
	columns, err := decodeCSV(meta["cols"])
	if err != nil {
		return parsedGroup{}, err
	}
	common, err := parseCommonAttrs(meta["common"], values)
	if err != nil {
		return parsedGroup{}, err
	}
	delete(meta, "n")
	delete(meta, "cols")
	delete(meta, "common")
	return parsedGroup{
		Action:      action,
		CommonAttrs: common,
		Columns:     columns,
		Meta:        normalizeMeta(meta, legend),
	}, nil
}

func parseGroupRow(line string, group parsedGroup, templates map[string]string, dictValues map[string]string) (compactRecord, error) {
	parts := strings.Split(line, "|")
	if len(parts) < 2 {
		return compactRecord{}, fmt.Errorf("invalid group row %q", line)
	}
	rowValues := parts[1:]
	if len(rowValues) != len(group.Columns) {
		return compactRecord{}, fmt.Errorf("group row has %d columns, want %d: %q", len(rowValues), len(group.Columns), line)
	}
	addr, err := resolveAddress(rowValues[0], templates)
	if err != nil {
		return compactRecord{}, err
	}
	attrs := append([]string(nil), group.CommonAttrs...)
	for i, column := range group.Columns[1:] {
		value, err := resolveValue(rowValues[i+1], dictValues)
		if err != nil {
			return compactRecord{}, err
		}
		attrs = append(attrs, column+"="+value)
	}
	meta := cloneMap(group.Meta)
	return compactRecord{Action: group.Action, Addr: addr, Attrs: attrs, Meta: meta}, nil
}

func parseCommonAttrs(raw string, values map[string]string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	items := strings.Split(raw, ",")
	attrs := make([]string, 0, len(items))
	for _, item := range items {
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid common attribute %q", item)
		}
		path, err := codec.Unescape(parts[0])
		if err != nil {
			return nil, err
		}
		value, err := resolveValue(parts[1], values)
		if err != nil {
			return nil, err
		}
		attrs = append(attrs, path+"="+value)
	}
	return attrs, nil
}

func parseListGroup(line string, templates, values map[string]string, legend map[string]string) ([]compactRecord, error) {
	parts := strings.SplitN(line, "|", 5)
	if len(parts) != 5 {
		return nil, fmt.Errorf("invalid list group line %q", line)
	}
	action := plan.Action(parts[2])
	if !isRecordAction(action) || action == plan.ActionOutput {
		return nil, fmt.Errorf("invalid list group action %q", parts[2])
	}
	meta := parseMeta(parts[4])
	common, err := parseCommonAttrs(meta["common"], values)
	if err != nil {
		return nil, err
	}
	refs, err := decodeCSV(meta["refs"])
	if err != nil {
		return nil, err
	}
	vals, err := decodeCSVWithValues(meta["vals"], values)
	if err != nil {
		return nil, err
	}
	if len(refs) != len(vals) {
		return nil, fmt.Errorf("list group refs=%d vals=%d", len(refs), len(vals))
	}
	column, err := codec.Unescape(meta["col"])
	if err != nil {
		return nil, err
	}
	delete(meta, "n")
	delete(meta, "col")
	delete(meta, "vals")
	delete(meta, "refs")
	delete(meta, "common")
	normalized := normalizeMeta(meta, legend)
	records := make([]compactRecord, 0, len(refs))
	for i, ref := range refs {
		addr, err := resolveAddress(ref, templates)
		if err != nil {
			return nil, err
		}
		attrs := append([]string(nil), common...)
		attrs = append(attrs, column+"="+vals[i])
		records = append(records, compactRecord{Action: action, Addr: addr, Attrs: attrs, Meta: cloneMap(normalized)})
	}
	return records, nil
}

func parseIAMLens(line string, templates, values map[string]string, legend map[string]string) ([]compactRecord, error) {
	parts := strings.SplitN(line, "|", 6)
	if len(parts) != 6 {
		return nil, fmt.Errorf("invalid IAM lens line %q", line)
	}
	action := plan.Action(parts[3])
	if action != plan.ActionCreate && action != plan.ActionDelete {
		return nil, fmt.Errorf("invalid IAM lens action %q", parts[3])
	}
	meta := parseMeta(parts[5])
	var scopePath, scopeValue string
	var err error
	if meta["proj"] != "" {
		scopePath = "project"
		scopeValue, err = resolveValue(meta["proj"], values)
	} else {
		scopePath, scopeValue, err = parseLensPathValue(meta["scope"], values)
	}
	if err != nil {
		return nil, err
	}
	var principalPath, principalValue string
	if meta["mem"] != "" {
		principalPath = "member"
		principalValue, err = resolveValue(meta["mem"], values)
	} else {
		principalPath, principalValue, err = parseLensPathValue(meta["principal"], values)
	}
	if err != nil {
		return nil, err
	}
	rolePath := "role"
	if meta["role_path"] != "" {
		rolePath, err = codec.Unescape(meta["role_path"])
		if err != nil {
			return nil, err
		}
	}
	roles, err := decodeCSVWithValues(meta["roles"], values)
	if err != nil {
		return nil, err
	}
	refs, err := decodeCSV(meta["refs"])
	if err != nil {
		return nil, err
	}
	if len(roles) != len(refs) {
		return nil, fmt.Errorf("IAM lens refs=%d roles=%d", len(refs), len(roles))
	}
	for _, key := range []string{"n", "proj", "mem", "scope", "principal", "role_path", "roles", "refs"} {
		delete(meta, key)
	}
	normalized := normalizeMeta(meta, legend)
	records := make([]compactRecord, 0, len(refs))
	for i, ref := range refs {
		addr, err := resolveAddress(ref, templates)
		if err != nil {
			return nil, err
		}
		attrs := []string{
			scopePath + "=" + scopeValue,
			principalPath + "=" + principalValue,
			rolePath + "=" + roles[i],
		}
		records = append(records, compactRecord{Action: action, Addr: addr, Attrs: attrs, Meta: cloneMap(normalized)})
	}
	return records, nil
}

func parseLensPathValue(raw string, values map[string]string) (string, string, error) {
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid lens path/value %q", raw)
	}
	path, err := codec.Unescape(parts[0])
	if err != nil {
		return "", "", err
	}
	value, err := resolveValue(parts[1], values)
	if err != nil {
		return "", "", err
	}
	return path, value, nil
}

func parseReasonCodes(legend map[string]string, raw string) error {
	for _, item := range splitNonEmpty(raw) {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid reason code %q", item)
		}
		code, err := codec.Unescape(parts[0])
		if err != nil {
			return err
		}
		full, err := codec.Unescape(parts[1])
		if err != nil {
			return err
		}
		legend[code] = full
	}
	return nil
}

func parseHeader(line string) map[string]int {
	fields := strings.Fields(line)
	values := make(map[string]int)
	for _, field := range fields[1:] {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if value, err := strconv.Atoi(parts[1]); err == nil {
			values[parts[0]] = value
		}
	}
	return values
}

func parseMeta(field string) map[string]string {
	values := make(map[string]string)
	for _, entry := range splitNonEmpty(field) {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		values[parts[0]] = parts[1]
	}
	return values
}

func normalizeMeta(meta map[string]string, legend map[string]string) map[string]string {
	aliases := map[string]string{
		"why":  "reason",
		"unk":  "unknown",
		"sens": "sensitive",
		"repl": "replace_path",
		"acts": "actions",
		"def":  "defaults",
		"comp": "computed",
		"summ": "summarized",
		"prev": "previous",
		"gen":  "generated_config",
		"omit": "omitted_attrs",
	}
	normalized := make(map[string]string, len(meta))
	for key, value := range meta {
		if alias, ok := aliases[key]; ok {
			key = alias
		}
		if key == "reason" {
			decoded, err := codec.Unescape(value)
			if err == nil {
				if full, ok := legend[decoded]; ok {
					value = codec.Escape(full)
				}
			}
		}
		normalized[key] = value
	}
	return normalized
}

func resolveAddress(raw string, templates map[string]string) (string, error) {
	if !strings.HasPrefix(raw, "$") {
		return codec.Unescape(raw)
	}
	parts := strings.SplitN(strings.TrimPrefix(raw, "$"), ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid template reference %q", raw)
	}
	prefix, ok := templates[parts[0]]
	if !ok {
		return "", fmt.Errorf("unknown template %q", parts[0])
	}
	suffix, err := codec.Unescape(parts[1])
	if err != nil {
		return "", err
	}
	return prefix + suffix, nil
}

func decodeCSV(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value, err := codec.Unescape(part)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func decodeCSVWithValues(raw string, values map[string]string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value, err := resolveValue(part, values)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func resolveValue(raw string, values map[string]string) (string, error) {
	if !strings.HasPrefix(raw, "$V") {
		return codec.Unescape(raw)
	}
	value, ok := values[strings.TrimPrefix(raw, "$")]
	if !ok {
		return "", fmt.Errorf("unknown value dictionary %q", raw)
	}
	return value, nil
}

func expandAttrs(attrs []string, values map[string]string) ([]string, error) {
	out := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		parts := strings.SplitN(attr, "=", 2)
		if len(parts) != 2 {
			out = append(out, attr)
			continue
		}
		value, err := resolveValue(parts[1], values)
		if err != nil {
			return nil, err
		}
		out = append(out, parts[0]+"="+value)
	}
	return out, nil
}

func splitNonEmpty(field string) []string {
	if field == "" {
		return nil
	}
	var out []string
	for _, value := range strings.Split(field, ";") {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func decodedMeta(raw string) string {
	decoded, err := codec.Unescape(raw)
	if err != nil {
		return raw
	}
	return decoded
}

func sameCSV(raw string, want []string) bool {
	if raw == "" {
		return false
	}
	parts := strings.Split(raw, ",")
	if len(parts) != len(want) {
		return false
	}
	for i := range parts {
		value, err := codec.Unescape(parts[i])
		if err != nil {
			return false
		}
		if value != want[i] {
			return false
		}
	}
	return true
}

func isRecordAction(action plan.Action) bool {
	return action == plan.ActionCreate ||
		action == plan.ActionUpdate ||
		action == plan.ActionReplace ||
		action == plan.ActionDelete ||
		action == plan.ActionRead ||
		action == plan.ActionOutput
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func parseCountFields(raw string) (map[string]int, error) {
	counts := make(map[string]int)
	for _, entry := range splitNonEmpty(raw) {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		value, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		counts[parts[0]] = value
	}
	return counts, nil
}

func sumCounts(values map[string]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func representedPaths(attrs []string) map[string]struct{} {
	paths := make(map[string]struct{}, len(attrs))
	for _, attr := range attrs {
		parts := strings.SplitN(attr, "=", 2)
		if len(parts) != 2 {
			continue
		}
		path, err := codec.Unescape(parts[0])
		if err != nil {
			path = parts[0]
		}
		paths[path] = struct{}{}
	}
	return paths
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func rawLineValue(value plan.Value) string {
	switch value.Kind {
	case plan.ValueNull:
		return "null"
	case plan.ValueUnknown:
		return "unknown"
	case plan.ValueSensitive:
		return "sensitive"
	case plan.ValueExists:
		return "exists"
	case plan.ValueRaw:
		encoded, err := json.Marshal(value.Raw)
		if err != nil {
			return ""
		}
		return string(encoded)
	default:
		return "null"
	}
}

func addUniqueRecord(records map[string]compactRecord, record compactRecord) error {
	key := compactRecordKey(record)
	if _, exists := records[key]; exists {
		return fmt.Errorf("duplicate compact record %s", record.Addr)
	}
	records[key] = record
	return nil
}

func compactRecordKey(record compactRecord) string {
	if deposed := decodedMeta(record.Meta["deposed"]); deposed != "" {
		return record.Addr + "#deposed=" + deposed
	}
	return record.Addr
}

func resourceRecordKey(resource plan.ResourceChange) string {
	if resource.Deposed != "" {
		return resource.Address + "#deposed=" + resource.Deposed
	}
	return resource.Address
}

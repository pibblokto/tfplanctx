package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/piblokto/tfplanctx/internal/codec"
	"github.com/piblokto/tfplanctx/internal/plan"
)

func renderReviewGroup(b *strings.Builder, group reviewGroup, templates []addressTemplate, dictionaries []valueDictionary) {
	if group.ListColumn != "" {
		b.WriteString(renderListGroupLine(group, templates, dictionaries))
		return
	}
	fmt.Fprintf(b, "G|%s|%s|%s|n=%d;cols=%s",
		group.ID,
		group.Action,
		codec.Escape(group.Type),
		len(group.Records),
		joinEscaped(group.Columns),
	)
	if len(group.CommonAttrs) > 0 {
		fmt.Fprintf(b, ";common=%s", renderCommonAttrs(group.CommonAttrs, dictionaries))
	}
	if len(group.Meta) > 0 {
		fmt.Fprintf(b, ";%s", strings.Join(group.Meta, ";"))
	}
	b.WriteByte('\n')
	for _, record := range group.Records {
		values := []string{renderTemplatedAddress(record.Resource.Address, templates)}
		for _, column := range group.Columns[1:] {
			values = append(values, renderDictionaryValue(reviewAttrValue(record.Attrs, column), dictionaries))
		}
		fmt.Fprintf(b, "|%s\n", strings.Join(values, "|"))
	}
}

func maybeListCompressGroup(group reviewGroup, templates []addressTemplate, dictionaries []valueDictionary) reviewGroup {
	if len(group.Records) < 3 || len(group.Columns) != 2 {
		return group
	}
	column := group.Columns[1]
	values := make([]string, 0, len(group.Records))
	for _, record := range group.Records {
		value := reviewAttrValue(record.Attrs, column)
		if !isScalarRenderedValue(value) {
			return group
		}
		values = append(values, value)
	}
	candidate := group
	candidate.ListColumn = column
	candidate.ListValues = values
	regularCost := estimatePlainReviewGroupCost(group, templates, dictionaries)
	listCost := len(renderListGroupLine(candidate, templates, dictionaries))
	if listCost >= regularCost {
		return group
	}
	return candidate
}

func estimatePlainReviewGroupCost(group reviewGroup, templates []addressTemplate, dictionaries []valueDictionary) int {
	copy := group
	copy.ListColumn = ""
	copy.ListValues = nil
	return estimateReviewGroupCost(copy, templates, dictionaries)
}

func renderListGroupLine(group reviewGroup, templates []addressTemplate, dictionaries []valueDictionary) string {
	parts := []string{
		fmt.Sprintf("n=%d", len(group.Records)),
		"col=" + codec.Escape(group.ListColumn),
		"vals=" + renderDictionaryList(group.ListValues, dictionaries),
		"refs=" + renderGroupRefs(group.Records, templates),
	}
	if len(group.CommonAttrs) > 0 {
		parts = append(parts, "common="+renderCommonAttrs(group.CommonAttrs, dictionaries))
	}
	parts = append(parts, group.Meta...)
	return fmt.Sprintf("GL|%s|%s|%s|%s\n",
		group.ID,
		group.Action,
		codec.Escape(group.Type),
		strings.Join(parts, ";"),
	)
}

func renderGroupRefs(records []reviewRecord, templates []addressTemplate) string {
	refs := make([]string, 0, len(records))
	for _, record := range records {
		refs = append(refs, codec.EscapeListItem(renderTemplatedAddress(record.Resource.Address, templates)))
	}
	return strings.Join(refs, ",")
}

func renderLensRefs(records []reviewRecord, templates []addressTemplate) string {
	return renderGroupRefs(records, templates)
}

func isScalarRenderedValue(value string) bool {
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{") {
		return false
	}
	for _, prefix := range []string{"list(", "object(", "json("} {
		if strings.HasPrefix(value, prefix) {
			return false
		}
	}
	return true
}

func selectValueDictionaries(records []reviewRecord) []valueDictionary {
	counts := make(map[string]int)
	for _, record := range records {
		for _, attr := range record.Attrs {
			if len(attr.Value) >= 20 {
				counts[attr.Value]++
			}
		}
	}
	var candidates []valueDictionary
	for value, count := range counts {
		if count < 3 {
			continue
		}
		definitionCost := len("VAL|V999|") + len(codec.Escape(value)) + 1
		uncompressedCost := count * len(value)
		compressedCost := definitionCost + count*len("$V999")
		saving := uncompressedCost - compressedCost
		if saving > 0 {
			candidates = append(candidates, valueDictionary{Value: value, Saving: saving})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Saving != candidates[j].Saving {
			return candidates[i].Saving > candidates[j].Saving
		}
		return candidates[i].Value < candidates[j].Value
	})
	if len(candidates) > maxValueDictionaries {
		candidates = candidates[:maxValueDictionaries]
	}
	for index := range candidates {
		candidates[index].ID = fmt.Sprintf("V%d", index+1)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	return candidates
}

func renderDictionaryValue(value string, dictionaries []valueDictionary) string {
	for _, dictionary := range dictionaries {
		if dictionary.Value == value {
			return "$" + dictionary.ID
		}
	}
	return value
}

func renderDictionaryList(values []string, dictionaries []valueDictionary) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, codec.EscapeListItem(renderDictionaryValue(value, dictionaries)))
	}
	return strings.Join(parts, ",")
}

func selectIAMLenses(records []reviewRecord, templates []addressTemplate, dictionaries []valueDictionary, legend map[string]string) ([]iamLens, []reviewRecord) {
	buckets := make(map[string][]reviewRecord)
	for _, record := range records {
		scopePath, scopeValue, principalPath, principalValue, rolePath, _, ok := iamLensFields(record)
		if !ok {
			continue
		}
		key := strings.Join([]string{
			string(record.Resource.Action),
			record.Resource.Type,
			record.Resource.ProviderName,
			reviewReason(record.Resource.ActionReason, legend),
			scopePath,
			scopeValue,
			principalPath,
			principalValue,
			rolePath,
			strings.Join(record.Resource.UnknownPaths, ","),
			strings.Join(record.Resource.SensitivePaths, ","),
			strings.Join(record.DefaultPaths, ","),
			strings.Join(record.OmittedComputedPaths, ","),
			strings.Join(record.Resource.ReplacePaths, ","),
			strings.Join(riskNames(record.Resource), ","),
			fmt.Sprintf("%d", record.OmittedAttributes),
		}, "\x00")
		buckets[key] = append(buckets[key], record)
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	used := make(map[string]struct{})
	var lenses []iamLens
	for _, key := range keys {
		candidateRecords := buckets[key]
		if len(candidateRecords) < 3 {
			continue
		}
		sort.Slice(candidateRecords, func(i, j int) bool {
			return candidateRecords[i].Resource.Address < candidateRecords[j].Resource.Address
		})
		lens, ok := buildIAMLens(candidateRecords, fmt.Sprintf("I%d", len(lenses)+1), legend)
		if !ok {
			continue
		}
		group := buildReviewGroup(candidateRecords, "GX", legend)
		group = maybeListCompressGroup(group, templates, dictionaries)
		lensCost := estimateIAMLensCost(lens, templates, dictionaries)
		groupCost := estimateReviewGroupCost(group, templates, dictionaries)
		if lensCost >= groupCost {
			continue
		}
		lenses = append(lenses, lens)
		for _, record := range candidateRecords {
			used[record.Resource.Address] = struct{}{}
		}
	}
	var remaining []reviewRecord
	for _, record := range records {
		if _, ok := used[record.Resource.Address]; !ok {
			remaining = append(remaining, record)
		}
	}
	return lenses, remaining
}

func buildIAMLens(records []reviewRecord, id string, legend map[string]string) (iamLens, bool) {
	first := records[0]
	scopePath, scopeValue, principalPath, principalValue, rolePath, roleValue, ok := iamLensFields(first)
	if !ok {
		return iamLens{}, false
	}
	lens := iamLens{
		ID:             id,
		Action:         first.Resource.Action,
		Type:           first.Resource.Type,
		Records:        records,
		ScopePath:      scopePath,
		ScopeValue:     scopeValue,
		PrincipalPath:  principalPath,
		PrincipalValue: principalValue,
		RolePath:       rolePath,
		Roles:          []string{roleValue},
		Meta:           reviewMetadata(first, legend),
	}
	for _, record := range records[1:] {
		gotScopePath, gotScopeValue, gotPrincipalPath, gotPrincipalValue, gotRolePath, gotRoleValue, ok := iamLensFields(record)
		if !ok ||
			gotScopePath != scopePath ||
			gotScopeValue != scopeValue ||
			gotPrincipalPath != principalPath ||
			gotPrincipalValue != principalValue ||
			gotRolePath != rolePath {
			return iamLens{}, false
		}
		lens.Roles = append(lens.Roles, gotRoleValue)
	}
	return lens, true
}

func iamLensFields(record reviewRecord) (scopePath, scopeValue, principalPath, principalValue, rolePath, roleValue string, ok bool) {
	if !isIAMLikeType(record.Resource.Type) || !isGroupable(record) || record.Resource.Action != plan.ActionCreate && record.Resource.Action != plan.ActionDelete {
		return "", "", "", "", "", "", false
	}
	if len(record.Attrs) != 3 {
		return "", "", "", "", "", "", false
	}
	for _, attr := range record.Attrs {
		switch attr.Path {
		case "project", "project_id", "account_id", "folder", "organization", "namespace":
			scopePath, scopeValue = attr.Path, attr.Value
		case "member", "principal":
			principalPath, principalValue = attr.Path, attr.Value
		case "role":
			rolePath, roleValue = attr.Path, attr.Value
		}
	}
	return scopePath, scopeValue, principalPath, principalValue, rolePath, roleValue,
		scopePath != "" && principalPath != "" && rolePath != ""
}

func isIAMLikeType(resourceType string) bool {
	lower := strings.ToLower(resourceType)
	return strings.Contains(lower, "iam") ||
		strings.Contains(lower, "access_binding") ||
		strings.Contains(lower, "role_assignment")
}

func estimateIAMLensCost(lens iamLens, templates []addressTemplate, dictionaries []valueDictionary) int {
	return len(renderIAMLensLine(lens, templates, dictionaries))
}

func renderIAMLensLine(lens iamLens, templates []addressTemplate, dictionaries []valueDictionary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "L|IAM|%s|%s|%s|n=%d",
		lens.ID,
		lens.Action,
		codec.Escape(lens.Type),
		len(lens.Records),
	)
	if lens.ScopePath == "project" {
		fmt.Fprintf(&b, ";proj=%s", renderDictionaryValue(lens.ScopeValue, dictionaries))
	} else {
		fmt.Fprintf(&b, ";scope=%s:%s", codec.EscapeListItem(lens.ScopePath), renderDictionaryValue(lens.ScopeValue, dictionaries))
	}
	if lens.PrincipalPath == "member" {
		fmt.Fprintf(&b, ";mem=%s", renderDictionaryValue(lens.PrincipalValue, dictionaries))
	} else {
		fmt.Fprintf(&b, ";principal=%s:%s", codec.EscapeListItem(lens.PrincipalPath), renderDictionaryValue(lens.PrincipalValue, dictionaries))
	}
	if lens.RolePath != "role" {
		fmt.Fprintf(&b, ";role_path=%s", codec.Escape(lens.RolePath))
	}
	fmt.Fprintf(&b, ";roles=%s;refs=%s",
		renderDictionaryList(lens.Roles, dictionaries),
		renderLensRefs(lens.Records, templates),
	)
	if len(lens.Meta) > 0 {
		fmt.Fprintf(&b, ";%s", strings.Join(lens.Meta, ";"))
	}
	b.WriteByte('\n')
	return b.String()
}

func selectMigrationSummaries(records []reviewRecord) []migrationSummary {
	type bucket struct {
		creates []reviewRecord
		deletes []reviewRecord
	}
	buckets := make(map[string]*bucket)
	for _, record := range records {
		if record.Resource.Action != plan.ActionCreate && record.Resource.Action != plan.ActionDelete {
			continue
		}
		if !isIAMLikeType(record.Resource.Type) {
			continue
		}
		item := buckets[record.Resource.Type]
		if item == nil {
			item = &bucket{}
			buckets[record.Resource.Type] = item
		}
		if record.Resource.Action == plan.ActionCreate {
			item.creates = append(item.creates, record)
		} else {
			item.deletes = append(item.deletes, record)
		}
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var summaries []migrationSummary
	for _, key := range keys {
		item := buckets[key]
		if len(item.creates) == 0 || len(item.deletes) == 0 {
			continue
		}
		sameScope := commonIAMScope(item.creates, item.deletes)
		commonRoles := commonIAMRoles(item.creates, item.deletes)
		// Same scope alone is common in ordinary churn; require overlapping roles
		// before adding a heuristic migration hint so the summary earns its cost.
		if len(commonRoles) == 0 {
			continue
		}
		summaries = append(summaries, migrationSummary{
			Type:        key,
			Creates:     len(item.creates),
			Deletes:     len(item.deletes),
			SameScope:   sameScope,
			CommonRoles: commonRoles,
		})
	}
	return summaries
}

func commonIAMScope(groups ...[]reviewRecord) string {
	var common string
	for _, group := range groups {
		for _, record := range group {
			path, value, _, _, _, _, ok := iamLensFields(record)
			if !ok {
				return ""
			}
			current := path + ":" + value
			if common == "" {
				common = current
				continue
			}
			if common != current {
				return ""
			}
		}
	}
	return common
}

func commonIAMRoles(left, right []reviewRecord) []string {
	leftRoles := make(map[string]struct{})
	for _, record := range left {
		_, _, _, _, _, role, ok := iamLensFields(record)
		if ok {
			leftRoles[role] = struct{}{}
		}
	}
	var common []string
	seen := make(map[string]struct{})
	for _, record := range right {
		_, _, _, _, _, role, ok := iamLensFields(record)
		if !ok {
			continue
		}
		if _, ok := leftRoles[role]; ok {
			if _, duplicate := seen[role]; !duplicate {
				common = append(common, role)
				seen[role] = struct{}{}
			}
		}
	}
	sort.Strings(common)
	return common
}

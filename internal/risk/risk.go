package risk

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/piblokto/tfplanctx/internal/diff"
)

var statefulResourceTypes = map[string]struct{}{
	"aws_db_instance":                   {},
	"aws_rds_cluster":                   {},
	"aws_ebs_volume":                    {},
	"aws_s3_bucket":                     {},
	"aws_dynamodb_table":                {},
	"aws_elasticache_cluster":           {},
	"aws_elasticache_replication_group": {},
	"aws_opensearch_domain":             {},
	"aws_elasticsearch_domain":          {},
	"google_sql_database_instance":      {},
	"google_compute_disk":               {},
	"azurerm_postgresql_server":         {},
	"azurerm_mysql_server":              {},
	"azurerm_mssql_server":              {},
	"azurerm_managed_disk":              {},
}

var helmSensitivePaths = map[string]struct{}{
	"values":        {},
	"set":           {},
	"set_sensitive": {},
	"chart":         {},
	"version":       {},
	"repository":    {},
	"namespace":     {},
}

// Resource contains the raw inputs needed for deterministic risk detection.
type Resource struct {
	Type    string
	Action  string
	Before  any
	After   any
	Changes []diff.Change
}

// Detect returns stable, conservative risk labels.
func Detect(resource Resource) []string {
	var risks []string

	if publicIngress(resource) {
		risks = append(risks, "public_ingress")
	}
	if dataLoss(resource) {
		risks = append(risks, "data_loss")
	}
	if iamWildcard(resource) {
		risks = append(risks, "iam_wildcard")
	}
	if privilegedKubernetes(resource) {
		risks = append(risks, "privileged_kubernetes")
	}
	if helmReleaseChanged(resource) {
		risks = append(risks, "helm_release_changed")
	}

	sort.Strings(risks)
	return risks
}

func publicIngress(resource Resource) bool {
	switch resource.Type {
	case "aws_security_group":
		after, ok := asMap(resource.After)
		if !ok {
			return false
		}
		return containsPublicCIDR(after["ingress"])
	case "aws_security_group_rule":
		after, ok := asMap(resource.After)
		if !ok {
			return false
		}
		ruleType, _ := after["type"].(string)
		return strings.EqualFold(ruleType, "ingress") && containsPublicCIDR(after)
	default:
		return false
	}
}

func dataLoss(resource Resource) bool {
	if resource.Action != "D" && resource.Action != "R" {
		return false
	}
	_, ok := statefulResourceTypes[resource.Type]
	return ok
}

func iamWildcard(resource Resource) bool {
	if resource.Type != "aws_iam_policy" && resource.Type != "aws_iam_role_policy" && !strings.Contains(resource.Type, "aws_iam_policy_document") {
		return false
	}
	return containsIAMWildcard(resource.After)
}

func privilegedKubernetes(resource Resource) bool {
	if resource.Type != "kubernetes_manifest" && !strings.HasPrefix(resource.Type, "kubernetes_") {
		return false
	}
	for _, change := range resource.Changes {
		path := strings.ToLower(change.Path.String())
		if (strings.Contains(path, "securitycontext.privileged") || strings.Contains(path, "security_context.privileged")) && change.After == true {
			return true
		}
	}
	return false
}

func helmReleaseChanged(resource Resource) bool {
	if resource.Type != "helm_release" {
		return false
	}
	for _, change := range resource.Changes {
		if len(change.Path) == 0 || change.Path[0].IsIndex {
			continue
		}
		if _, ok := helmSensitivePaths[change.Path[0].Key]; ok {
			return true
		}
	}
	return false
}

func containsPublicCIDR(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed == "0.0.0.0/0" || typed == "::/0"
	case []any:
		for _, item := range typed {
			if containsPublicCIDR(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if containsPublicCIDR(item) {
				return true
			}
		}
	}
	return false
}

func containsIAMWildcard(value any) bool {
	switch typed := value.(type) {
	case string:
		var decoded any
		decoder := json.NewDecoder(bytes.NewBufferString(typed))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err == nil {
			return containsIAMWildcard(decoded)
		}
	case []any:
		for _, item := range typed {
			if containsIAMWildcard(item) {
				return true
			}
		}
	case map[string]any:
		for key, item := range typed {
			if strings.EqualFold(key, "Action") || strings.EqualFold(key, "Resource") {
				if isWildcardValue(item) {
					return true
				}
			}
			if containsIAMWildcard(item) {
				return true
			}
		}
	}
	return false
}

func isWildcardValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed == "*"
	case []any:
		for _, item := range typed {
			if stringItem, ok := item.(string); ok && stringItem == "*" {
				return true
			}
		}
	}
	return false
}

func asMap(value any) (map[string]any, bool) {
	mapped, ok := value.(map[string]any)
	return mapped, ok
}

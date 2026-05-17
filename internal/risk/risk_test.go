package risk

import (
	"testing"

	"github.com/pibblokto/tfplanctx/internal/diff"
)

func TestPublicIngressDetection(t *testing.T) {
	risks := Detect(Resource{
		Type:   "aws_security_group",
		Action: "U",
		After:  map[string]any{"ingress": []any{map[string]any{"cidr_blocks": []any{"0.0.0.0/0"}}}},
	})
	if len(risks) != 1 || risks[0] != "public_ingress" {
		t.Fatalf("risks = %#v", risks)
	}
}

func TestDataLossDetection(t *testing.T) {
	risks := Detect(Resource{Type: "aws_db_instance", Action: "D"})
	if len(risks) != 1 || risks[0] != "data_loss" {
		t.Fatalf("risks = %#v", risks)
	}
}

func TestIAMWildcardDetection(t *testing.T) {
	risks := Detect(Resource{
		Type:   "aws_iam_policy",
		Action: "U",
		After:  map[string]any{"policy": `{"Statement":[{"Action":"*","Resource":"*"}]}`},
	})
	if len(risks) != 1 || risks[0] != "iam_wildcard" {
		t.Fatalf("risks = %#v", risks)
	}
}

func TestPrivilegedKubernetesDetection(t *testing.T) {
	risks := Detect(Resource{
		Type:    "kubernetes_manifest",
		Action:  "U",
		Changes: []diff.Change{{Path: diff.Path{{Key: "manifest"}, {Key: "securityContext"}, {Key: "privileged"}}, After: true}},
	})
	if len(risks) != 1 || risks[0] != "privileged_kubernetes" {
		t.Fatalf("risks = %#v", risks)
	}
}

func TestHelmReleaseChangeDetection(t *testing.T) {
	risks := Detect(Resource{
		Type:   "helm_release",
		Action: "U",
		Changes: []diff.Change{{
			Path:   diff.Path{{Key: "version"}},
			Before: "1.0.0",
			After:  "1.1.0",
		}},
	})
	if len(risks) != 1 || risks[0] != "helm_release_changed" {
		t.Fatalf("risks = %#v", risks)
	}
}

func TestPublicCIDROnEgressDoesNotTriggerIngressRisk(t *testing.T) {
	risks := Detect(Resource{
		Type:   "aws_security_group_rule",
		Action: "U",
		After: map[string]any{
			"type":        "egress",
			"cidr_blocks": []any{"0.0.0.0/0"},
		},
	})
	if len(risks) != 0 {
		t.Fatalf("risks = %#v, want none", risks)
	}
}

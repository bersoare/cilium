// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package ipam

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	cilium_v2alpha1_api "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	slimv1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/apis/meta/v1"
)

func TestPoolSelector_NodeMatching(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	ps := NewPoolSelector(logger)

	// Create pools with different selectors
	catchAllPool := &cilium_v2alpha1_api.CiliumResourceIPPool{
		ObjectMeta: metav1.ObjectMeta{Name: "catch-all-pool"},
		Spec: cilium_v2alpha1_api.ResourceIPPoolSpec{
			NodeSelector: nil, // Catch-all
		},
	}

	rack1Pool := &cilium_v2alpha1_api.CiliumResourceIPPool{
		ObjectMeta: metav1.ObjectMeta{Name: "rack1-pool"},
		Spec: cilium_v2alpha1_api.ResourceIPPoolSpec{
			NodeSelector: &slimv1.LabelSelector{
				MatchLabels: map[string]string{
					"rack": "rack1",
				},
			},
		},
	}

	rack2Pool := &cilium_v2alpha1_api.CiliumResourceIPPool{
		ObjectMeta: metav1.ObjectMeta{Name: "rack2-pool"},
		Spec: cilium_v2alpha1_api.ResourceIPPoolSpec{
			NodeSelector: &slimv1.LabelSelector{
				MatchLabels: map[string]string{
					"rack": "rack2",
				},
			},
		},
	}

	gpuPool := &cilium_v2alpha1_api.CiliumResourceIPPool{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-pool"},
		Spec: cilium_v2alpha1_api.ResourceIPPoolSpec{
			NodeSelector: &slimv1.LabelSelector{
				MatchLabels: map[string]string{
					"accelerator": "gpu",
				},
			},
		},
	}

	// Register pools
	ps.UpsertPool(catchAllPool)
	ps.UpsertPool(rack1Pool)
	ps.UpsertPool(rack2Pool)
	ps.UpsertPool(gpuPool)

	// Test node with rack1 label
	node1 := &v2.CiliumNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			Labels: map[string]string{
				"rack": "rack1",
			},
		},
	}
	allowed := ps.RecalculateNode(node1)
	assert.ElementsMatch(t, []string{"catch-all-pool", "rack1-pool"}, allowed,
		"node1 should have access to catch-all and rack1 pools")

	// Test node with rack2 label
	node2 := &v2.CiliumNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node2",
			Labels: map[string]string{
				"rack": "rack2",
			},
		},
	}
	allowed = ps.RecalculateNode(node2)
	assert.ElementsMatch(t, []string{"catch-all-pool", "rack2-pool"}, allowed,
		"node2 should have access to catch-all and rack2 pools")

	// Test node with GPU label
	node3 := &v2.CiliumNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node3",
			Labels: map[string]string{
				"rack":        "rack1",
				"accelerator": "gpu",
			},
		},
	}
	allowed = ps.RecalculateNode(node3)
	assert.ElementsMatch(t, []string{"catch-all-pool", "rack1-pool", "gpu-pool"}, allowed,
		"node3 should have access to catch-all, rack1, and gpu pools")

	// Test node with no matching labels (only catch-all should match)
	node4 := &v2.CiliumNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node4",
			Labels: map[string]string{
				"zone": "us-west-1",
			},
		},
	}
	allowed = ps.RecalculateNode(node4)
	assert.ElementsMatch(t, []string{"catch-all-pool"}, allowed,
		"node4 should only have access to catch-all pool")

	// Test pool deletion
	ps.DeletePool("rack1-pool")
	allowed = ps.RecalculateNode(node1)
	assert.ElementsMatch(t, []string{"catch-all-pool"}, allowed,
		"after rack1-pool deletion, node1 should only have catch-all")

	// Recalculate node3 after deletion to update its cache
	allowed = ps.RecalculateNode(node3)
	assert.ElementsMatch(t, []string{"catch-all-pool", "gpu-pool"}, allowed,
		"after rack1-pool deletion, node3 should have catch-all and gpu pools")

	// Verify GetAllowedPoolsForNode returns correct values
	retrieved := ps.GetAllowedPoolsForNode("node3")
	assert.ElementsMatch(t, []string{"catch-all-pool", "gpu-pool"}, retrieved,
		"GetAllowedPoolsForNode should return cached value")

	// Test node removal
	ps.RemoveNode("node1")
	retrieved = ps.GetAllowedPoolsForNode("node1")
	assert.Empty(t, retrieved, "after RemoveNode, node should have no allowed pools")
}

func TestPoolSelector_MatchExpressions(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	ps := NewPoolSelector(logger)

	// Create pool with MatchExpressions
	pool := &cilium_v2alpha1_api.CiliumResourceIPPool{
		ObjectMeta: metav1.ObjectMeta{Name: "expr-pool"},
		Spec: cilium_v2alpha1_api.ResourceIPPoolSpec{
			NodeSelector: &slimv1.LabelSelector{
				MatchExpressions: []slimv1.LabelSelectorRequirement{
					{
						Key:      "tier",
						Operator: slimv1.LabelSelectorOpIn,
						Values:   []string{"frontend", "backend"},
					},
					{
						Key:      "deprecated",
						Operator: slimv1.LabelSelectorOpDoesNotExist,
					},
				},
			},
		},
	}

	ps.UpsertPool(pool)

	// Test node that matches (tier=frontend, no deprecated label)
	node1 := &v2.CiliumNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			Labels: map[string]string{
				"tier": "frontend",
			},
		},
	}
	allowed := ps.RecalculateNode(node1)
	require.Contains(t, allowed, "expr-pool", "node1 should match pool with expressions")

	// Test node that doesn't match (tier=compute)
	node2 := &v2.CiliumNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node2",
			Labels: map[string]string{
				"tier": "compute",
			},
		},
	}
	allowed = ps.RecalculateNode(node2)
	assert.NotContains(t, allowed, "expr-pool", "node2 should not match pool")

	// Test node that doesn't match (has deprecated label)
	node3 := &v2.CiliumNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node3",
			Labels: map[string]string{
				"tier":       "frontend",
				"deprecated": "true",
			},
		},
	}
	allowed = ps.RecalculateNode(node3)
	assert.NotContains(t, allowed, "expr-pool", "node3 should not match pool (has deprecated label)")
}

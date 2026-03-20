// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package ipam

import (
	"log/slog"

	"k8s.io/apimachinery/pkg/labels"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	cilium_v2alpha1_api "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	"github.com/cilium/cilium/pkg/lock"
	"github.com/cilium/cilium/pkg/logging/logfields"
)

// PoolSelector manages the mapping between pools and nodes based on nodeSelector.
// It determines which pools are eligible for allocation on which nodes.
type PoolSelector struct {
	logger *slog.Logger
	mutex  lock.RWMutex

	// pools stores all known pools indexed by name
	pools map[string]*cilium_v2alpha1_api.CiliumResourceIPPool

	// nodeToAllowedPools maps node name to list of pool names that can be allocated on that node
	nodeToAllowedPools map[string][]string
}

func NewPoolSelector(logger *slog.Logger) *PoolSelector {
	return &PoolSelector{
		logger:             logger,
		pools:              make(map[string]*cilium_v2alpha1_api.CiliumResourceIPPool),
		nodeToAllowedPools: make(map[string][]string),
	}
}

// UpsertPool registers or updates a pool.
// Call RecalculateNode() afterwards to update node-to-pool mappings.
func (ps *PoolSelector) UpsertPool(pool *cilium_v2alpha1_api.CiliumResourceIPPool) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	ps.pools[pool.Name] = pool.DeepCopy()
}

// DeletePool removes a pool from tracking.
func (ps *PoolSelector) DeletePool(poolName string) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	delete(ps.pools, poolName)

	// Clean up node mappings
	for nodeName := range ps.nodeToAllowedPools {
		ps.nodeToAllowedPools[nodeName] = ps.filterPools(ps.nodeToAllowedPools[nodeName], func(name string) bool {
			return name != poolName
		})
	}
}

// RecalculateNode recalculates which pools are allowed for a given node based on its labels.
// Returns the list of allowed pool names.
func (ps *PoolSelector) RecalculateNode(node *v2.CiliumNode) []string {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	var allowedPools []string
	nodeLabels := labels.Set(node.Labels)

	for poolName, pool := range ps.pools {
		if ps.nodeMatchesPoolSelector(nodeLabels, pool) {
			allowedPools = append(allowedPools, poolName)
		}
	}

	ps.nodeToAllowedPools[node.Name] = allowedPools

	ps.logger.Debug(
		"Recalculated allowed pools for node",
		logfields.NodeName, node.Name,
		"allowedPools", allowedPools,
	)

	return allowedPools
}

// RemoveNode removes a node from tracking.
func (ps *PoolSelector) RemoveNode(nodeName string) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	delete(ps.nodeToAllowedPools, nodeName)
}

// GetAllowedPoolsForNode returns the list of pool names that can be allocated on the given node.
func (ps *PoolSelector) GetAllowedPoolsForNode(nodeName string) []string {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	return append([]string{}, ps.nodeToAllowedPools[nodeName]...)
}

// nodeMatchesPoolSelector checks if a node's labels match a pool's nodeSelector.
// If the pool has no nodeSelector, it's a catch-all pool that matches all nodes.
func (ps *PoolSelector) nodeMatchesPoolSelector(nodeLabels labels.Set, pool *cilium_v2alpha1_api.CiliumResourceIPPool) bool {
	// If pool has no selector, it's a catch-all pool
	if pool.Spec.NodeSelector == nil {
		return true
	}

	// Convert slim LabelSelector to standard k8s LabelSelector
	standardSelector := &metav1.LabelSelector{
		MatchLabels:      pool.Spec.NodeSelector.MatchLabels,
		MatchExpressions: make([]metav1.LabelSelectorRequirement, len(pool.Spec.NodeSelector.MatchExpressions)),
	}
	for i, expr := range pool.Spec.NodeSelector.MatchExpressions {
		standardSelector.MatchExpressions[i] = metav1.LabelSelectorRequirement{
			Key:      expr.Key,
			Operator: metav1.LabelSelectorOperator(expr.Operator),
			Values:   expr.Values,
		}
	}

	selector, err := metav1.LabelSelectorAsSelector(standardSelector)
	if err != nil {
		ps.logger.Warn(
			"Failed to parse pool selector",
			logfields.PoolName, pool.Name,
			logfields.Error, err,
		)
		return false
	}

	return selector.Matches(nodeLabels)
}

// filterPools returns a new slice containing only elements that pass the filter function.
func (ps *PoolSelector) filterPools(pools []string, filter func(string) bool) []string {
	var result []string
	for _, pool := range pools {
		if filter(pool) {
			result = append(result, pool)
		}
	}
	return result
}

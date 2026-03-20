// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package v2alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	slimv1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:categories={cilium},singular="ciliumresourceippool",path="ciliumresourceippools",scope="Cluster",shortName={crip}
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// CiliumResourceIPPool defines an IP pool that can be used for pooled IPAM (i.e. the multi-pool IPAM
// mode).
type CiliumResourceIPPool struct {
	// +deepequal-gen=false
	metav1.TypeMeta `json:",inline"`
	// +deepequal-gen=false
	// +kubebuilder:validation:Optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec ResourceIPPoolSpec `json:"spec"`

	// +kubebuilder:validation:Optional
	Status CiliumResourceIPPoolStatus `json:"status,omitempty"`
}

type ResourceIPPoolSpec struct {
	// IPv4 specifies the IPv4 CIDRs and mask sizes of the pool
	//
	// +kubebuilder:validation:Optional
	IPv4 *IPv4PoolSpec `json:"ipv4"`

	// IPv6 specifies the IPv6 CIDRs and mask sizes of the pool
	//
	// +kubebuilder:validation:Optional
	IPv6 *IPv6PoolSpec `json:"ipv6"`

	// NodeSelector selects the set of nodes that are eligible to use this pool.
	// If not specified, the pool can be used by all nodes (catch-all pool).
	//
	// +kubebuilder:validation:Optional
	NodeSelector *slimv1.LabelSelector `json:"nodeSelector,omitempty"`

	// VlanID specifies the VLAN ID for the network configuration.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4094
	VlanID *int32 `json:"vlanId,omitempty"`

	// Routes specifies the routes to be configured for this pool.
	//
	// +kubebuilder:validation:Optional
	Routes []RouteSpec `json:"routes,omitempty"`
}

// RouteSpec defines a network route configuration.
// +deepequal-gen=true
type RouteSpec struct {
	// Destination is the destination CIDR for the route.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=cidr
	Destination string `json:"destination"`

	// Gateway is the gateway address for the route.
	//
	// +kubebuilder:validation:Optional
	Gateway string `json:"gateway,omitempty"`
}

// CiliumResourceIPPoolStatus defines the observed state of CiliumResourceIPPool.
type CiliumResourceIPPoolStatus struct {
	// Conditions represent the latest available observations of the pool's state.
	//
	// +kubebuilder:validation:Optional
	// +deepequal-gen=false
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +deepequal-gen=false

// CiliumResourceIPPoolList is a list of CiliumResourceIPPool objects.
type CiliumResourceIPPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of CiliumResourceIPPool.
	Items []CiliumResourceIPPool `json:"items"`
}

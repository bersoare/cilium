// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package types

import (
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDriverType(t *testing.T) {
	t.Run("test (un)marshaling", func(t *testing.T) {
		for i := range DeviceManagerTypeUnknown {
			// make sure we handle all supported types
			str, err := i.MarshalText()
			require.NoError(t, err)
			require.NotNil(t, str)

			var unmarshaled DeviceManagerType
			require.NoError(t, unmarshaled.UnmarshalText(str))
			require.Equal(t, i, unmarshaled)

			require.NotEmpty(t, i.String())
		}

		dontExist := DeviceManagerTypeUnknown + 1
		str, err := dontExist.MarshalText()
		require.Error(t, err)
		require.Nil(t, str)

		jsonText := `\"idontexist\"`
		require.Error(t, dontExist.UnmarshalText([]byte(jsonText)))
		require.NotZero(t, dontExist)

		require.Empty(t, dontExist.String())
	})
}

func TestRouteSetMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		routeSet RouteSet
		expected string
	}{
		{
			name:     "nil route set",
			routeSet: nil,
			expected: "null",
		},
		{
			name:     "empty route set",
			routeSet: RouteSet{},
			expected: "{}",
		},
		{
			name: "single route with single gateway",
			routeSet: RouteSet{
				netip.MustParsePrefix("10.0.0.0/8"): {
					netip.MustParseAddr("192.168.1.1"): {},
				},
			},
			expected: `{"10.0.0.0/8":["192.168.1.1"]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.routeSet)
			require.NoError(t, err)

			if tt.expected != "" {
				assert.JSONEq(t, tt.expected, string(data))
			}

			// Verify round-trip
			var decoded RouteSet
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)

			if tt.routeSet == nil {
				assert.Nil(t, decoded)
			} else {
				assert.Equal(t, len(tt.routeSet), len(decoded))
				for dest, gws := range tt.routeSet {
					decodedGws, ok := decoded[dest]
					assert.True(t, ok, "destination %s not found", dest)
					assert.Equal(t, len(gws), len(decodedGws))
					for gw := range gws {
						_, ok := decodedGws[gw]
						assert.True(t, ok, "gateway %s not found", gw)
					}
				}
			}
		})
	}
}

func TestPrefixSetMarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		prefixSet PrefixSet
		expected  string
	}{
		{
			name:      "nil prefix set",
			prefixSet: nil,
			expected:  "null",
		},
		{
			name:      "empty prefix set",
			prefixSet: PrefixSet{},
			expected:  "[]",
		},
		{
			name: "single IPv4 prefix",
			prefixSet: PrefixSet{
				netip.MustParsePrefix("172.16.0.0/12"): {},
			},
			expected: `["172.16.0.0/12"]`,
		},
		{
			name: "single IPv6 prefix",
			prefixSet: PrefixSet{
				netip.MustParsePrefix("fd00::/64"): {},
			},
			expected: `["fd00::/64"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.prefixSet)
			require.NoError(t, err)

			if tt.expected != "" {
				assert.JSONEq(t, tt.expected, string(data))
			}

			// Verify round-trip
			var decoded PrefixSet
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)

			if tt.prefixSet == nil {
				assert.Nil(t, decoded)
			} else {
				assert.Equal(t, len(tt.prefixSet), len(decoded))
				for prefix := range tt.prefixSet {
					_, ok := decoded[prefix]
					assert.True(t, ok, "prefix %s not found", prefix)
				}
			}
		})
	}
}

func TestAddrSetMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		addrSet  AddrSet
		expected string
	}{
		{
			name:     "nil addr set",
			addrSet:  nil,
			expected: "null",
		},
		{
			name:     "empty addr set",
			addrSet:  AddrSet{},
			expected: "[]",
		},
		{
			name: "single IPv4 address",
			addrSet: AddrSet{
				netip.MustParseAddr("172.16.0.1"): {},
			},
			expected: `["172.16.0.1"]`,
		},
		{
			name: "IPv6 address",
			addrSet: AddrSet{
				netip.MustParseAddr("fd00::1"): {},
			},
			expected: `["fd00::1"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.addrSet)
			require.NoError(t, err)

			if tt.expected != "" {
				assert.JSONEq(t, tt.expected, string(data))
			}

			// Verify round-trip
			var decoded AddrSet
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)

			if tt.addrSet == nil {
				assert.Nil(t, decoded)
			} else {
				assert.Equal(t, len(tt.addrSet), len(decoded))
				for addr := range tt.addrSet {
					_, ok := decoded[addr]
					assert.True(t, ok, "address %s not found", addr)
				}
			}
		})
	}
}

func TestDeviceConfigJSONSerialization(t *testing.T) {
	config := DeviceConfig{
		IPv4Addr: netip.MustParsePrefix("10.0.1.5/24"),
		IPv6Addr: netip.MustParsePrefix("fd00::1/64"),
		IPPool:   "test-pool",
		Vlan:     100,
		Routes: RouteSet{
			netip.MustParsePrefix("10.0.0.0/8"): {
				netip.MustParseAddr("192.168.1.1"): {},
			},
		},
		DirectRoutes: PrefixSet{
			netip.MustParsePrefix("192.168.0.0/16"): {},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(config)
	require.NoError(t, err)

	t.Logf("JSON: %s", string(data))

	// Unmarshal back
	var decoded DeviceConfig
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, config.IPv4Addr, decoded.IPv4Addr)
	assert.Equal(t, config.IPv6Addr, decoded.IPv6Addr)
	assert.Equal(t, config.IPPool, decoded.IPPool)
	assert.Equal(t, config.Vlan, decoded.Vlan)

	// Verify routes
	assert.Equal(t, len(config.Routes), len(decoded.Routes))
	for dest, gws := range config.Routes {
		decodedGws, ok := decoded.Routes[dest]
		assert.True(t, ok, "destination %s not found", dest)
		assert.Equal(t, len(gws), len(decodedGws))
	}

	// Verify direct routes
	assert.Equal(t, len(config.DirectRoutes), len(decoded.DirectRoutes))
	for prefix := range config.DirectRoutes {
		_, ok := decoded.DirectRoutes[prefix]
		assert.True(t, ok, "direct route %s not found", prefix)
	}
}

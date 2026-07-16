// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package devicemanagers

import (
	"testing"

	"github.com/cilium/hive/hivetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	"github.com/cilium/cilium/pkg/networkdriver/types"
)

func TestInitManagers_NilConfig_ReturnsNil(t *testing.T) {
	mgrs, err := InitManagers(hivetest.Logger(t), nil)
	require.NoError(t, err)
	assert.Nil(t, mgrs)
}

func TestInitManagers_DummyDisabled_NotInMap(t *testing.T) {
	cfg := &v2alpha1.CiliumNetworkDriverDeviceManagerConfig{
		Dummy: &v2alpha1.DummyDeviceManagerConfig{Enabled: false, Count: 2},
	}
	mgrs, err := InitManagers(hivetest.Logger(t), cfg)
	require.NoError(t, err)
	assert.NotContains(t, mgrs, types.DeviceManagerTypeDummy)
}

func TestInitManagers_DummyEnabled_InMap(t *testing.T) {
	cfg := &v2alpha1.CiliumNetworkDriverDeviceManagerConfig{
		Dummy: &v2alpha1.DummyDeviceManagerConfig{Enabled: true, Count: 2},
	}
	mgrs, err := InitManagers(hivetest.Logger(t), cfg)
	require.NoError(t, err)
	require.Contains(t, mgrs, types.DeviceManagerTypeDummy)
	assert.Equal(t, types.DeviceManagerTypeDummy, mgrs[types.DeviceManagerTypeDummy].Type())
}

func TestInitManagers_DummyEnabled_NilDummy_NotInMap(t *testing.T) {
	cfg := &v2alpha1.CiliumNetworkDriverDeviceManagerConfig{
		Dummy: nil,
	}
	mgrs, err := InitManagers(hivetest.Logger(t), cfg)
	require.NoError(t, err)
	assert.NotContains(t, mgrs, types.DeviceManagerTypeDummy)
}

func TestInitManagers_DummyNegativeCount_Error(t *testing.T) {
	cfg := &v2alpha1.CiliumNetworkDriverDeviceManagerConfig{
		Dummy: &v2alpha1.DummyDeviceManagerConfig{Enabled: true, Count: -1},
	}
	_, err := InitManagers(hivetest.Logger(t), cfg)
	require.Error(t, err)
}

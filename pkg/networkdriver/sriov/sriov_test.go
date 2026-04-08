// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package sriov

import (
	"fmt"
	"log/slog"
	"maps"
	"path"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
	resourceapi "k8s.io/api/resource/v1"

	"github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	"github.com/cilium/cilium/pkg/networkdriver/types"
)

const (
	testDataPath = "./testdata/"
)

// fakeDriverBinder is an in-memory DriverBinder for use in unit tests.
// It records the calls made to it and simulates driver bind/unbind operations
// without touching sysfs.
type fakeDriverBinder struct {
	// drivers is the set of "loaded" driver names.
	drivers map[string]bool
	// bound is the current driver bound to each PCI address.
	bound map[string]string
	// bindCalls records each (pciAddr, targetDriver) pair passed to BindDriver.
	bindCalls []bindCall
}

type bindCall struct {
	pciAddr      string
	targetDriver string
}

func newFakeDriverBinder(loadedDrivers []string, initialBinding map[string]string) *fakeDriverBinder {
	drivers := make(map[string]bool, len(loadedDrivers))
	for _, d := range loadedDrivers {
		drivers[d] = true
	}
	bound := make(map[string]string, len(initialBinding))
	for k, v := range initialBinding {
		bound[k] = v
	}
	return &fakeDriverBinder{drivers: drivers, bound: bound}
}

func (f *fakeDriverBinder) DriverExists(name string) bool {
	return f.drivers[name]
}

func (f *fakeDriverBinder) CurrentDriver(pciAddr string) (string, error) {
	return f.bound[pciAddr], nil
}

func (f *fakeDriverBinder) BindDriver(pciAddr, targetDriver string) (string, error) {
	if !f.drivers[targetDriver] {
		return "", fmt.Errorf("driver %q not found: module may not be loaded", targetDriver)
	}
	prev := f.bound[pciAddr]
	f.bound[pciAddr] = targetDriver
	f.bindCalls = append(f.bindCalls, bindCall{pciAddr, targetDriver})
	return prev, nil
}

// --------

func compareAttrs(t *testing.T, one, two map[resourceapi.QualifiedName]resourceapi.DeviceAttribute) {
	require.NotEmpty(t, one)
	require.ElementsMatch(t, slices.Collect(maps.Keys(one)), slices.Collect(maps.Keys(two)))

	for k, v := range one {
		require.NotEmpty(t, v.String())
		other := two[k]
		require.Equal(t, v.String(), other.String())
	}
}

func TestSriov(t *testing.T) {

	listLinkFunc := func() ([]netlink.Link, error) {
		return []netlink.Link{
			&netlink.GenericLink{
				LinkType: "device",
				LinkAttrs: netlink.LinkAttrs{
					Name:      "mypf",
					Vfs:       []netlink.VfInfo{{ID: 1}},
					ParentDev: "0000:02:00.0",
				},
			},
			&netlink.GenericLink{
				LinkType: "device",
				LinkAttrs: netlink.LinkAttrs{
					Name:      "myvf",
					ParentDev: "0000:02:00.1",
				},
			},
		}, nil
	}

	var mgr *SRIOVManager
	var err error

	t.Run("test sriov setup on startup", func(t *testing.T) {
		cfg := &v2alpha1.SRIOVDeviceManagerConfig{
			Enabled:           true,
			SysPciDevicesPath: testDataPath,
			Ifaces: []v2alpha1.SRIOVDeviceConfig{
				{IfName: "mypf", VfCount: 1},
			},
		}

		mgr, err = NewManager(slog.Default(), cfg, withNetlinkLister(listLinkFunc))
		require.NoError(t, err)

		// now restore the file
		require.NoError(t, writeVfs(path.Join(mgr.pciDevicesPath(), "0000:02:00.0"), 0))
	})

	t.Run("test device parsing", func(t *testing.T) {

		mgr, err := NewManager(slog.Default(), &v2alpha1.SRIOVDeviceManagerConfig{
			Enabled:           true,
			SysPciDevicesPath: testDataPath,
		}, withNetlinkLister(listLinkFunc))

		require.NoError(t, err)

		byPCI, err := mgr.linkAttrsByPCIAddr()
		require.NoError(t, err)
		require.Contains(t, byPCI, PCIAddr("0000:02:00.1"))
		device, err := mgr.parseDevice("0000:02:00.1", byPCI)
		require.NoError(t, err)
		require.NotNil(t, device)

		expectedDevice := PciDevice{
			Addr:            "0000:02:00.1",
			PfName:          "mypf",
			Driver:          "mydriver",
			VfID:            1,
			KernelIfaceName: "myvf",
			DeviceID:        "mydeviceid",
			Vendor:          "myvendor",
			driverBinder:    device.driverBinder, // not compared by value; just carry through
		}

		require.Equal(t, expectedDevice, *device)
		compareAttrs(t, device.GetAttrs(), expectedDevice.GetAttrs())
	})
}

func TestPciDeviceDriverBinding(t *testing.T) {
	const (
		pciAddr         = "0000:03:00.1"
		kernelDriver    = "mlx5_core"
		userspaceDriver = "vfio-pci"
		pfName          = "eth0"
		vfID            = 0
	)

	makeDevice := func(fb *fakeDriverBinder) *PciDevice {
		return &PciDevice{
			Addr:         pciAddr,
			Driver:       kernelDriver,
			PfName:       pfName,
			VfID:         vfID,
			driverBinder: fb,
		}
	}

	t.Run("Setup with VfDriver binds to userspace driver and saves OriginalDriver", func(t *testing.T) {
		fb := newFakeDriverBinder(
			[]string{kernelDriver, userspaceDriver},
			map[string]string{pciAddr: kernelDriver},
		)
		dev := makeDevice(fb)

		err := dev.Setup(types.DeviceConfig{VfDriver: userspaceDriver})
		require.NoError(t, err)
		require.Equal(t, kernelDriver, dev.OriginalDriver)

		require.Len(t, fb.bindCalls, 1)
		require.Equal(t, bindCall{pciAddr, userspaceDriver}, fb.bindCalls[0])
		require.Equal(t, userspaceDriver, fb.bound[pciAddr])
	})

	t.Run("Setup with VfDriver that does not exist returns error", func(t *testing.T) {
		fb := newFakeDriverBinder(
			[]string{kernelDriver},
			map[string]string{pciAddr: kernelDriver},
		)
		dev := makeDevice(fb)

		err := dev.Setup(types.DeviceConfig{VfDriver: "i40e"})
		require.Error(t, err)
		require.ErrorContains(t, err, "i40e")
		// OriginalDriver must NOT be set on error.
		require.Empty(t, dev.OriginalDriver)
		require.Empty(t, fb.bindCalls)
	})

	t.Run("Setup without VfDriver does not call BindDriver", func(t *testing.T) {
		fb := newFakeDriverBinder(
			[]string{kernelDriver},
			map[string]string{pciAddr: kernelDriver},
		)
		dev := makeDevice(fb)

		err := dev.Setup(types.DeviceConfig{})
		require.NoError(t, err)
		require.Empty(t, dev.OriginalDriver)
		require.Empty(t, fb.bindCalls)
	})

	t.Run("Free with OriginalDriver restores kernel driver and clears OriginalDriver", func(t *testing.T) {
		fb := newFakeDriverBinder(
			[]string{kernelDriver, userspaceDriver},
			map[string]string{pciAddr: userspaceDriver},
		)
		dev := makeDevice(fb)
		dev.OriginalDriver = kernelDriver

		err := dev.Free(types.DeviceConfig{})
		require.NoError(t, err)
		require.Empty(t, dev.OriginalDriver)

		require.Len(t, fb.bindCalls, 1)
		require.Equal(t, bindCall{pciAddr, kernelDriver}, fb.bindCalls[0])
		require.Equal(t, kernelDriver, fb.bound[pciAddr])
	})

	t.Run("Free without OriginalDriver does not call BindDriver", func(t *testing.T) {
		fb := newFakeDriverBinder(
			[]string{kernelDriver},
			map[string]string{pciAddr: kernelDriver},
		)
		dev := makeDevice(fb)
		dev.OriginalDriver = ""

		err := dev.Free(types.DeviceConfig{})
		require.NoError(t, err)
		require.Empty(t, fb.bindCalls)
	})

	t.Run("OriginalDriver is serialized in MarshalBinary and restored via UnmarshalBinary", func(t *testing.T) {
		fb := newFakeDriverBinder(nil, nil)
		dev := &PciDevice{
			Addr:           pciAddr,
			Driver:         userspaceDriver,
			OriginalDriver: kernelDriver,
			driverBinder:   fb, // not serialized
		}

		data, err := dev.MarshalBinary()
		require.NoError(t, err)

		restored := &PciDevice{}
		require.NoError(t, restored.UnmarshalBinary(data))
		require.Equal(t, kernelDriver, restored.OriginalDriver)
		require.Nil(t, restored.driverBinder) // not serialized
	})

	t.Run("RestoreDevice re-injects driverBinder", func(t *testing.T) {
		cfg := &v2alpha1.SRIOVDeviceManagerConfig{
			Enabled:           true,
			SysPciDevicesPath: testDataPath,
		}
		mgr, err := NewManager(slog.Default(), cfg, withNetlinkLister(func() ([]netlink.Link, error) {
			return nil, nil
		}))
		require.NoError(t, err)

		original := &PciDevice{
			Addr:           pciAddr,
			OriginalDriver: kernelDriver,
		}
		data, err := original.MarshalBinary()
		require.NoError(t, err)

		restored, err := mgr.RestoreDevice(data)
		require.NoError(t, err)

		restoredPci, ok := restored.(*PciDevice)
		require.True(t, ok)
		require.NotNil(t, restoredPci.driverBinder)
		require.Equal(t, kernelDriver, restoredPci.OriginalDriver)
	})

	t.Run("Full Setup then Free round-trip restores original driver", func(t *testing.T) {
		fb := newFakeDriverBinder(
			[]string{kernelDriver, userspaceDriver},
			map[string]string{pciAddr: kernelDriver},
		)
		dev := makeDevice(fb)

		require.NoError(t, dev.Setup(types.DeviceConfig{VfDriver: userspaceDriver}))
		require.Equal(t, kernelDriver, dev.OriginalDriver)
		require.Equal(t, userspaceDriver, fb.bound[pciAddr])

		require.NoError(t, dev.Free(types.DeviceConfig{}))
		require.Empty(t, dev.OriginalDriver)
		require.Equal(t, kernelDriver, fb.bound[pciAddr])

		require.Len(t, fb.bindCalls, 2)
		require.Equal(t, bindCall{pciAddr, userspaceDriver}, fb.bindCalls[0])
		require.Equal(t, bindCall{pciAddr, kernelDriver}, fb.bindCalls[1])
	})
}

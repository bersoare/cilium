// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package sriov

import (
	"fmt"
	"os"
	"path"
)

const (
	pciDriversPath = "bus/pci/drivers"
)

// DriverBinder abstracts PCI driver bind/unbind operations so that the real
// sysfs implementation and test fakes can be swapped via dependency injection.
type DriverBinder interface {
	// DriverExists reports whether the named driver is present (i.e. its module
	// is loaded and the sysfs directory exists).
	DriverExists(driverName string) bool

	// CurrentDriver returns the name of the driver currently bound to the PCI
	// device, or "" if no driver is bound.
	CurrentDriver(pciAddr string) (string, error)

	// BindDriver unbinds the PCI device from its current driver (if any) and
	// binds it to targetDriver. It returns the name of the previously bound
	// driver so that callers can restore it later.
	BindDriver(pciAddr, targetDriver string) (previousDriver string, err error)
}

// sysfsDriverBinder is the production DriverBinder that operates on the real
// sysfs tree mounted at sysPath (default: /host/sys).
type sysfsDriverBinder struct {
	sysPath string
}

func newSysfsDriverBinder(sysPath string) DriverBinder {
	return &sysfsDriverBinder{sysPath: sysPath}
}

func (s *sysfsDriverBinder) DriverExists(driverName string) bool {
	_, err := os.Stat(path.Join(s.sysPath, pciDriversPath, driverName))
	return err == nil
}

func (s *sysfsDriverBinder) CurrentDriver(pciAddr string) (string, error) {
	driverLink := path.Join(s.sysPath, pciDevicesPath, pciAddr, "driver")

	target, err := os.Readlink(driverLink)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading driver symlink for %s: %w", pciAddr, err)
	}

	return path.Base(target), nil
}

func (s *sysfsDriverBinder) BindDriver(pciAddr, targetDriver string) (string, error) {
	if !s.DriverExists(targetDriver) {
		return "", fmt.Errorf("driver %q not found in %s: module may not be loaded",
			targetDriver, path.Join(s.sysPath, pciDriversPath))
	}

	previousDriver, err := s.unbind(pciAddr)
	if err != nil {
		return "", fmt.Errorf("unbinding current driver before binding %s: %w", targetDriver, err)
	}

	bindPath := path.Join(s.sysPath, pciDriversPath, targetDriver, "bind")
	if err := os.WriteFile(bindPath, []byte(pciAddr), 0o200); err != nil {
		return "", fmt.Errorf("binding %s to driver %s: %w", pciAddr, targetDriver, err)
	}

	return previousDriver, nil
}

// unbind unbinds the PCI device from its current driver and returns the
// driver name that was unbound, or "" if no driver was bound.
func (s *sysfsDriverBinder) unbind(pciAddr string) (string, error) {
	driver, err := s.CurrentDriver(pciAddr)
	if err != nil {
		return "", err
	}

	if driver == "" {
		return "", nil
	}

	unbindPath := path.Join(s.sysPath, pciDevicesPath, pciAddr, "driver", "unbind")
	if err := os.WriteFile(unbindPath, []byte(pciAddr), 0o200); err != nil {
		return "", fmt.Errorf("unbinding driver %s from %s: %w", driver, pciAddr, err)
	}

	return driver, nil
}

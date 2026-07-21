# Cilium Network Driver

Cilium Network Driver allows cilium-agent to expose network devices directly
to pods, without those pods participating in the Cilium fabric. The driver
registers as a
[DRA](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/)
plugin and publishes `ResourceSlice` resources to the Kubernetes API so pods
can claim devices via the standard DRA framework.

## Requirements

- Kubernetes v1.34+
- Cilium agent with `--enable-cilium-network-driver` (set automatically
  when the Helm flag is enabled)

## Use cases

Applications that need direct network device access on a separate network
plane from the Cilium-managed pod network and/or physical device
hand-off from the host, such as:

- DPDK-based applications (VNFs, packet-processing pipelines)
- High-frequency trading or other low-latency workloads

## Device Managers

A Device Manager implements the `types.DeviceManager` interface and is
responsible for discovering and lifecycle-managing a class of network device.
Available device managers:

| Manager  | Key in CRD        | Devices managed                        |
|----------|-------------------|----------------------------------------|
| `sriov`  | `sriov`           | SR-IOV Virtual Functions (legacy VFs)  |
| `dummy`  | `dummy`           | Linux dummy interfaces                 |

## How to use the Network Driver

### 1. Enable the feature

The DRA framework, NRI (CRI integration hook), and device discovery require
host mounts that are not needed by any other Cilium feature. The Network
Driver must therefore be explicitly enabled:

```bash
helm upgrade cilium cilium/cilium \
  --set networkDriver.enabled=true
```

This sets `--enable-cilium-network-driver` on the agent.

### 2. Provide a node configuration

The agent reads its configuration from a `CiliumNetworkDriverNodeConfig` CRD
whose `metadata.name` matches the node hostname.

**Minimal example — dummy devices (3 devices):**

```yaml
apiVersion: cilium.io/v2alpha1
kind: CiliumNetworkDriverNodeConfig
metadata:
  name: worker-node-1        # must match node hostname
spec:
  driverName: "networkdriver.cilium.io"  # optional; this is the default
  deviceManagerConfigs:
    dummy:
      enabled: true
      count: 3          # number of dummy links to create and advertise
  pools:
    - name: fast-net
      filter:
        deviceManagers:
          - dummy
```

**SR-IOV example — 4 VFs on ens1f0:**

```yaml
apiVersion: cilium.io/v2alpha1
kind: CiliumNetworkDriverNodeConfig
metadata:
  name: worker-node-1
spec:
  driverName: "networkdriver.cilium.io"
  deviceManagerConfigs:
    sriov:
      enabled: true
      ifaces:
        - ifName: ens1f0
          vfCount: 4
  pools:
    - name: sriov-pool
      filter:
        pfNames:
          - ens1f0
```

#### Pool filters

Pools group devices that share a common purpose. Only devices matched by
the pool's filter are advertised in the corresponding `ResourceSlice`.
All specified filter fields are ANDed together.

| Filter field     | SR-IOV                                                                                  | Dummy                                   |
|------------------|-----------------------------------------------------------------------------------------|-----------------------------------------|
| `deviceManagers` | Match when set to `sr-iov`                                                              | Match when set to `dummy`               |
| `ifNames`        | Kernel interface name of the VF (empty for DPDK/vfio devices — use `pciAddrs` instead) | Kernel interface name of the dummy link |
| `pfNames`        | Physical Function kernel interface name                                                 | Ignored — dummy devices always match    |
| `parentIfNames`  | Ignored — SR-IOV devices always match                                                   | Not applicable (non-empty → no match)   |
| `pciAddrs`       | PCI address of the VF (e.g. `0000:03:00.1`)                                            | Not applicable (non-empty → no match)   |
| `vendorIDs`      | PCI vendor ID                                                                           | Not applicable (non-empty → no match)   |
| `deviceIDs`      | PCI device ID                                                                           | Not applicable (non-empty → no match)   |
| `drivers`        | Kernel driver bound to the VF (e.g. `mlx5_core`, `vfio-pci`)                          | Not applicable (non-empty → no match)   |

#### Filter conflict rules

Filters are validated at configuration load time and enforced at runtime.

**Config-time validation** rejects a configuration with duplicate pool names or
where the same `ifNames` value appears across more than one pool, since that
field uniquely identifies a single device.

**Runtime conflict resolution** handles cases where a device matches more than
one pool despite passing config-time validation (e.g. when pools overlap via
`pfNames`, `drivers`, or `vendorIDs`). The device is assigned to exactly one
pool using the following priority:

1. **Previous assignment** — if the device was assigned to a pool in a prior
   reconcile cycle and that pool still matches the device, the assignment is
   kept unchanged. This ensures stability across reconcile cycles.
2. **Alphabetically first matching pool** — deterministic tie-break for devices
   that have no prior assignment.

An error is logged whenever a device matches more than one pool.

#### Device configuration options

Device-specific configuration is passed as opaque parameters in the
`ResourceClaim` (see step 3). Supported fields (from `types/types.go`):

| Field           | Type     | Description                                              |
|-----------------|----------|----------------------------------------------------------|
| `vlan`          | `int32`  | 802.1q VLAN ID to configure on the device (SR-IOV only)  |
| `podIfName`     | `string` | Rename the interface inside the pod namespace            |

### 3. Prepare device requests

Create a `DeviceClass` to encapsulate device selection logic:

```yaml
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: sriov-pool.networkdriver.cilium.io
spec:
  selectors:
  - cel:
      expression: >
        device.driver == "networkdriver.cilium.io" &&
        device.attributes["networkdriver.cilium.io"].pool == "sriov-pool"
```

Create a `ResourceClaimTemplate` that references the class and passes device
configuration as opaque parameters:

```yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: sriov-claim
spec:
  spec:
    devices:
      requests:
      - name: net
        exactly:
          deviceClassName: sriov-pool.networkdriver.cilium.io
      config:
      - requests:
          - net
        opaque:
          driver: networkdriver.cilium.io
          parameters:
            vlan: 1001
            podIfName: sriov0
```

Alternatively, skip the `DeviceClass` and match directly via CEL:

```yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: sriov-claim-direct
spec:
  spec:
    devices:
      requests:
      - name: net
        exactly:
          selectors:
          - cel:
              expression: >
                device.driver == "networkdriver.cilium.io" &&
                device.attributes["networkdriver.cilium.io"].pool == "sriov-pool"
      config:
      - requests:
          - net
        opaque:
          driver: networkdriver.cilium.io
          parameters:
            podIfName: sriov0
            vlan: 1001
```

### 4. Request a device from a pod

Reference the `ResourceClaimTemplate` in the pod spec:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: dpdk-app
spec:
  resourceClaims:
  - name: net
    resourceClaimTemplateName: sriov-claim-direct
  containers:
  - name: app
    image: my-dpdk-app:latest
```

## Verifying the setup

### Check node configuration was applied

```bash
# List all per-node configurations
kubectl get ciliumnetworkdrivernodeconfigs

# Inspect the configuration for a specific node
kubectl get ciliumnetworkdrivernodeconfig worker-node-1 -o yaml
```

### Verify published devices (ResourceSlices)

```bash
# List all ResourceSlices published by the network driver
# note: the `driver=` parameter should match the driver name
# from CiliumNetworkDriverNodeConfig
kubectl get resourceslice -l resource.kubernetes.io/driver=networkdriver.cilium.io

# Or for another DRA driver
kubectl get resourceslice -l resource.kubernetes.io/driver=<name>

# Or for all DRA drivers
kubectl get resourceslice

# Inspect a specific slice
kubectl get resourceslice <name> -o yaml
```

Example output:
```
NAME                                                  NODE           DRIVER                         POOL         AGE
worker-node-1-networkdriver.cilium.io-abc12       worker-node-1  networkdriver.cilium.io    sriov-pool   30s
```

### Verify ResourceClaims and allocations

```bash
# List all resource claims
kubectl get resourceclaims -A

# Check claim status (allocated, reserved, device status)
kubectl get resourceclaim <name> -n <namespace> -o yaml

# List claim templates
kubectl get resourceclaimtemplates -A
```

### Verify DeviceClasses

```bash
kubectl get deviceclasses
```

## Feature status

Experimental. The API and configuration format may change between releases.
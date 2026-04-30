# libvirt backend

This backend provisions the same baseline cluster shape on libvirt instead of Multipass.

## Scope

- first usable version, not full feature parity with the multipass path
- supports `1 control-plane + N workers`
- uses a dir storage pool, Ubuntu cloud image, cloud-init seed ISO, and DHCP on an existing libvirt network
- bootstraps kubeadm through SSH after the guests receive DHCP leases
- waits until all joined nodes report `Ready` before finishing

## Required variables

- `ssh_private_key_path`
- `ssh_public_key`

## Common defaults

- `libvirt_uri = "qemu:///system"`
- `libvirt_network_name = "default"`
- `libvirt_pool_name = "infra-lab"`
- `libvirt_pool_path = "/var/lib/libvirt/images/infra-lab"`

## Runtime notes

- The current implementation expects a libvirt network with DHCP, typically `default`.
- Guest network naming is left to Ubuntu and libvirt; the backend does not assume `eth0`.
- If `libvirt_uri = "qemu:///system"`, you may need to run `./scripts/k8s-tool.sh up` with `sudo` unless the host user already has libvirt manage permissions.
- Validation and live smoke tests were run on `seoy@100.123.80.48`.

## Example

```bash
BACKEND=libvirt \
TF_VAR_libvirt_uri=qemu:///system \
TF_VAR_ssh_private_key_path=/home/seoy/.ssh/id_ed25519 \
TF_VAR_ssh_public_key="$(cat /home/seoy/.ssh/id_ed25519.pub)" \
./scripts/k8s-tool.sh up
```

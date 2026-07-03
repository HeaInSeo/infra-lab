# tori Lustre Lab Infrastructure

This OpenTofu root module declares the isolated single-node Lustre lab VM.
It creates the libvirt pool, Rocky cloud-image clone, 80 GiB dedicated Lustre
target disk, private-network VM, and cloud-init SSH bootstrap only.

It deliberately does not install or configure Lustre. Server package build,
MGS/MDT/OST formatting, and client mount verification remain a separately
versioned provisioning step after the VM contract is stable.

## Usage

Run this module on the libvirt host, not from the tori development checkout.
Provide a local `terraform.tfvars` file (never commit it):

```hcl
base_image_url = "https://download.rockylinux.org/pub/rocky/8/images/x86_64/Rocky-8-GenericCloud-Base.latest.x86_64.qcow2"
ssh_public_key = "ssh-ed25519 ..."
```

Verify the base image checksum before use. Then run `tofu init`,
`tofu plan`, and `tofu apply`.

Before applying, ensure `pool_path` is approved for VM images. This module
does not manage existing manually-created VMs; importing or replacing the
current `tori-lustre-lab` requires an explicit migration plan.

resource "libvirt_pool" "lustre_lab" {
  name = "tori-lustre-lab-pool"
  type = "dir"
  target { path = var.pool_path }
}

resource "libvirt_volume" "base" {
  name   = "Rocky-8-GenericCloud-Base.qcow2"
  pool   = libvirt_pool.lustre_lab.name
  source = var.base_image_url
  format = "qcow2"
}

resource "libvirt_volume" "os" {
  name           = "tori-lustre-lab-os.qcow2"
  pool           = libvirt_pool.lustre_lab.name
  base_volume_id = libvirt_volume.base.id
  size           = 32212254720
}

resource "libvirt_volume" "lustre_target" {
  name   = "tori-lustre-lab-lustre-target.qcow2"
  pool   = libvirt_pool.lustre_lab.name
  format = "qcow2"
  size   = 85899345920
}

resource "libvirt_cloudinit_disk" "seed" {
  name      = "tori-lustre-lab-seed.iso"
  pool      = libvirt_pool.lustre_lab.name
  user_data = templatefile("${path.module}/cloud-init.yaml.tftpl", { ssh_public_key = var.ssh_public_key })
  meta_data = yamlencode({ instance-id = "tori-lustre-lab", local-hostname = "tori-lustre-lab" })
}

resource "libvirt_domain" "lustre_lab" {
  name   = "tori-lustre-lab"
  memory = 8192
  vcpu   = 4

  cloudinit = libvirt_cloudinit_disk.seed.id

  cpu { mode = "host-passthrough" }

  disk { volume_id = libvirt_volume.os.id }
  disk { volume_id = libvirt_volume.lustre_target.id }

  network_interface { network_name = "default" }
}

locals {
  memory_mib = tonumber(replace(replace(var.vm_memory, "G", ""), "M", "")) * (endswith(var.vm_memory, "G") ? 1024 : 1)
  disk_mib   = tonumber(replace(replace(var.vm_disk, "G", ""), "M", "")) * (endswith(var.vm_disk, "G") ? 1024 : 1)
}

resource "libvirt_pool" "lab" {
  name = var.libvirt_pool_name
  type = "dir"

  target = {
    path = var.libvirt_pool_path
  }

  create = {
    build     = true
    start     = true
    autostart = true
  }
}

resource "libvirt_volume" "ubuntu_base" {
  name = var.libvirt_base_image_name
  pool = libvirt_pool.lab.name

  target = {
    format = {
      type = "qcow2"
    }
  }

  create = {
    content = {
      url = var.libvirt_base_image_url
    }
  }
}

resource "libvirt_volume" "vm_disk" {
  name     = "${var.env_name}.qcow2"
  pool     = libvirt_pool.lab.name
  capacity = local.disk_mib * 1024 * 1024

  target = {
    format = {
      type = "qcow2"
    }
  }

  backing_store = {
    path = libvirt_volume.ubuntu_base.path
    format = {
      type = "qcow2"
    }
  }
}

resource "libvirt_cloudinit_disk" "seed" {
  name = "${var.env_name}-seed"
  user_data = templatefile("${path.module}/templates/user-data.yaml.tftpl", {
    ssh_public_key = var.ssh_public_key
    vm_user        = var.vm_user
    workspace_path = var.workspace_path
    workspace_dirs = var.workspace_dirs
  })
  meta_data = yamlencode({
    instance-id    = var.env_name
    local-hostname = var.env_name
  })
}

resource "libvirt_volume" "seed" {
  name = "${var.env_name}-seed.iso"
  pool = libvirt_pool.lab.name

  create = {
    content = {
      url = libvirt_cloudinit_disk.seed.path
    }
  }
}

resource "libvirt_domain" "vm" {
  name        = var.env_name
  type        = "kvm"
  memory      = local.memory_mib
  memory_unit = "MiB"
  vcpu        = var.vm_cpus
  running     = true

  cpu = {
    mode = "host-passthrough"
  }

  os = {
    type         = "hvm"
    type_arch    = "x86_64"
    type_machine = "q35"
    boot_devices = [
      {
        dev = "hd"
      }
    ]
  }

  devices = {
    disks = [
      {
        source = {
          volume = {
            pool   = libvirt_volume.vm_disk.pool
            volume = libvirt_volume.vm_disk.name
          }
        }
        target = {
          bus = "virtio"
          dev = "vda"
        }
        driver = {
          type = "qcow2"
        }
      },
      {
        device = "cdrom"
        source = {
          volume = {
            pool   = libvirt_volume.seed.pool
            volume = libvirt_volume.seed.name
          }
        }
        target = {
          bus = "sata"
          dev = "sda"
        }
      }
    ]

    interfaces = [
      {
        type  = "network"
        model = { type = "virtio" }
        source = {
          network = {
            network = var.libvirt_network_name
          }
        }
        wait_for_ip = {
          source  = "lease"
          timeout = 300
        }
      }
    ]

    consoles = [
      {
        type = "pty"
        target = {
          type = "serial"
          port = "0"
        }
      }
    ]
  }
}

data "libvirt_domain_interface_addresses" "vm" {
  domain = libvirt_domain.vm.name
  source = "lease"
}

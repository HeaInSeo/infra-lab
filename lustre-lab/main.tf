resource "libvirt_pool" "lustre_lab" {
  name = "tori-lustre-lab-pool"
  type = "dir"
  target = {
    path = var.pool_path
  }
}

resource "libvirt_volume" "base" {
  name = "Rocky-8-GenericCloud-Base.qcow2"
  pool = libvirt_pool.lustre_lab.name
  target = {
    format = {
      type = "qcow2"
    }
  }
  create = {
    content = {
      url = var.base_image_url
    }
  }
}

resource "libvirt_volume" "os" {
  name     = "tori-lustre-lab-os.qcow2"
  pool     = libvirt_pool.lustre_lab.name
  capacity = 32212254720
  target = {
    format = {
      type = "qcow2"
    }
  }
  backing_store = {
    path = libvirt_volume.base.path
    format = {
      type = "qcow2"
    }
  }
}

resource "libvirt_volume" "lustre_target" {
  name     = "tori-lustre-lab-lustre-target.qcow2"
  pool     = libvirt_pool.lustre_lab.name
  capacity = 85899345920
  target = {
    format = {
      type = "qcow2"
    }
  }
}

resource "libvirt_cloudinit_disk" "seed" {
  name      = "tori-lustre-lab-seed.iso"
  user_data = templatefile("${path.module}/cloud-init.yaml.tftpl", { ssh_public_key = var.ssh_public_key })
  meta_data = yamlencode({ instance-id = "tori-lustre-lab", local-hostname = "tori-lustre-lab" })
}

resource "libvirt_volume" "seed" {
  name = "tori-lustre-lab-seed.iso"
  pool = libvirt_pool.lustre_lab.name
  create = {
    content = {
      url = libvirt_cloudinit_disk.seed.path
    }
  }
}

resource "libvirt_domain" "lustre_lab" {
  name   = "tori-lustre-lab"
  memory = 8388608
  vcpu   = 4
  type   = "kvm"

  cpu = {
    mode = "host-passthrough"
  }
  os = {
    type         = "hvm"
    type_arch    = "x86_64"
    type_machine = "q35"
  }
  devices = {
    disks = [
      {
        source = {
          volume = {
            pool   = libvirt_volume.os.pool
            volume = libvirt_volume.os.name
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
        source = {
          volume = {
            pool   = libvirt_volume.lustre_target.pool
            volume = libvirt_volume.lustre_target.name
          }
        }
        target = {
          bus = "virtio"
          dev = "vdb"
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
        type = "network"
        source = {
          network = {
            network = "default"
          }
        }
        model = {
          type = "virtio"
        }
      }
    ]
  }
}

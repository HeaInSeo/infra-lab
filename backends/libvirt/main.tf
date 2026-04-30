provider "libvirt" {
  uri = var.libvirt_uri
}

locals {
  root_dir = abspath("${path.module}/../..")

  master_memory_mib = tonumber(replace(replace(var.master_memory, "G", ""), "M", "")) * (endswith(var.master_memory, "G") ? 1024 : 1)
  worker_memory_mib = tonumber(replace(replace(var.worker_memory, "G", ""), "M", "")) * (endswith(var.worker_memory, "G") ? 1024 : 1)
  master_disk_mib   = tonumber(replace(replace(var.master_disk, "G", ""), "M", "")) * (endswith(var.master_disk, "G") ? 1024 : 1)
  worker_disk_mib   = tonumber(replace(replace(var.worker_disk, "G", ""), "M", "")) * (endswith(var.worker_disk, "G") ? 1024 : 1)

  nodes = merge(
    {
      for i in range(var.masters) :
      "${var.name_prefix}-master-${i}" => {
        role       = "master"
        memory_mib = local.master_memory_mib
        disk_bytes = local.master_disk_mib * 1024 * 1024
        vcpu       = var.master_cpus
      }
    },
    {
      for i in range(var.workers) :
      "${var.name_prefix}-worker-${i}" => {
        role       = "worker"
        memory_mib = local.worker_memory_mib
        disk_bytes = local.worker_disk_mib * 1024 * 1024
        vcpu       = var.worker_cpus
      }
    }
  )

  worker_node_names = [
    for name, node in local.nodes : name
    if node.role == "worker"
  ]

  extra_master_node_names = [
    for name, node in local.nodes : name
    if node.role == "master" && name != "${var.name_prefix}-master-0"
  ]
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

resource "libvirt_volume" "node_disk" {
  for_each = local.nodes

  name = "${each.key}.qcow2"
  pool = libvirt_pool.lab.name

  target = {
    format = {
      type = "qcow2"
    }
  }

  capacity = each.value.disk_bytes

  backing_store = {
    path = libvirt_volume.ubuntu_base.path
    format = {
      type = "qcow2"
    }
  }
}

resource "libvirt_cloudinit_disk" "node_seed" {
  for_each = local.nodes

  name = "${each.key}-seed"
  user_data = templatefile("${path.module}/templates/user-data.yaml.tftpl", {
    base_user_data = file("${local.root_dir}/cloud-init/k8s.yaml")
    ssh_public_key = var.ssh_public_key
  })
  meta_data = yamlencode({
    instance-id    = each.key
    local-hostname = each.key
  })
}

resource "libvirt_volume" "node_seed_volume" {
  for_each = local.nodes

  name = "${each.key}-seed.iso"
  pool = libvirt_pool.lab.name

  create = {
    content = {
      url = libvirt_cloudinit_disk.node_seed[each.key].path
    }
  }
}

resource "libvirt_domain" "node" {
  for_each = local.nodes

  name        = each.key
  type        = "kvm"
  memory      = each.value.memory_mib
  memory_unit = "MiB"
  vcpu        = each.value.vcpu
  running     = true

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
            pool   = libvirt_volume.node_disk[each.key].pool
            volume = libvirt_volume.node_disk[each.key].name
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
            pool   = libvirt_volume.node_seed_volume[each.key].pool
            volume = libvirt_volume.node_seed_volume[each.key].name
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

data "libvirt_domain_interface_addresses" "node" {
  for_each = local.nodes

  domain = libvirt_domain.node[each.key].name
  source = "lease"
}

resource "null_resource" "init_cluster" {
  depends_on = [libvirt_domain.node]

  triggers = {
    master0_ip = one([
      for addr in data.libvirt_domain_interface_addresses.node["${var.name_prefix}-master-0"].interfaces[0].addrs :
      addr.addr if addr.type == "ipv4"
    ])
    vm_user = var.vm_user
  }

  provisioner "local-exec" {
    working_dir = local.root_dir
    command     = <<-EOT
      set -e
      VM_RUNTIME=ssh VM_USER="${var.vm_user}" SSH_PRIVATE_KEY_PATH="${var.ssh_private_key_path}" \
        bash scripts/runtime/run-remote.sh "${self.triggers.master0_ip}" "scripts/cluster/cluster-init.sh" "/home/${var.vm_user}/cluster-init.sh"
    EOT
  }
}

resource "null_resource" "join_all" {
  depends_on = [null_resource.init_cluster]

  triggers = {
    master0_ip = one([
      for addr in data.libvirt_domain_interface_addresses.node["${var.name_prefix}-master-0"].interfaces[0].addrs :
      addr.addr if addr.type == "ipv4"
    ])
    worker_ips = join(",", [
      for name in local.worker_node_names :
      one([
        for addr in data.libvirt_domain_interface_addresses.node[name].interfaces[0].addrs :
        addr.addr if addr.type == "ipv4"
      ])
    ])
    master_ips = join(",", [
      for name in local.extra_master_node_names :
      one([
        for addr in data.libvirt_domain_interface_addresses.node[name].interfaces[0].addrs :
        addr.addr if addr.type == "ipv4"
      ])
    ])
    kubeconfig_path = var.kubeconfig_path
    vm_user         = var.vm_user
  }

  provisioner "local-exec" {
    working_dir = local.root_dir
    command     = <<-EOT
      set -e
      VM_RUNTIME=ssh VM_USER="${var.vm_user}" SSH_PRIVATE_KEY_PATH="${var.ssh_private_key_path}" \
        MASTER0_ENDPOINT="${self.triggers.master0_ip}" MASTER_ENDPOINTS="${self.triggers.master_ips}" WORKER_ENDPOINTS="${self.triggers.worker_ips}" \
        KUBECONFIG_PATH="${var.kubeconfig_path}" bash scripts/cluster/join-all.sh
    EOT
  }
}

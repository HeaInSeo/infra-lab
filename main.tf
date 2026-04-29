locals {
  k8s_cloud_init_sha = filesha256("${path.module}/cloud-init/k8s.yaml")
  cluster_init_sha   = filesha256("${path.module}/scripts/cluster/cluster-init.sh")
  join_all_sha       = filesha256("${path.module}/scripts/cluster/join-all.sh")
  run_remote_sha     = filesha256("${path.module}/scripts/multipass/multipass-run-remote.sh")
}

resource "null_resource" "masters" {
  count = var.masters

  triggers = {
    name           = "${var.name_prefix}-master-${count.index}"
    image          = var.multipass_image
    mem            = var.master_memory
    cpus           = tostring(var.master_cpus)
    disk           = var.master_disk
    cloud_init_sha = local.k8s_cloud_init_sha
  }

  provisioner "local-exec" {
    working_dir = path.module
    command     = <<EOT
set -e
RECREATE_ON_DIFF=${var.recreate_on_diff ? 1 : 0} bash scripts/multipass/multipass-launch.sh "${self.triggers.name}" "${self.triggers.image}" "${self.triggers.mem}" "${self.triggers.disk}" "${self.triggers.cpus}" "cloud-init/k8s.yaml"
EOT
  }

  provisioner "local-exec" {
    when        = destroy
    working_dir = path.module
    command     = <<EOT
bash scripts/multipass/multipass-delete.sh "${self.triggers.name}" || true
EOT
  }
}

resource "null_resource" "workers" {
  depends_on = [null_resource.masters]
  count      = var.workers

  triggers = {
    name           = "${var.name_prefix}-worker-${count.index}"
    image          = var.multipass_image
    mem            = var.worker_memory
    cpus           = tostring(var.worker_cpus)
    disk           = var.worker_disk
    cloud_init_sha = local.k8s_cloud_init_sha
  }

  provisioner "local-exec" {
    working_dir = path.module
    command     = <<EOT
set -e
RECREATE_ON_DIFF=${var.recreate_on_diff ? 1 : 0} bash scripts/multipass/multipass-launch.sh "${self.triggers.name}" "${self.triggers.image}" "${self.triggers.mem}" "${self.triggers.disk}" "${self.triggers.cpus}" "cloud-init/k8s.yaml"
EOT
  }

  provisioner "local-exec" {
    when        = destroy
    working_dir = path.module
    command     = <<EOT
bash scripts/multipass/multipass-delete.sh "${self.triggers.name}" || true
EOT
  }
}

resource "null_resource" "init_cluster" {
  depends_on = [null_resource.masters]

  triggers = {
    script_sha   = local.cluster_init_sha
    run_sha      = local.run_remote_sha
    name_prefix  = var.name_prefix
    masters      = tostring(var.masters)
    master0_name = "${var.name_prefix}-master-0"
    vm_user      = var.vm_user
    cloud_init   = local.k8s_cloud_init_sha
    image        = var.multipass_image
    master_mem   = var.master_memory
    master_cpus  = tostring(var.master_cpus)
    master_disk  = var.master_disk
  }

  provisioner "local-exec" {
    working_dir = path.module
    command     = <<EOT
set -e
VM_USER="${var.vm_user}" bash scripts/multipass/multipass-run-remote.sh "${var.name_prefix}-master-0" "scripts/cluster/cluster-init.sh" "/home/${var.vm_user}/cluster-init.sh"
EOT
  }
}

resource "null_resource" "join_all" {
  depends_on = [null_resource.workers, null_resource.init_cluster]

  triggers = {
    script_sha       = local.join_all_sha
    name_prefix      = var.name_prefix
    masters          = tostring(var.masters)
    workers          = tostring(var.workers)
    kubeconfig       = var.kubeconfig_path
    vm_user          = var.vm_user
    cloud_init       = local.k8s_cloud_init_sha
    image            = var.multipass_image
    master_mem       = var.master_memory
    master_cpus      = tostring(var.master_cpus)
    master_disk      = var.master_disk
    worker_mem       = var.worker_memory
    worker_cpus      = tostring(var.worker_cpus)
    worker_disk      = var.worker_disk
    recreate_on_diff = tostring(var.recreate_on_diff)
  }

  provisioner "local-exec" {
    working_dir = path.module
    command     = <<EOT
set -e
NAME_PREFIX="${var.name_prefix}" MASTERS="${var.masters}" WORKERS="${var.workers}" KUBECONFIG_PATH="${var.kubeconfig_path}" VM_USER="${var.vm_user}" bash scripts/cluster/join-all.sh
EOT
  }
}

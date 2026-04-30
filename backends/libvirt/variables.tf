variable "name_prefix" {
  description = "VM name prefix"
  type        = string
  default     = "lab"
}

variable "masters" {
  description = "Number of control-plane nodes"
  type        = number
  default     = 1
}

variable "workers" {
  description = "Number of worker nodes"
  type        = number
  default     = 2
}

variable "master_memory" {
  description = "Memory for control-plane nodes"
  type        = string
  default     = "4G"
}

variable "worker_memory" {
  description = "Memory for worker nodes"
  type        = string
  default     = "4G"
}

variable "master_cpus" {
  description = "vCPU count for control-plane nodes"
  type        = number
  default     = 2
}

variable "worker_cpus" {
  description = "vCPU count for worker nodes"
  type        = number
  default     = 2
}

variable "master_disk" {
  description = "Disk size for control-plane nodes"
  type        = string
  default     = "40G"
}

variable "worker_disk" {
  description = "Disk size for worker nodes"
  type        = string
  default     = "50G"
}

variable "kubeconfig_path" {
  description = "Local kubeconfig export path"
  type        = string
  default     = "../../kubeconfig.libvirt"
}

variable "vm_user" {
  description = "Default guest user"
  type        = string
  default     = "ubuntu"
}

variable "multipass_image" {
  description = "Legacy compatibility variable from the multipass backend; ignored by the libvirt backend."
  type        = string
  default     = "24.04"
}

variable "libvirt_uri" {
  description = "libvirt connection URI"
  type        = string
  default     = "qemu:///system"
}

variable "libvirt_pool_name" {
  description = "Storage pool name used for lab volumes"
  type        = string
  default     = "infra-lab"
}

variable "libvirt_pool_path" {
  description = "Filesystem path for the libvirt dir pool"
  type        = string
  default     = "/var/lib/libvirt/images/infra-lab"
}

variable "libvirt_network_name" {
  description = "Existing libvirt network name used for DHCP-based node addresses"
  type        = string
  default     = "default"
}

variable "libvirt_base_image_name" {
  description = "Storage volume name for the shared Ubuntu cloud image"
  type        = string
  default     = "ubuntu-24.04-base.qcow2"
}

variable "libvirt_base_image_url" {
  description = "Ubuntu cloud image URL used as the base qcow2 volume"
  type        = string
  default     = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
}

variable "ssh_private_key_path" {
  description = "Private key path used for SSH bootstrap into libvirt guests"
  type        = string
}

variable "ssh_public_key" {
  description = "Public key content injected into libvirt guests for SSH bootstrap"
  type        = string
}

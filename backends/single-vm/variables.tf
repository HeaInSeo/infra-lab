variable "env_name" {
  description = "Single VM environment and domain name"
  type        = string
}

variable "vm_user" {
  description = "Default guest user"
  type        = string
  default     = "ubuntu"
}

variable "vm_cpus" {
  description = "vCPU count"
  type        = number
  default     = 2
}

variable "vm_memory" {
  description = "Guest memory, e.g. 8G or 8192M"
  type        = string
  default     = "4G"
}

variable "vm_disk" {
  description = "Guest disk size, e.g. 80G"
  type        = string
  default     = "40G"
}

variable "workspace_path" {
  description = "Remote workspace path"
  type        = string
}

variable "workspace_dirs" {
  description = "Workspace subdirectories to create"
  type        = list(string)
  default     = []
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
  description = "Filesystem path used when single-vm creates the libvirt dir pool"
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

variable "ssh_public_key" {
  description = "Public key content injected into the guest"
  type        = string
}

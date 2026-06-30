variable "libvirt_uri" {
  type    = string
  default = "qemu:///system"
}

variable "pool_path" {
  type    = string
  default = "/data500/libvirt/tori-lustre-lab"
}

variable "base_image_url" {
  type = string
}

variable "base_image_sha256" {
  type = string
}

variable "ssh_public_key" {
  type      = string
  sensitive = true
}

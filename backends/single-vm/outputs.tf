output "domain_name" {
  value = libvirt_domain.vm.name
}

output "ipv4" {
  value = one([
    for addr in data.libvirt_domain_interface_addresses.vm.interfaces[0].addrs :
    addr.addr if addr.type == "ipv4"
  ])
}

output "ssh_user" {
  value = var.vm_user
}

output "workspace_path" {
  value = var.workspace_path
}

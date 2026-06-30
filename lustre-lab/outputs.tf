output "domain_name" { value = libvirt_domain.lustre_lab.name }
output "lustre_target_volume" { value = libvirt_volume.lustre_target.id }

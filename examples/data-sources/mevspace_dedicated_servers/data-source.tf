data "mevspace_dedicated_servers" "all" {}

output "servers" {
  value = { for s in data.mevspace_dedicated_servers.all.items : s.id => s.attributes.status }
}

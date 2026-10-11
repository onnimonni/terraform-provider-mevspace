data "mevspace_templates" "dedicated" {
  type = "dedicated"
}

output "template_names" {
  value = [for t in data.mevspace_templates.dedicated.items : "${t.id}: ${t.name}"]
}

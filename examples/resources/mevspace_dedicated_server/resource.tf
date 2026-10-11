data "mevspace_templates" "dedicated" {
  type = "dedicated"
}

locals {
  debian13 = one([for t in data.mevspace_templates.dedicated.items : t.id if t.name == "Debian 13"])
}

# Server must already be ordered. Setting template_id REINSTALLS (wipes) the server.
resource "mevspace_dedicated_server" "web" {
  hostname    = "srv12345.mevspace.com"
  template_id = local.debian13
  raid_value  = "1"
  power_state = "running"

  reinstall_triggers = {
    version = "1"
  }
}

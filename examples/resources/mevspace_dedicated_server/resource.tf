data "mevspace_templates" "dedicated" {
  type = "dedicated"
}

locals {
  debian13 = one([for t in data.mevspace_templates.dedicated.items : t.id if t.name == "Debian 13"])
}

# Adopt an already ordered server without touching it.
resource "mevspace_dedicated_server" "db" {
  hostname = "srv11111.mevnode.com"
}

# Install an OS. Any plan that would reinstall (and WIPE) the server
# fails unless allow_reinstall = true.
resource "mevspace_dedicated_server" "web" {
  hostname        = "srv12345.mevnode.com"
  template_id     = local.debian13
  raid_value      = "1"
  allow_reinstall = true
  power_state     = "running"

  reinstall_triggers = {
    version = "1"
  }
}

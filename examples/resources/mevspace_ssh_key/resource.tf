resource "mevspace_ssh_key" "me" {
  label = "laptop"
  value = file("~/.ssh/id_ed25519.pub")
}

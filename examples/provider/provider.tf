terraform {
  required_providers {
    mevspace = {
      source = "onnimonni/mevspace"
    }
  }
}

# Token from https://panel.mevspace.com, or set MEVSPACE_API_TOKEN
provider "mevspace" {}

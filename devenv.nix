{ pkgs, ... }:

{
  packages = [
    pkgs.git
    pkgs.opentofu
    pkgs.golangci-lint
    pkgs.goreleaser
    pkgs.sops
    pkgs.age
    pkgs.gnupg
    pkgs.terraform-plugin-docs
    pkgs.jq
  ];

  languages.go.enable = true;

  # Regenerate docs/ for the registry from the local build (no Terraform download).
  scripts.gen-docs.exec = ''
    set -euo pipefail
    root=$(git rev-parse --show-toplevel)
    tmp=$(mktemp -d)
    trap 'rm -r -- "$tmp"' EXIT
    go build -o "$tmp/terraform-provider-mevspace" "$root"
    cat > "$tmp/tofurc" <<EOF
    provider_installation {
      dev_overrides { "registry.opentofu.org/onnimonni/mevspace" = "$tmp" }
      direct {}
    }
    EOF
    printf 'terraform {\n  required_providers {\n    mevspace = { source = "onnimonni/mevspace" }\n  }\n}\n' > "$tmp/main.tf"
    (cd "$tmp" && TF_CLI_CONFIG_FILE="$tmp/tofurc" tofu providers schema -json) \
      | jq '.provider_schemas |= with_entries(.key = "mevspace")' > "$tmp/schema.json"
    cd "$root"
    tfplugindocs generate --providers-schema "$tmp/schema.json" --provider-name mevspace --rendered-provider-name MEVSpace
  '';

  git-hooks.hooks = {
    gofmt.enable = true;
    govet.enable = true;
    go-test = {
      enable = true;
      name = "go test";
      entry = "go test ./...";
      pass_filenames = false;
      files = "\\.go$";
    };
  };

  enterTest = ''
    go vet ./...
    go test ./...
  '';
}

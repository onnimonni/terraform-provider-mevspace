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
    pkgs.yubikey-manager
  ];

  languages.go.enable = true;

  # Sign a CI-built draft release with the YubiKey and publish it: sign-release vX.Y.Z
  scripts.sign-release.exec = ''
    set -euo pipefail
    tag=''${1:?usage: sign-release vX.Y.Z}
    fpr=0C36D59D531CA3884CFED6D32D6ADE1A7D2F2EBF
    repo=onnimonni/terraform-provider-mevspace
    root=$(git rev-parse --show-toplevel)
    sums="terraform-provider-mevspace_''${tag#v}_SHA256SUMS"
    tmp=$(mktemp -d)
    trap 'rm -r -- "$tmp"' EXIT

    gh release download "$tag" -R "$repo" -D "$tmp" --skip-existing
    rm -f -- "$tmp/$sums.sig"
    echo "Checking every released file against $sums..."
    (cd "$tmp" && shasum -a 256 -c "$sums")

    echo "Signing $sums with the YubiKey (touch it when it blinks)..."
    gpg --local-user "$fpr" --detach-sign --output "$tmp/$sums.sig" "$tmp/$sums"

    # Verify against the committed public key, not the local keyring.
    gpg --dearmor < "$root/keys/signing-key.asc" > "$tmp/key.gpg"
    gpgv --keyring "$tmp/key.gpg" "$tmp/$sums.sig" "$tmp/$sums"

    gh release upload "$tag" -R "$repo" --clobber "$tmp/$sums.sig"
    gh release edit "$tag" -R "$repo" --draft=false
    echo "Signed and published $tag"
  '';

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

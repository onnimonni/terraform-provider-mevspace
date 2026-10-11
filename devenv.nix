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
  ];

  languages.go.enable = true;

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

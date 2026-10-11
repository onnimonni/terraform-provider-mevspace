# terraform-provider-mevspace

OpenTofu / Terraform provider for [MEVSpace](https://mevspace.com), built on the
[MEVSpace API](https://api.mevspace.com/explore/).

## Auth

Create an API token in [panel.mevspace.com](https://panel.mevspace.com) and set `MEVSPACE_API_TOKEN`
(or `api_token` in the provider block). `MEVSPACE_ENDPOINT` / `endpoint` overrides the API URL.

## Resources

| Name | Notes |
|---|---|
| `mevspace_ssh_key` | Full CRUD. `label` updates in place, `value` change recreates. Import by id. |
| `mevspace_dedicated_server` | Adopts an **already ordered** server by `hostname`. Manages OS reinstall (`template_id`, `raid_value`, `startup_script_id`, `reinstall_triggers`, gated by `allow_reinstall`) and `power_state` (`running`/`stopped`). Destroy only removes from state. Import by hostname. |

⚠️ Setting `template_id` on create, or changing any install field later, **reinstalls and wipes** the server. Such plans fail unless `allow_reinstall = true`.

## Data sources

`mevspace_templates` (optional `type` = `dedicated`/`vps`/`vps_cloud`, no token needed),
`mevspace_startup_scripts`, `mevspace_ssh_keys`, `mevspace_dedicated_servers`, `mevspace_dedicated_server`.

List data sources return `items[]` with `id`, `name` and `attributes` (every API field as string),
since the API does not document response schemas.

## CLI

`mevspace` ships alongside the provider (release archives `mevspace-<os>-<arch>`), or `go install github.com/onnimonni/terraform-provider-mevspace/cmd/mevspace@latest`.

```sh
mevspace context create work            # prompts for token (hidden), stored 0600 in ~/.config/mevspace/cli.toml
mevspace server list
mevspace server describe nd12345.mevnode.com
mevspace server restart nd12345.mevnode.com
mevspace server reinstall nd12345.mevnode.com --template "Debian 13" --raid 1   # asks to type hostname
mevspace server kvm nd12345.mevnode.com --client-ip 203.0.113.7
mevspace ssh-key create laptop ~/.ssh/id_ed25519.pub
mevspace template list --type dedicated
mevspace server list --json
```

`MEVSPACE_API_TOKEN` overrides the active context; `--context <name>` picks another one.
KVM access details are not returned by the API; MEVSpace sends them separately.

## API limitations

The API cannot order/cancel servers, manage VPS, or report power state; `power_state` is write-only.

## Development

```sh
devenv shell
go test ./...
go build .
```

Local testing with `~/.tofurc`:

```hcl
provider_installation {
  dev_overrides { "registry.opentofu.org/onnimonni/mevspace" = "/path/to/this/repo" }
  direct {}
}
```

See `examples/`. Regenerate registry docs (`docs/`) after schema changes with `gen-docs`.

## Releasing

1. Push a `vX.Y.Z` tag. CI runs goreleaser and creates a **draft** release with the provider and `mevspace` CLI.
2. Run `sign-release vX.Y.Z` (devenv). It verifies every asset against `SHA256SUMS`, signs it with the
   YubiKey-held key (touch required), checks the signature against `keys/signing-key.asc`, uploads the
   `.sig` and publishes the release.

Signing key: NIST P-521, fingerprint `0C36 D59D 531C A388 4CFE  D6D3 2D6A DE1A 7D2F 2EBF`
(`keys/signing-key.asc`). The private key exists only on a YubiKey; no signing secret is stored in CI.

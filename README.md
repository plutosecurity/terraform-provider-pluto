# Terraform provider for Pluto Security

Manage Pluto policies as code. The provider wraps Pluto's external API (`/api/ext/v1/policies`).

Every policy it writes is marked `managed_by = "terraform"`. The Pluto dashboard then shows a
warning that edits made there are reverted on the next `terraform apply`. Editing in the
dashboard stays allowed; the next `terraform plan` shows those edits as drift.

## Use it

```hcl
terraform {
  required_providers {
    pluto = { source = "plutosecurity/pluto" }
  }
}

provider "pluto" {
  host = "https://<your Pluto API host>" # or PLUTO_HOST
  # api_key from PLUTO_API_KEY: a pluto_... key with policy write access
}

resource "pluto_policy" "notetakers" {
  name             = "Block AI notetakers"
  category         = "AI Notetakers"
  policy_type      = "block"
  enforcement_tool = "Falcon"
  tools            = ["Otter", "Fireflies"]

  targeted_scopes = [{
    type   = "user_department"
    values = ["Engineering"]
  }]
}
```

Bring an existing policy under Terraform with its id:

```sh
terraform import pluto_policy.notetakers <policy id>
```

### `pluto_policy` attributes

| Attribute | Notes |
|---|---|
| `name`, `category`, `policy_type`, `enforcement_tool` | Required. `enforcement_tool` is the integration as Pluto names it (e.g. `"Falcon"`); use `"Monitor"` for `detect` policies. Changing `category` or `enforcement_tool` replaces the policy. |
| `tools` | Tool names the policy targets. Omit to leave the stored selection alone. |
| `targeted_scopes` | Who the policy applies to; `[]` applies it to everyone. Omit to leave it alone. |
| `description`, `custom_message` | Optional. |
| `enabled`, `notify_end_user` | Default `true`. |

Not covered yet: MCP, skill, extension and runtime-enforcement selections, severity selections,
and notification channels. A policy that uses them can still be managed; those settings are
left as stored.

### Deleting

A delete removes a policy with no enforcement rules at once. One with rules is scheduled and
reads as `status: deleting` until its rules are torn down; the provider treats that as deleted.

## Develop

```sh
go build ./...
go test ./...                                   # unit tests only
TF_ACC=1 go test ./...                          # full lifecycle against a fake Pluto API (needs terraform on PATH)
```

To run a local build, point Terraform at it in `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides { "plutosecurity/pluto" = "/path/to/your/GOBIN" }
  direct {}
}
```

`docs/` is generated from the schema and `examples/`; do not edit it by hand:

```sh
go generate ./...                               # needs terraform on PATH
```

## Release

Releases are built and signed by `.github/workflows/release.yml` (GoReleaser) and picked up by
the [Terraform Registry](https://registry.terraform.io/providers/plutosecurity/pluto) through
its release webhook. The workflow needs the `GPG_PRIVATE_KEY` and `PASSPHRASE` repository
secrets for the registry signing key.

1. Add an entry to `CHANGELOG.md` and merge to `main`.
2. Tag `main` with the next SemVer version and push the tag:

   ```sh
   git tag v0.1.0 && git push origin v0.1.0
   ```

3. Check the GitHub Release has the zips, `_SHA256SUMS`, `_SHA256SUMS.sig` and `_manifest.json`
   assets. Never edit a published release; cut a new version instead.

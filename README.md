# Terraform provider for Pluto Security

Manage Pluto policies with Terraform.

```hcl
terraform {
  required_providers {
    pluto = {
      source  = "plutosecurity/pluto"
      version = "~> 0.1"
    }
  }
}
```

Usage is documented for Pluto customers at
[docs.pluto.security](https://docs.pluto.security/integrations/pluto-integrations/terraform).

## Develop

```sh
go build ./...
go generate ./...        # regenerate docs/
TF_ACC=1 go test ./...   # needs terraform on PATH
```

## Release

Push a `v*` tag. `.github/workflows/release.yml` builds and signs the release with GoReleaser.

## License

[MPL-2.0](LICENSE)

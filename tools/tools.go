//go:build generate

// Package tools pins the code-generation tools so `go generate` is reproducible.
package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)

// Format the examples that are embedded in the generated docs.
//go:generate terraform fmt -recursive ../examples/

// Generate docs/ from the provider schema plus examples/ and templates/.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-dir .. --provider-name pluto

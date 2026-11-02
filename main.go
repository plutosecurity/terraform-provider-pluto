package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/plutosecurity/terraform-provider-pluto/internal/provider"
)

// Regenerate docs/ from the schema and examples/ (see tools/tools.go).
//go:generate go generate -C tools ./...

// version is set by the release build.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with debugger support")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/plutosecurity/pluto",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}

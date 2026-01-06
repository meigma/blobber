// Command blobber provides a CLI for pushing and pulling files to OCI registries.
package main

import (
	"os"

	"github.com/gilmanlab/blobber/cmd/blobber/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}

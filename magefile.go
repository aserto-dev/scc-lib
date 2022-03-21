//go:build mage

package main

import (
	"os"

	"github.com/aserto-dev/mage-loot/common"
	"github.com/aserto-dev/mage-loot/deps"
	"github.com/magefile/mage/mg"
)

func init() {
	// Set go version for docker builds
	os.Setenv("GO_VERSION", "1.17")
	// Set private repositories
	os.Setenv("GOPRIVATE", "github.com/aserto-dev")
}

func Deps() {
	deps.GetAllDeps()
}

// Lint runs linting for the entire project.
func Lint() error {
	return common.Lint()
}

// All runs all targets in the appropriate order.
// The targets are run in the following order:
// deps, lint
func All() error {
	mg.SerialDeps(Deps, Lint)
	return nil
}

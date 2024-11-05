//go:build tools
// +build tools

package internal

import (
	_ "github.com/google/wire/cmd/wire"
	_ "go.uber.org/mock/mockgen/model"
)

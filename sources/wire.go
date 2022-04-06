//go:build wireinject
// +build wireinject

package sources

import (
	gomock "github.com/golang/mock/gomock"
	"github.com/google/wire"
	"github.com/rs/zerolog"
)

func NewGitlab(log *zerolog.Logger, cfg *Config) Source {
	wire.Build(
		wire.Struct(new(gitlabSource), "*"),
		wire.Bind(new(Source), new(*gitlabSource)),
		newGitlabInteraction,
	)

	return &gitlabSource{}
}

func NewTestGitlab(ctrl *gomock.Controller, log *zerolog.Logger, cfg *Config, pager glIntr) Source {
	wire.Build(
		wire.Struct(new(gitlabSource), "*"),
		wire.Bind(new(Source), new(*gitlabSource)),
	)

	return &gitlabSource{}
}

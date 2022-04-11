//go:build wireinject
// +build wireinject

package sources

import (
	"github.com/aserto-dev/scc-lib/internal/interactions"
	gomock "github.com/golang/mock/gomock"
	"github.com/google/wire"
	"github.com/rs/zerolog"
)

func NewGitlab(log *zerolog.Logger, cfg *Config) Source {
	wire.Build(
		wire.Struct(new(gitlabSource), "*"),
		wire.Bind(new(Source), new(*gitlabSource)),
		interactions.NewGitlabInteraction,
	)

	return &gitlabSource{}
}

func NewGithub(log *zerolog.Logger, cfg *Config) Source {
	wire.Build(
		wire.Struct(new(githubSource), "*"),
		wire.Bind(new(Source), new(*githubSource)),
		interactions.NewGithubInteraction,
		interactions.NewGraphqInteraction,
	)

	return &githubSource{}
}

func NewTestGithub(ctrl *gomock.Controller, log *zerolog.Logger, cfg *Config, pager interactions.GhIntr, graphql interactions.GraphQLIntr) Source {
	wire.Build(
		wire.Struct(new(githubSource), "*"),
		wire.Bind(new(Source), new(*githubSource)),
	)

	return &githubSource{}
}

func NewTestGitlab(ctrl *gomock.Controller, log *zerolog.Logger, cfg *Config, pager interactions.GlIntr) Source {
	wire.Build(
		wire.Struct(new(gitlabSource), "*"),
		wire.Bind(new(Source), new(*gitlabSource)),
	)

	return &gitlabSource{}
}

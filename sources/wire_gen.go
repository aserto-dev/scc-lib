// Code generated by Wire. DO NOT EDIT.

//go:generate go run -mod=mod github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package sources

import (
	"github.com/aserto-dev/scc-lib/internal/interactions"
	"github.com/rs/zerolog"
	"go.uber.org/mock/gomock"
)

// Injectors from wire.go:

func NewGitlab(log *zerolog.Logger, cfg *Config) Source {
	glIntr := interactions.NewGitlabInteraction()
	sourcesGitlabSource := &gitlabSource{
		logger:           log,
		cfg:              cfg,
		interactionsFunc: glIntr,
	}
	return sourcesGitlabSource
}

func NewGithub(log *zerolog.Logger, cfg *Config) Source {
	ghIntr := interactions.NewGithubInteraction()
	gqlIntr := interactions.NewGraphqlInteraction()
	sourcesGithubSource := &githubSource{
		logger:           log,
		cfg:              cfg,
		interactionsFunc: ghIntr,
		graphqlFunc:      gqlIntr,
	}
	return sourcesGithubSource
}

func NewTestGithub(ctrl *gomock.Controller, log *zerolog.Logger, cfg *Config, pager interactions.GhIntr, graphql interactions.GqlIntr) Source {
	sourcesGithubSource := &githubSource{
		logger:           log,
		cfg:              cfg,
		interactionsFunc: pager,
		graphqlFunc:      graphql,
	}
	return sourcesGithubSource
}

func NewTestGitlab(ctrl *gomock.Controller, log *zerolog.Logger, cfg *Config, pager interactions.GlIntr) Source {
	sourcesGitlabSource := &gitlabSource{
		logger:           log,
		cfg:              cfg,
		interactionsFunc: pager,
	}
	return sourcesGitlabSource
}

package interactions

import (
	"context"

	"github.com/google/go-github/v33/github"
	"golang.org/x/oauth2"
)

//go:generate mockgen -source=githubintr.go -destination=mock_githubintr.go -package=interactions --build_flags=--mod=mod

type GhIntr func(ctx context.Context, token, tokenType string) GithubIntr

type GithubIntr interface {
	GetUsers(context.Context, string) (*github.User, *github.Response, error)
	ListRepoSecrets(context.Context, string, string, *github.ListOptions) (*github.Secrets, error)
	GetRepoPublicKey(context.Context, string, string) (*github.PublicKey, error)
	CreateOrUpdateRepoSecret(context.Context, string, string, *github.EncryptedSecret) (*github.Response, error)
	GetRepo(context.Context, string, string) (*github.Repository, error)
	CreateRepo(context.Context, string, *github.Repository) error
	ListRepoTags(context.Context, string, string, *github.ListOptions) ([]*github.RepositoryTag, error)
	GetRepoRef(context.Context, string, string, string) (*github.Reference, *github.Response, error)
	CreateRepoTag(context.Context, string, string, *github.Tag) (*github.Tag, error)
	CreateRepoRef(context.Context, string, string, *github.Reference) error
}

type githubInteraction struct {
	Client *github.Client
}

func NewGithubInteraction() GhIntr {
	return func(ctx context.Context, token, tokenType string) GithubIntr {
		tokenSource := oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: token,
				TokenType:   tokenType,
			},
		)
		clientWithToken := oauth2.NewClient(ctx, tokenSource)

		githubClient := github.NewClient(clientWithToken)

		return &githubInteraction{Client: githubClient}
	}
}

func (gh *githubInteraction) GetUsers(ctx context.Context, username string) (*github.User, *github.Response, error) {
	return gh.Client.Users.Get(ctx, username)
}

func (gh *githubInteraction) ListRepoSecrets(ctx context.Context, owner, repo string, opts *github.ListOptions) (*github.Secrets, error) {
	secrets, _, err := gh.Client.Actions.ListRepoSecrets(ctx, owner, repo, opts)
	return secrets, err
}

func (gh *githubInteraction) GetRepoPublicKey(ctx context.Context, org, repo string) (*github.PublicKey, error) {
	key, _, err := gh.Client.Actions.GetRepoPublicKey(ctx, org, repo)
	return key, err
}

func (gh *githubInteraction) CreateOrUpdateRepoSecret(ctx context.Context, org, repo string, secret *github.EncryptedSecret) (*github.Response, error) {
	return gh.Client.Actions.CreateOrUpdateRepoSecret(ctx, org, repo, secret)
}

func (gh *githubInteraction) GetRepo(ctx context.Context, owner, repo string) (*github.Repository, error) {
	repoResult, _, err := gh.Client.Repositories.Get(ctx, owner, repo)
	return repoResult, err
}

func (gh *githubInteraction) CreateRepo(ctx context.Context, owner string, repo *github.Repository) error {
	_, _, err := gh.Client.Repositories.Create(ctx, owner, repo)
	return err
}

func (gh *githubInteraction) ListRepoTags(ctx context.Context, owner, repo string, opts *github.ListOptions) ([]*github.RepositoryTag, error) {
	tags, _, err := gh.Client.Repositories.ListTags(ctx, owner, repo, opts)
	return tags, err
}

func (gh *githubInteraction) GetRepoRef(ctx context.Context, owner, repo, ref string) (*github.Reference, *github.Response, error) {
	return gh.Client.Git.GetRef(ctx, owner, repo, ref)
}

func (gh *githubInteraction) CreateRepoTag(ctx context.Context, owner, repo string, tag *github.Tag) (*github.Tag, error) {
	tagResult, _, err := gh.Client.Git.CreateTag(ctx, owner, repo, tag)
	return tagResult, err
}

func (gh *githubInteraction) CreateRepoRef(ctx context.Context, owner, repo string, ref *github.Reference) error {
	_, _, err := gh.Client.Git.CreateRef(ctx, owner, repo, ref)
	return err
}

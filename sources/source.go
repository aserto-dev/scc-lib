package sources

import (
	"context"
	"net/http"

	"github.com/aserto-dev/go-grpc/aserto/api/v1"
	scc "github.com/aserto-dev/go-grpc/aserto/tenant/scc/v1"
)

var (
	defaultTag = "v0.0.0"
)

type AccessToken struct {
	Token string
	Type  string
}

type Config struct {
	CreateRepoTimeoutSeconds int
}

type Commit struct {
	Branch  string
	Message string
	Owner   string
	Repo    string
	Content map[string]string
}

type Source interface {
	ValidateConnection(ctx context.Context, accessToken *AccessToken) (*http.Response, error)
	Profile(ctx context.Context, accessToken *AccessToken) (string, []*scc.Repo, error)
	ListOrgs(ctx context.Context, accessToken *AccessToken, page *api.PaginationRequest) ([]string, *api.PaginationResponse, error)
	ListRepos(ctx context.Context, accessToken *AccessToken, owner string, page *api.PaginationRequest) ([]*scc.Repo, *api.PaginationResponse, error)
	CreateRepo(ctx context.Context, accessToken *AccessToken, owner, name string) error
	GetRepo(ctx context.Context, accessToken *AccessToken, owner, repo string) (*scc.Repo, error)
	HasSecret(ctx context.Context, token *AccessToken, owner, repo, secretName string) (bool, error)
	AddSecretToRepo(ctx context.Context, token *AccessToken, orgName, repoName, secretName, value string, overrideSecret bool) error
	InitialTag(ctx context.Context, accessToken *AccessToken, fullName string) error
	CreateCommitOnBranch(ctx context.Context, accessToken *AccessToken, commit *Commit) error
	GetDefaultBranch(ctx context.Context, accessToken *AccessToken, owner, repo string) (string, error)
}

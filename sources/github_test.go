package sources

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aserto-dev/go-grpc/aserto/api/v1"
	"github.com/aserto-dev/go-utils/cerr"
	"github.com/aserto-dev/scc-lib/internal/interactions"
	gomock "github.com/golang/mock/gomock"
	"github.com/google/go-github/v33/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

var mockGithubIntr *interactions.MockGithubIntr
var mockGraphqlIntr *interactions.MockGraphqlIntr

const (
	policyRepo     = "policy"
	policyURL      = "https://github.com/policy"
	githubUsername = "aserto-dev"
	defaultBranch  = "main"
)

func init() {
	t := &testing.T{}
	ctrl := gomock.NewController(t)
	mockGithubIntr = interactions.NewMockGithubIntr(ctrl)
	mockGraphqlIntr = interactions.NewMockGraphqlIntr(ctrl)
}

func newMockGithubIntrFunc(ctrl *gomock.Controller) interactions.GhIntr {
	return func(ctx context.Context, token, tokenType string) interactions.GithubIntr {
		return mockGithubIntr
	}
}

func newMockGraphqlIntrFunc(ctrl *gomock.Controller) interactions.GraphQLIntr {
	return func(ctx context.Context, token, tokenType string) interactions.GraphqlIntr {
		return mockGraphqlIntr
	}
}

func TestMockGithubConstructor(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)

	// Act
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)

	// Assert
	assert.NotNil(p)
}

func TestGithubConstructor(t *testing.T) {
	// Arrange
	assert := require.New(t)
	p := NewGithub(&zerolog.Logger{}, &Config{})
	token := &AccessToken{Token: ""}

	// Act
	err := p.ValidateConnection(context.Background(), token)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "GET https://api.github.com/user: 401 Bad credentials []")
}

func TestValidateConnectionGetUsersFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().GetUsers(gomock.Any(), gomock.Any()).Return(nil, nil, errors.New("no Connection"))

	// Act
	err := p.ValidateConnection(context.Background(), token)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "failed to connect to Github: no Connection")
}

func TestGithubValidateConnectionErrorResponse(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	body := io.NopCloser(strings.NewReader("this is the body"))
	resp := &github.Response{Response: &http.Response{StatusCode: 404, Status: "Not Found", Body: body}}

	// Expect
	mockGithubIntr.EXPECT().GetUsers(gomock.Any(), gomock.Any()).Return(nil, resp, nil)

	// Act
	err := p.ValidateConnection(context.Background(), token)

	// Assert
	assert.Error(err)
	assertoErr := cerr.UnwrapAsertoError(err)
	assert.Contains(assertoErr.Data()["msg"], "unexpected reply from GitHub")
}

func TestGithubValidateConnection(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	resp := &github.Response{Response: &http.Response{StatusCode: 200}}

	// Expect
	mockGithubIntr.EXPECT().GetUsers(gomock.Any(), gomock.Any()).Return(nil, resp, nil)

	// Act
	err := p.ValidateConnection(context.Background(), token)

	// Assert
	assert.NoError(err)
}

func TestGithubProfileQueryFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGraphqlIntr.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	// Act
	username, repos, err := p.Profile(context.Background(), token)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "error running query against github graphql server: boom")
	assert.Empty(username)
	assert.Nil(repos)
}

func TestGithubProfile(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGraphqlIntr.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	// Act
	username, repos, err := p.Profile(context.Background(), token)

	// Assert
	assert.NoError(err)
	assert.Empty(username)
	assert.Empty(repos)
}

func TestGithubHasSecretFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{CreateRepoTimeoutSeconds: 0}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().ListRepoSecrets(gomock.Any(), githubUsername, policyRepo, gomock.Any()).Return(nil, errors.New("Failed to get secret"))

	// Act
	exists, err := p.HasSecret(context.Background(), token, githubUsername, policyRepo, "ASERTO_PUSH_KEY")

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "failed to list repo secrets: E10034 timeout after multiple retries: Failed to get secret")
	assert.False(exists)
}

func TestGithubHasSecretTrue(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{CreateRepoTimeoutSeconds: 1}, mockintrGh, mockintrGQL)
	secret := &github.Secret{Name: "ASERTO_PUSH_KEY"}
	secrets := []*github.Secret{secret}
	result := &github.Secrets{Secrets: secrets}
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().ListRepoSecrets(gomock.Any(), githubUsername, policyRepo, gomock.Any()).Return(result, nil)

	// Act
	exists, err := p.HasSecret(context.Background(), token, githubUsername, policyRepo, "ASERTO_PUSH_KEY")

	// Assert
	assert.NoError(err)
	assert.True(exists)
}

func TestGithubHasSecretFalse(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{CreateRepoTimeoutSeconds: 1}, mockintrGh, mockintrGQL)
	secret := &github.Secret{Name: "other_secret"}
	secrets := []*github.Secret{secret}
	result := &github.Secrets{Secrets: secrets}
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().ListRepoSecrets(gomock.Any(), githubUsername, policyRepo, gomock.Any()).Return(result, nil)

	// Act
	exists, err := p.HasSecret(context.Background(), token, githubUsername, policyRepo, "ASERTO_PUSH_KEY")

	// Assert
	assert.NoError(err)
	assert.False(exists)
}

func TestAddSecretToRepoNoOrg(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{CreateRepoTimeoutSeconds: 0}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Act
	err := p.AddSecretToRepo(context.Background(), token, "", policyRepo, "ASERTO_PUSH_KEY", "value", false)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "No org name was provided")
}

func TestAddSecretToRepoNoRepo(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{CreateRepoTimeoutSeconds: 0}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Act
	err := p.AddSecretToRepo(context.Background(), token, githubUsername, "", "ASERTO_PUSH_KEY", "value", false)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "No repo name was provided")
}

func TestAddSecretToRepoGetPublicKeyFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{CreateRepoTimeoutSeconds: 0}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().GetRepoPublicKey(gomock.Any(), githubUsername, policyRepo).Return(nil, errors.New("failed to connect"))

	// Act
	err := p.AddSecretToRepo(context.Background(), token, githubUsername, policyRepo, "ASERTO_PUSH_KEY", "value", false)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "failed to get public repo key for encryption: E10034 timeout after multiple retries: failed to connect")
}

func TestAddSecretToRepoSecretExistsOverrideFalse(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{CreateRepoTimeoutSeconds: 1}, mockintrGh, mockintrGQL)
	secret := &github.Secret{Name: "ASERTO_PUSH_KEY"}
	secrets := []*github.Secret{secret}
	result := &github.Secrets{Secrets: secrets}
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().GetRepoPublicKey(gomock.Any(), githubUsername, policyRepo).Return(&github.PublicKey{}, nil)
	mockGithubIntr.EXPECT().ListRepoSecrets(gomock.Any(), githubUsername, policyRepo, gomock.Any()).Return(result, nil)

	// Act
	err := p.AddSecretToRepo(context.Background(), token, githubUsername, policyRepo, "ASERTO_PUSH_KEY", "value", false)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "E10022 repo has already been connected to a policy")
}

func TestAddSecretToRepoSecretExistsOverrideTrueCreateFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{CreateRepoTimeoutSeconds: 0}, mockintrGh, mockintrGQL)
	secret := &github.Secret{Name: "ASERTO_PUSH_KEY"}
	secrets := []*github.Secret{secret}
	result := &github.Secrets{Secrets: secrets}
	token := &AccessToken{Token: "sometokenvalue"}
	body := io.NopCloser(strings.NewReader("this is the body"))
	resp := &github.Response{Response: &http.Response{StatusCode: 404, Status: "Not Found", Body: body}}

	// Expect
	mockGithubIntr.EXPECT().GetRepoPublicKey(gomock.Any(), githubUsername, policyRepo).Return(&github.PublicKey{}, nil)
	mockGithubIntr.EXPECT().ListRepoSecrets(gomock.Any(), githubUsername, policyRepo, gomock.Any()).Return(result, nil)
	mockGithubIntr.EXPECT().
		CreateOrUpdateRepoSecret(gomock.Any(), githubUsername, policyRepo, gomock.Any()).
		Return(resp, errors.New("Failed to create repo secret"))

	// Act
	err := p.AddSecretToRepo(context.Background(), token, githubUsername, policyRepo, "ASERTO_PUSH_KEY", "value", true)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "E10023 failed to setup repo secret: E10034 timeout after multiple retries: Failed to create repo secret")
}

func TestAddSecretToRepoSecretExistsOverrideTrueCreate(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{CreateRepoTimeoutSeconds: 1}, mockintrGh, mockintrGQL)
	secret := &github.Secret{Name: "ASERTO_PUSH_KEY"}
	secrets := []*github.Secret{secret}
	result := &github.Secrets{Secrets: secrets}
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().GetRepoPublicKey(gomock.Any(), githubUsername, policyRepo).Return(&github.PublicKey{}, nil)
	mockGithubIntr.EXPECT().ListRepoSecrets(gomock.Any(), githubUsername, policyRepo, gomock.Any()).Return(result, nil)
	mockGithubIntr.EXPECT().
		CreateOrUpdateRepoSecret(gomock.Any(), githubUsername, policyRepo, gomock.Any()).
		Return(nil, nil)

	// Act
	err := p.AddSecretToRepo(context.Background(), token, githubUsername, policyRepo, "ASERTO_PUSH_KEY", "value", true)

	// Assert
	assert.NoError(err)
}

func TestListOrgsPageNil(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Act
	orgs, resp, err := p.ListOrgs(context.Background(), token, nil)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "page must not be empty")
	assert.Empty(orgs)
	assert.Nil(resp)
}

func TestListOrgsPageSizeInvalid(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: int32(-2)}

	// Act
	orgs, resp, err := p.ListOrgs(context.Background(), token, page)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "page size must be >= -1 and <= 100")
	assert.Empty(orgs)
	assert.Nil(resp)
}

func TestGithubListOrgsQueryFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: int32(-1), Token: ""}

	// Expect
	mockGraphqlIntr.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	// Act
	orgs, resp, err := p.ListOrgs(context.Background(), token, page)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "error running query against github graphql server: boom")
	assert.Empty(orgs)
	assert.Nil(resp)
}

func TestGithubListOrgs(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: int32(-1), Token: ""}

	// Expect
	mockGraphqlIntr.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	// Act
	orgs, resp, err := p.ListOrgs(context.Background(), token, page)

	// Assert
	assert.NoError(err)
	assert.Empty(orgs)
	assert.NotNil(resp)
	assert.Equal(resp.TotalSize, int32(0))
}

func TestListReposPageNil(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Act
	repos, resp, err := p.ListRepos(context.Background(), token, githubUsername, nil)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "page must not be empty")
	assert.Empty(repos)
	assert.Nil(resp)
}

func TestListReposPageSizeInvalid(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: int32(-2)}

	// Act
	repos, resp, err := p.ListRepos(context.Background(), token, githubUsername, page)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "page size must be >= -1 and <= 100")
	assert.Empty(repos)
	assert.Nil(resp)
}

func TestListReposQueryFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: int32(-1)}

	// Expect
	mockGraphqlIntr.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	// Act
	repos, resp, err := p.ListRepos(context.Background(), token, githubUsername, page)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "error running query against github graphql server: boom")
	assert.Empty(repos)
	assert.Nil(resp)
}

func TestListRepos(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: int32(-1)}

	// Expect
	mockGraphqlIntr.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	// Act
	repos, resp, err := p.ListRepos(context.Background(), token, githubUsername, page)

	// Assert
	assert.NoError(err)
	assert.Empty(repos)
	assert.NotNil(resp)
	assert.Equal(resp.TotalSize, int32(0))
}

func TestGetRepoFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().GetRepo(gomock.Any(), githubUsername, policyRepo).Return(nil, errors.New("no Connection"))

	// Act
	repo, err := p.GetRepo(context.Background(), token, githubUsername, policyRepo)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "failed to get repo: no Connection")
	assert.Nil(repo)
}

func TestGithubGetRepo(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	user := githubUsername
	searchedRepo := policyRepo
	URL := policyURL
	githubUser := &github.User{Login: &user}
	githubRepo := &github.Repository{Name: &searchedRepo, Owner: githubUser, HTMLURL: &URL}

	// Expect
	mockGithubIntr.EXPECT().GetRepo(gomock.Any(), githubUsername, policyRepo).Return(githubRepo, nil)

	// Act
	repo, err := p.GetRepo(context.Background(), token, githubUsername, policyRepo)

	// Assert
	assert.NoError(err)
	assert.NotNil(repo)
	assert.Equal(repo.Url, policyURL)
}

func TestGithubCreateRepoGetUsersFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().GetUsers(gomock.Any(), gomock.Any()).Return(nil, nil, errors.New("boom"))

	// Act
	err := p.CreateRepo(context.Background(), token, githubUsername, policyRepo)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "failed to read user from github: boom")
}

func TestGithubCreateFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	username := githubUsername
	user := &github.User{Login: &username}

	// Expect
	mockGithubIntr.EXPECT().GetUsers(gomock.Any(), gomock.Any()).Return(user, nil, nil)
	mockGithubIntr.EXPECT().CreateRepo(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	// Act
	err := p.CreateRepo(context.Background(), token, githubUsername, policyRepo)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "failed to create repo: boom")
}

func TestGithubCreate(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	username := githubUsername
	user := &github.User{Login: &username}

	// Expect
	mockGithubIntr.EXPECT().GetUsers(gomock.Any(), gomock.Any()).Return(user, nil, nil)
	mockGithubIntr.EXPECT().CreateRepo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	// Act
	err := p.CreateRepo(context.Background(), token, githubUsername, policyRepo)

	// Assert
	assert.NoError(err)
}

func TestGetDefultRepoFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().GetRepo(gomock.Any(), githubUsername, policyRepo).Return(nil, errors.New("no Connection"))

	// Act
	repo, err := p.GetDefaultBranch(context.Background(), token, githubUsername, policyRepo)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "failed to get repo: no Connection")
	assert.Empty(repo)
}

func TestGithubGetDefultRepo(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	defaultBr := defaultBranch
	githubRepo := &github.Repository{DefaultBranch: &defaultBr}

	// Expect
	mockGithubIntr.EXPECT().GetRepo(gomock.Any(), githubUsername, policyRepo).Return(githubRepo, nil)

	// Act
	repo, err := p.GetDefaultBranch(context.Background(), token, githubUsername, policyRepo)

	// Assert
	assert.NoError(err)
	assert.NotEmpty(repo)
	assert.Equal(repo, defaultBranch)
}

func TestGithubInitialTagWithInvalidRepoPath(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Act
	err := p.InitialTag(context.Background(), token, "policy")

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "invalid full github repo name 'policy', should be in the form owner/repo")
}

func TestGithubInitialTagAndGetRepoFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().GetRepo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("not found"))

	// Act
	err := p.InitialTag(context.Background(), token, githubUsername+"/"+policyRepo)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "failed to get repo: not found")
}

func TestGithubInitialTagAndListRepoTagsFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}

	// Expect
	mockGithubIntr.EXPECT().GetRepo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
	mockGithubIntr.EXPECT().
		ListRepoTags(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("tags not found"))

	// Act
	err := p.InitialTag(context.Background(), token, githubUsername+"/"+policyRepo)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "failed to list tags for repo 'aserto-dev/policy': tags not found")
}

func TestGithubInitialTagAndRepoHasTag(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	repoTag := &github.RepositoryTag{}

	// Expect
	mockGithubIntr.EXPECT().GetRepo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
	mockGithubIntr.EXPECT().
		ListRepoTags(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*github.RepositoryTag{repoTag}, nil)

	// Act
	err := p.InitialTag(context.Background(), token, githubUsername+"/"+policyRepo)

	// Assert
	assert.NoError(err)
}

func TestGithubInitialTagAndGetRepoRefFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	defaultBr := defaultBranch
	githubRepo := &github.Repository{DefaultBranch: &defaultBr}
	resp := &github.Response{Response: &http.Response{StatusCode: 404}}

	// Expect
	mockGithubIntr.EXPECT().GetRepo(gomock.Any(), gomock.Any(), gomock.Any()).Return(githubRepo, nil)
	mockGithubIntr.EXPECT().
		ListRepoTags(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)
	mockGithubIntr.EXPECT().
		GetRepoRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, resp, errors.New("ref not found"))

	// Act
	err := p.InitialTag(context.Background(), token, githubUsername+"/"+policyRepo)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "E10034 timeout after multiple retries: repo seems to be empty; response code from github [404]: ref not found")
}

func TestGithubInitialTag(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrGh := newMockGithubIntrFunc(ctrl)
	mockintrGQL := newMockGraphqlIntrFunc(ctrl)
	p := NewTestGithub(ctrl, &zerolog.Logger{}, &Config{CreateRepoTimeoutSeconds: 1}, mockintrGh, mockintrGQL)
	token := &AccessToken{Token: "sometokenvalue"}
	defaultBr := defaultBranch
	githubRepo := &github.Repository{DefaultBranch: &defaultBr}
	resp := &github.Response{Response: &http.Response{StatusCode: 200}}
	sha := "somesha"
	obj := &github.GitObject{SHA: &sha}
	ref := &github.Reference{Object: obj}
	tag := &github.Tag{Object: obj, Tag: &defaultTag}

	// Expect
	mockGithubIntr.EXPECT().GetRepo(gomock.Any(), gomock.Any(), gomock.Any()).Return(githubRepo, nil)
	mockGithubIntr.EXPECT().
		ListRepoTags(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)
	mockGithubIntr.EXPECT().
		GetRepoRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(ref, resp, nil)
	mockGithubIntr.EXPECT().
		CreateRepoTag(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(tag, nil)
	mockGithubIntr.EXPECT().
		CreateRepoRef(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	// Act
	err := p.InitialTag(context.Background(), token, githubUsername+"/"+policyRepo)

	// Assert
	assert.NoError(err)
}
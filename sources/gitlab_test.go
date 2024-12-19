package sources_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	cerr "github.com/aserto-dev/errors"
	"github.com/aserto-dev/go-grpc/aserto/api/v1"
	"github.com/aserto-dev/scc-lib/internal/interactions"
	"github.com/aserto-dev/scc-lib/sources"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	"go.uber.org/mock/gomock"
)

var mockIntr *interactions.MockGitlabIntr

const (
	repo        = "policy"
	file        = ".gitignore"
	fileContent = "cover.out"
)

//nolint:gochecknoinits // test function in test namespace
func init() {
	t := &testing.T{}
	ctrl := gomock.NewController(t)
	mockIntr = interactions.NewMockGitlabIntr(ctrl)
}

func newMockIntrFunc(ctrl *gomock.Controller) interactions.GlIntr {
	return func(token string) (interactions.GitlabIntr, error) {
		if token == "" {
			return nil, errors.New("Kaboom")
		}
		return mockIntr, nil
	}
}

func TestMockConstructor(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintr := newMockIntrFunc(ctrl)

	// Act
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintr)

	// Assert
	assert.NotNil(p)
}

func TestConstructor(t *testing.T) {
	// Arrange
	assert := require.New(t)
	p := sources.NewGitlab(&zerolog.Logger{}, &sources.Config{})
	token := &sources.AccessToken{Token: ""}

	// Act
	err := p.ValidateConnection(context.Background(), token, []string{})

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "GET https://gitlab.com/api/v4/user: 401 {message: 401 Unauthorized}")
}

func TestValidateConnectionWithEmptyToken(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintr := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintr)
	token := &sources.AccessToken{Token: ""}

	// Act
	err := p.ValidateConnection(context.Background(), token, []string{})

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "failed to create Gitlab client: Kaboom")
}

func TestValidateConnectionDoesntConnect(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}

	// Expect
	mockIntr.EXPECT().CurrentUser().Return(nil, nil, errors.New("no Connection"))

	// Act
	err := p.ValidateConnection(context.Background(), token, []string{})

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "failed to connect to Gitlab: no Connection")
}

func TestValidateConnection(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	resp := &gitlab.Response{Response: &http.Response{StatusCode: 200}}

	// Expect
	mockIntr.EXPECT().CurrentUser().Return(nil, resp, nil)

	// Act
	err := p.ValidateConnection(context.Background(), token, []string{})

	// Assert
	assert.NoError(err)
}

func TestValidateConnectionErrorResponse(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	body := io.NopCloser(strings.NewReader("this is the body"))
	resp := &gitlab.Response{Response: &http.Response{StatusCode: 404, Status: "Not Found", Body: body}}

	// Expect
	mockIntr.EXPECT().CurrentUser().Return(nil, resp, nil)

	// Act
	err := p.ValidateConnection(context.Background(), token, []string{})

	// Assert
	assert.Error(err)
	assertoErr := cerr.UnwrapAsertoError(err)
	assert.Contains(assertoErr.Data()["msg"], "unexpected reply from Gitlab")
}

func TestProfileConnectionWithEmptyToken(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintr := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintr)
	token := &sources.AccessToken{Token: ""}

	// Act
	_, _, err := p.Profile(context.Background(), token)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "failed to create Gitlab client: Kaboom")
}

func TestProfileDoesntConnect(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}

	// Expect
	mockIntr.EXPECT().CurrentUser().Return(nil, nil, errors.New("no Connection"))

	// Act
	_, _, err := p.Profile(context.Background(), token)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "no Connection")
}

func TestProfileReadOnePage(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	var projects []*gitlab.Project
	gitlabUser := &gitlab.User{Username: "aserto-tests"}
	projects = append(projects, &gitlab.Project{Name: "template-policy", Owner: gitlabUser, WebURL: "gitlab.com/template-policy"})
	resp := &gitlab.Response{NextPage: 0}

	// Expect
	mockIntr.EXPECT().CurrentUser().Return(gitlabUser, nil, nil)
	mockIntr.EXPECT().ListUserProjects("aserto-tests", gomock.Any()).Return(projects, resp, nil)

	// Act
	username, repos, err := p.Profile(context.Background(), token)

	// Assert
	assert.NoError(err)
	assert.Equal(username, gitlabUser.Username)
	assert.Equal(len(repos), 1)
	assert.Equal(repos[0].Name, "template-policy")
	assert.Equal(repos[0].CiUrl, "gitlab.com/template-policy/-/pipelines")
}

func TestProfileReadTwoPages(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	var projects []*gitlab.Project
	var projectsSecondPage []*gitlab.Project
	gitlabUser := &gitlab.User{Username: "aserto-tests"}
	projects = append(projects, &gitlab.Project{Name: "template-policy", Owner: gitlabUser, WebURL: "gitlab.com/template-policy"})
	projectsSecondPage = append(projectsSecondPage, &gitlab.Project{Name: "template-policy2", Owner: nil, WebURL: "gitlab.com/template-policy2"})
	resp := &gitlab.Response{NextPage: 1}
	resp2 := &gitlab.Response{NextPage: 0}

	// Expect
	mockIntr.EXPECT().CurrentUser().Return(gitlabUser, nil, nil)
	mockIntr.EXPECT().ListUserProjects("aserto-tests", gomock.Any()).Return(projects, resp, nil).Times(1)
	mockIntr.EXPECT().ListUserProjects("aserto-tests", gomock.Any()).Return(projectsSecondPage, resp2, nil).Times(1)

	// Act
	username, repos, err := p.Profile(context.Background(), token)

	// Assert
	assert.NoError(err)
	assert.Equal(username, gitlabUser.Username)
	assert.Equal(1, len(repos))
	assert.Equal(repos[0].Name, "template-policy")
}

func TestListOrgsWithNilPage(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	// Act
	_, _, err := p.ListOrgs(context.Background(), token, nil)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "page must not be empty")
}

func TestListOrgsWithInvalidPageSize(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: -2}
	// Act
	_, _, err := p.ListOrgs(context.Background(), token, page)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "page size must be >= -1 and <= 100")
}

func TestListOrgsWithStringPageToken(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: -1, Token: "next_token"}
	// Act
	_, _, err := p.ListOrgs(context.Background(), token, page)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "page token must be int: strconv.Atoi: parsing \"next_token\":")
}

func TestListOrgsAllInOnePage(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: -1, Token: ""}
	var groups []*gitlab.Group
	groups = append(groups, &gitlab.Group{Name: "tests", FullPath: "test7929"})
	resp := &gitlab.Response{NextPage: 0, TotalItems: 1}

	// Expect
	mockIntr.EXPECT().ListGroups(gomock.Any()).Return(groups, resp, nil)

	// Act
	orgs, pageResp, err := p.ListOrgs(context.Background(), token, page)

	// Assert
	assert.NoError(err)
	assert.Equal(pageResp.ResultSize, pageResp.TotalSize)
	assert.Equal(1, len(orgs))
	assert.Equal("test7929", orgs[0].Id)
}

func TestListOrgsWithTwoPages(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: -1, Token: ""}
	var groups []*gitlab.Group
	var groupsSecondPage []*gitlab.Group
	groups = append(groups, &gitlab.Group{Name: "tests", Path: "test7929"})
	groupsSecondPage = append(groupsSecondPage, &gitlab.Group{Name: "aserto-demo", Path: "aserto-demmo"})
	resp := &gitlab.Response{NextPage: 1, ItemsPerPage: 1, TotalItems: 2}
	resp2 := &gitlab.Response{NextPage: 0, ItemsPerPage: 1, TotalItems: 2}

	// Expect
	mockIntr.EXPECT().ListGroups(gomock.Any()).Return(groups, resp, nil)
	mockIntr.EXPECT().ListGroups(gomock.Any()).Return(groupsSecondPage, resp2, nil)

	// Act
	orgs, pageResp, err := p.ListOrgs(context.Background(), token, page)

	// Assert
	assert.NoError(err)
	assert.Equal(pageResp.ResultSize, pageResp.TotalSize)
	assert.Equal(2, len(orgs))
}

func TestListReposWithNilPage(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	// Act
	_, _, err := p.ListRepos(context.Background(), token, "aserto-demo", nil)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "page must not be empty")
}

func TestListReposWithInvalidPageSize(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: -2}
	// Act
	_, _, err := p.ListRepos(context.Background(), token, "aserto-demo", page)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "page size must be >= -1 and <= 100")
}

func TestListReposWithStringPageToken(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: -1, Token: "next_token"}
	// Act
	_, _, err := p.ListRepos(context.Background(), token, "aserto-demo", page)

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "page token must be int: strconv.Atoi: parsing \"next_token\":")
}

func TestListReposAllInOnePageWithUser(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: -1, Token: ""}
	var projects []*gitlab.Project
	gitlabUser := &gitlab.User{Username: "aserto-demo"}
	projects = append(projects, &gitlab.Project{Name: "template-policy", Owner: gitlabUser, WebURL: "gitlab.com/template-policy"})
	resp := &gitlab.Response{NextPage: 0, TotalItems: 1}

	// Expect
	mockIntr.EXPECT().CurrentUser().Return(gitlabUser, nil, nil)
	mockIntr.EXPECT().ListUserProjects("aserto-demo", gomock.Any()).Return(projects, resp, nil)

	// Act
	repos, pageResp, err := p.ListRepos(context.Background(), token, "aserto-demo", page)

	// Assert
	assert.NoError(err)
	assert.Equal(pageResp.ResultSize, pageResp.TotalSize)
	assert.Equal(1, len(repos))
	assert.Equal("template-policy", repos[0].Name)
}

func TestListReposAllInOnePageWithOrg(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	page := &api.PaginationRequest{Size: -1, Token: ""}
	var projects []*gitlab.Project
	gitlabUser := &gitlab.User{Username: "aserto-demo"}
	projects = append(projects, &gitlab.Project{Name: "template-policy", Owner: gitlabUser, WebURL: "gitlab.com/template-policy"})
	resp := &gitlab.Response{NextPage: 0, TotalItems: 1}

	// Expect
	mockIntr.EXPECT().CurrentUser().Return(gitlabUser, nil, nil)
	mockIntr.EXPECT().ListGroupProjects("aserto-dev", gomock.Any()).Return(projects, resp, nil)

	// Act
	repos, pageResp, err := p.ListRepos(context.Background(), token, "aserto-dev", page)

	// Assert
	assert.NoError(err)
	assert.Equal(pageResp.ResultSize, pageResp.TotalSize)
	assert.Equal(1, len(repos))
	assert.Equal("template-policy", repos[0].Name)
}

func TestGetRepoFail(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}

	// Expect
	mockIntr.EXPECT().GetProject("aserto-dev/policy").Return(nil, nil, errors.New("repo not found"))

	// Act
	repo, err := p.GetRepo(context.Background(), token, "aserto-dev", "policy")

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "failed to get project: aserto-dev/policy: repo not found")
	assert.Nil(repo)
}

func TestGetRepo(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	proj := &gitlab.Project{Name: "policy", WebURL: "gitlab.com/policy"}

	// Expect
	mockIntr.EXPECT().GetProject("aserto-dev/policy").Return(proj, nil, nil)

	// Act
	repo, err := p.GetRepo(context.Background(), token, "aserto-dev", "policy")

	// Assert
	assert.NoError(err)
	assert.NotNil(repo)
	assert.Equal(repo.Org, "aserto-dev")
}

func TestGetDefaultBranchFail(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}

	// Expect
	mockIntr.EXPECT().GetProject("aserto-dev/policy").Return(nil, nil, errors.New("repo not found"))

	// Act
	defaultBr, err := p.GetDefaultBranch(context.Background(), token, "aserto-dev", "policy")

	// Assert
	assert.Error(err)
	assert.Contains(err.Error(), "failed to get project: aserto-dev/policy: repo not found")
	assert.Equal(defaultBr, "")
}

func TestGetDefaultBranch(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	proj := &gitlab.Project{Name: "policy", WebURL: "gitlab.com/policy", DefaultBranch: "main"}

	// Expect
	mockIntr.EXPECT().GetProject("aserto-dev/policy").Return(proj, nil, nil)

	// Act
	defaultBr, err := p.GetDefaultBranch(context.Background(), token, "aserto-dev", "policy")

	// Assert
	assert.NoError(err)
	assert.NotEmpty(defaultBr)
	assert.Equal(defaultBr, "main")
}

func TestCreateRepoAndGetNamespaceFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}

	// Expect
	mockIntr.EXPECT().GetNamespace("aserto-dev").Return(nil, errors.New("namespace not found"))

	// Act
	err := p.CreateRepo(context.Background(), token, "aserto-dev", "policy")

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "namespace not found")
}

func TestCreateRepoFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	namespace := &gitlab.Namespace{ID: 1001}

	// Expect
	mockIntr.EXPECT().GetNamespace("aserto-dev").Return(namespace, nil)
	mockIntr.EXPECT().CreateProject(gomock.Any()).Return(nil, errors.New("failed to create repo"))

	// Act
	err := p.CreateRepo(context.Background(), token, "aserto-dev", "policy")

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "failed to create repo")
}

func TestCreateRepoProtectTagsFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	namespace := &gitlab.Namespace{ID: 1001}
	createdGitlabProj := &gitlab.Project{ID: 654}

	// Expect
	mockIntr.EXPECT().GetNamespace("aserto-dev").Return(namespace, nil)
	mockIntr.EXPECT().CreateProject(gomock.Any()).Return(createdGitlabProj, nil)
	mockIntr.EXPECT().ProtectRepositoryTags(gomock.Any(), gomock.Any()).Return(errors.New("failed to protct tags"))

	// Act
	err := p.CreateRepo(context.Background(), token, "aserto-dev", "policy")

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "failed to protct tags")
}

func TestCreateRepo(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	namespace := &gitlab.Namespace{ID: 1001}
	createdGitlabProj := &gitlab.Project{ID: 654}

	// Expect
	mockIntr.EXPECT().GetNamespace("aserto-dev").Return(namespace, nil)
	mockIntr.EXPECT().CreateProject(gomock.Any()).Return(createdGitlabProj, nil)
	mockIntr.EXPECT().ProtectRepositoryTags(gomock.Any(), gomock.Any()).Return(nil)

	// Act
	err := p.CreateRepo(context.Background(), token, "aserto-dev", "policy")

	// Assert
	assert.NoError(err)
}

func TestInitialTagWithWrongFullName(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}

	// Act
	err := p.InitialTag(context.Background(), token, "aserto-dev", "", "")

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "invalid full gitlab repo name 'aserto-dev', should be in the form owner/repo")
}

func TestInitialTagWithRepoAlreadyTagged(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "dsfcds"}
	tags := []string{"v0.0.0"}
	proj := &gitlab.Project{Name: "policy", WebURL: "gitlab.com/policy", TagList: tags}

	// Expect
	mockIntr.EXPECT().GetProject("aserto-dev/policy").Return(proj, nil, nil)

	// Act
	err := p.InitialTag(context.Background(), token, "aserto-dev/policy", "", "")

	// Assert
	assert.NoError(err)
}

func TestInitialTagFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "dsfcds"}
	tags := []string{}
	proj := &gitlab.Project{ID: 1001, Name: "policy", WebURL: "gitlab.com/policy", TagList: tags}

	// Expect
	mockIntr.EXPECT().GetProject("aserto-dev/policy").Return(proj, nil, nil)
	mockIntr.EXPECT().CreateTag(gomock.Any(), gomock.Any()).Return(errors.New("failed to create tag"))

	// Act
	err := p.InitialTag(context.Background(), token, "aserto-dev/policy", "", "")

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "failed to create tag")
}

func TestInitialTag(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "dsfcds"}
	tags := []string{}
	proj := &gitlab.Project{ID: 1001, Name: "policy", WebURL: "gitlab.com/policy", TagList: tags}

	// Expect
	mockIntr.EXPECT().GetProject("aserto-dev/policy").Return(proj, nil, nil)
	mockIntr.EXPECT().CreateTag(gomock.Any(), gomock.Any()).Return(nil)

	// Act
	err := p.InitialTag(context.Background(), token, "aserto-dev/policy", "", "")

	// Assert
	assert.NoError(err)
}

func TestHasSecretFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}

	// Expect
	mockIntr.EXPECT().
		GetProjectVariable("aserto-dev/policy", "ASERTO_PUSH_KEY").
		Return(nil, nil, errors.New("failed to connect to gitlab"))

	// Act
	secretExists, err := p.HasSecret(context.Background(), token, "aserto-dev", "policy", "ASERTO_PUSH_KEY")

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "failed to connect to gitlab")
	assert.False(secretExists)
}

func TestHasSecretNotFound(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	resp := &gitlab.Response{Response: &http.Response{StatusCode: 404}}

	// Expect
	mockIntr.EXPECT().
		GetProjectVariable("aserto-dev/policy", "ASERTO_PUSH_KEY").
		Return(nil, resp, errors.New("failed to connect to gitlab"))

	// Act
	secretExists, err := p.HasSecret(context.Background(), token, "aserto-dev", "policy", "ASERTO_PUSH_KEY")

	// Assert
	assert.NoError(err)
	assert.False(secretExists)
}

func TestHasSecret(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	variable := &gitlab.ProjectVariable{}

	// Expect
	mockIntr.EXPECT().
		GetProjectVariable("aserto-dev/policy", "ASERTO_PUSH_KEY").
		Return(variable, nil, nil)

	// Act
	secretExists, err := p.HasSecret(context.Background(), token, "aserto-dev", "policy", "ASERTO_PUSH_KEY")

	// Assert
	assert.NoError(err)
	assert.True(secretExists)
}

func TestAddSecretToRepoFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}

	// Expect
	mockIntr.EXPECT().
		GetProjectVariable("aserto-dev/policy", "ASERTO_PUSH_KEY").
		Return(nil, nil, errors.New("failed to connect to gitlab"))

	// Act
	err := p.AddSecretToRepo(context.Background(), token, "aserto-dev", "policy", "ASERTO_PUSH_KEY", "value", false)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "failed to connect to gitlab")
}

func TestAddSecretToRepoOverwriteSecretFalse(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	variable := &gitlab.ProjectVariable{}

	// Expect
	mockIntr.EXPECT().GetProjectVariable("aserto-dev/policy", "ASERTO_PUSH_KEY").Return(variable, nil, nil)

	// Act
	err := p.AddSecretToRepo(context.Background(), token, "aserto-dev", "policy", "ASERTO_PUSH_KEY", "value", false)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "E10022 repo has already been connected to a policy: you're trying to link to an existing repository that already has a secret. Please consider overwriting the Aserto push secret.")
}

func TestAddSecretToRepoOverwriteSecretTrue(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	variable := &gitlab.ProjectVariable{}

	// Expect
	mockIntr.EXPECT().GetProjectVariable("aserto-dev/policy", "ASERTO_PUSH_KEY").Return(variable, nil, nil)
	mockIntr.EXPECT().UpdateProjectVariable("aserto-dev/policy", "ASERTO_PUSH_KEY", gomock.Any()).Return(nil)

	// Act
	err := p.AddSecretToRepo(context.Background(), token, "aserto-dev", "policy", "ASERTO_PUSH_KEY", "value", true)

	// Assert
	assert.NoError(err)
}

func TestAddSecretToRepoNewVariable(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	resp := &gitlab.Response{Response: &http.Response{StatusCode: 404}}

	// Expect
	mockIntr.EXPECT().GetProjectVariable("aserto-dev/policy", "ASERTO_PUSH_KEY").Return(nil, resp, nil)
	mockIntr.EXPECT().CreateProjectVariable("aserto-dev/policy", gomock.Any()).Return(nil)

	// Act
	err := p.AddSecretToRepo(context.Background(), token, "aserto-dev", "policy", "ASERTO_PUSH_KEY", "value", false)

	// Assert
	assert.NoError(err)
}

func TestCommitOnBranchFails(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	content := make(map[string]string)
	content[file] = fileContent
	commit := sources.Commit{
		Branch:  "main",
		Message: "Some commit",
		Owner:   "aserto-dev",
		Repo:    repo,
		Content: content,
	}

	// Expect
	mockIntr.EXPECT().GetProjectFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("failed to connect to server"))
	mockIntr.EXPECT().CreateCommit(gomock.Any(), gomock.Any()).Return("", errors.New("failed to create commit"))

	// Act
	_, err := p.CreateCommitOnBranch(context.Background(), token, &commit)

	// Assert
	assert.Error(err)
	assert.Equal(err.Error(), "failed to create commit")
}

func TestCommitOnBranch(t *testing.T) {
	// Arrange
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	mockintrFunc := newMockIntrFunc(ctrl)
	p := sources.NewTestGitlab(ctrl, &zerolog.Logger{}, &sources.Config{}, mockintrFunc)
	token := &sources.AccessToken{Token: "sometokenvalue"}
	content := make(map[string]string)
	content[file] = fileContent
	commit := sources.Commit{
		Branch:  "main",
		Message: "Some commit",
		Owner:   "aserto-dev",
		Repo:    repo,
		Content: content,
	}
	returnedSha := "sha256"

	// Expect
	mockIntr.EXPECT().GetProjectFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("failed to connect to server"))
	mockIntr.EXPECT().CreateCommit(gomock.Any(), gomock.Any()).Return(returnedSha, nil)

	// Act
	commitSha, err := p.CreateCommitOnBranch(context.Background(), token, &commit)

	// Assert
	assert.NoError(err)
	assert.Equal(returnedSha, commitSha)
}

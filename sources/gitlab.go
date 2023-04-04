package sources

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aserto-dev/go-grpc/aserto/api/v1"
	scc "github.com/aserto-dev/go-grpc/aserto/tenant/scc/v1"
	"github.com/aserto-dev/scc-lib/errx"
	"github.com/aserto-dev/scc-lib/internal/interactions"
	"github.com/friendsofgo/errors"
	"github.com/rs/zerolog"
	"github.com/xanzy/go-gitlab"
)

var (
	_        Source = &gitlabSource{}
	gitlabCI        = "/-/pipelines"
)

// gitlabSource deals with source management on gitlab.com.
type gitlabSource struct {
	logger           *zerolog.Logger
	cfg              *Config
	interactionsFunc interactions.GlIntr
}

func (g *gitlabSource) ValidateConnection(ctx context.Context, accessToken *AccessToken, requiredScopes []string) error {
	client, err := g.interactionsFunc(accessToken.Token)
	if err != nil {
		return errors.Wrap(err, "failed to create Gitlab client")
	}

	_, response, err := client.CurrentUser()
	if err != nil {
		return errors.Wrapf(err, "failed to connect to Gitlab")
	}

	if response.StatusCode != http.StatusOK {
		return errx.ErrProviderVerification.
			Str("status", response.Status).
			Int("status-code", response.StatusCode).
			FromReader("gitlab-response", response.Body).
			Msg("unexpected reply from Gitlab")
	}

	return nil
}

func (g *gitlabSource) Profile(ctx context.Context, accessToken *AccessToken) (string, []*scc.Repo, error) {
	repos := []*scc.Repo{}
	client, err := g.interactionsFunc(accessToken.Token)

	if err != nil {
		return "", repos, errors.Wrap(err, "failed to create Gitlab client")
	}

	user, _, err := client.CurrentUser()
	if err != nil {
		return "", repos, err
	}

	username := user.Username

	opt := &gitlab.ListProjectsOptions{
		ListOptions: gitlab.ListOptions{},
	}

	for {
		projects, resp, err := client.ListUserProjects(username, opt)
		if err != nil {
			return "", repos, err
		}

		for _, proj := range projects {
			// Only add the projects that are owned by the current user.go tool cover -html=coverage.out -o coverage.html
			if proj.Owner == nil || proj.Owner.Username != username {
				continue
			}
			repos = append(repos, &scc.Repo{
				Name:  proj.Name,
				Org:   proj.Owner.Username,
				Url:   proj.WebURL,
				CiUrl: proj.WebURL + gitlabCI,
			})
		}

		if resp.NextPage == 0 {
			break
		}

		opt.Page = resp.NextPage
	}

	return username, repos, nil
}

func (g *gitlabSource) ListOrgs(ctx context.Context, accessToken *AccessToken, page *api.PaginationRequest) ([]*api.SccOrg, *api.PaginationResponse, error) {
	if page == nil {
		return nil, nil, errors.New("page must not be empty")
	}
	if page.Size < -1 || page.Size > 100 {
		return nil, nil, errors.New("page size must be >= -1 and <= 100")
	}

	var orgs []*api.SccOrg
	client, err := g.interactionsFunc(accessToken.Token)

	if err != nil {
		return orgs, nil, errors.Wrap(err, "failed to create Gitlab client")
	}

	pageToRead := 0
	if strings.TrimSpace(page.Token) != "" {
		pageToRead, err = strconv.Atoi(page.Token)
		if err != nil {
			return orgs, nil, errors.Wrap(err, "page token must be int")
		}
	}

	accessLevel := gitlab.DeveloperPermissions
	top := false
	opt := &gitlab.ListGroupsOptions{
		ListOptions:    gitlab.ListOptions{Page: pageToRead, PerPage: int(page.Size)},
		TopLevelOnly:   &top,
		MinAccessLevel: &accessLevel,
	}

	if page.Size == -1 {
		opt.ListOptions.PerPage = 100
	}

	for {

		groups, resp, err := client.ListGroups(opt)
		if err != nil {
			return orgs, nil, err
		}

		for _, group := range groups {
			org := &api.SccOrg{
				Name: group.Name,
				Id:   group.FullPath,
			}
			orgs = append(orgs, org)
		}

		response := &api.PaginationResponse{
			NextToken:  fmt.Sprintf("%d", resp.NextPage),
			ResultSize: int32(len(orgs)),
			TotalSize:  int32(resp.TotalItems),
		}

		if page.Size != -1 {
			return orgs, response, nil
		}
		if resp.NextPage == 0 {
			break
		}

		opt.ListOptions.Page = resp.NextPage
	}

	response := &api.PaginationResponse{
		NextToken:  "",
		ResultSize: int32(len(orgs)),
		TotalSize:  int32(len(orgs)),
	}
	return orgs, response, nil
}

func (g *gitlabSource) ListRepos(ctx context.Context, accessToken *AccessToken, org string, page *api.PaginationRequest) ([]*scc.Repo, *api.PaginationResponse, error) {
	if page == nil {
		return nil, nil, errors.New("page must not be empty")
	}
	if page.Size < -1 || page.Size > 100 {
		return nil, nil, errors.New("page size must be >= -1 and <= 100")
	}
	repos := []*scc.Repo{}
	client, err := g.interactionsFunc(accessToken.Token)

	if err != nil {
		return repos, nil, errors.Wrap(err, "failed to create Gitlab client")
	}

	pageToRead := 0

	if strings.TrimSpace(page.Token) != "" {
		pageToRead, err = strconv.Atoi(page.Token)
		if err != nil {
			return repos, nil, errors.Wrap(err, "page token must be int")
		}
	}

	user, _, err := client.CurrentUser()
	if err != nil {
		return repos, nil, err
	}

	pageSize := int(page.Size)

	listOpt := gitlab.ListOptions{PerPage: pageSize, Page: pageToRead}

	if pageSize == -1 {
		listOpt.PerPage = 100
	}

	if org == user.Username {
		opt := &gitlab.ListProjectsOptions{ListOptions: listOpt}
		return g.listPagedRepos(
			org, pageSize,
			func() ([]*gitlab.Project, *gitlab.Response, error) {
				return client.ListUserProjects(org, opt)
			}, &listOpt)
	}
	opt := &gitlab.ListGroupProjectsOptions{ListOptions: listOpt}
	return g.listPagedRepos(
		org, pageSize, func() ([]*gitlab.Project, *gitlab.Response, error) {
			return client.ListGroupProjects(org, opt)
		}, &listOpt)
}

func (g *gitlabSource) listPagedRepos(
	user string,
	pageSize int,
	lpFunc func() ([]*gitlab.Project, *gitlab.Response, error),
	opt *gitlab.ListOptions,
) ([]*scc.Repo, *api.PaginationResponse, error) {
	repos := []*scc.Repo{}

	for {
		projects, resp, err := lpFunc()
		if err != nil {
			return repos, nil, err
		}

		for _, proj := range projects {
			repos = append(repos, &scc.Repo{
				Name:  proj.Name,
				Org:   user,
				Url:   proj.WebURL,
				CiUrl: proj.WebURL + gitlabCI,
			})
		}

		response := &api.PaginationResponse{
			NextToken:  fmt.Sprintf("%d", resp.NextPage),
			ResultSize: int32(len(repos)),
			TotalSize:  int32(resp.TotalItems),
		}

		if pageSize != -1 {
			return repos, response, nil
		}
		if resp.NextPage == 0 {
			break
		}

		opt.Page = resp.NextPage
	}

	response := &api.PaginationResponse{
		NextToken:  "",
		ResultSize: int32(len(repos)),
		TotalSize:  int32(len(repos)),
	}
	return repos, response, nil
}

func (g *gitlabSource) GetRepo(ctx context.Context, accessToken *AccessToken, owner, repo string) (*scc.Repo, error) {

	resultRepo, _, err := g.getSccRepoWithGitlabProj(accessToken, owner, repo)

	return resultRepo, err
}

func (g *gitlabSource) getSccRepoWithGitlabProj(accessToken *AccessToken, owner, repo string) (*scc.Repo, *gitlab.Project, error) {
	client, err := g.interactionsFunc(accessToken.Token)

	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create Gitlab client")
	}

	var resultRepo *scc.Repo

	repoName := owner + "/" + repo

	proj, _, err := client.GetProject(repoName)
	if err != nil {
		return resultRepo, nil, errors.Wrapf(err, "failed to get project: %s", repoName)
	}

	resultRepo = &scc.Repo{
		Name:  proj.Name,
		Org:   owner,
		Url:   proj.WebURL,
		CiUrl: proj.WebURL + gitlabCI,
	}

	return resultRepo, proj, nil
}

func (g *gitlabSource) CreateRepo(ctx context.Context, accessToken *AccessToken, owner, name string) error {
	client, err := g.interactionsFunc(accessToken.Token)

	if err != nil {
		return errors.Wrap(err, "failed to create Gitlab client")
	}

	visibility := gitlab.PublicVisibility

	namespace, err := client.GetNamespace(owner)
	if err != nil {
		return err
	}

	opt := &gitlab.CreateProjectOptions{
		NamespaceID: &namespace.ID,
		Visibility:  &visibility,
		Name:        &name,
	}

	err = client.CreateProject(opt)

	if err != nil {
		return err
	}

	permission := gitlab.MaintainerPermissions

	protectedTags := "v*"
	protectedTagOpt := &gitlab.ProtectRepositoryTagsOptions{
		Name:              &protectedTags,
		CreateAccessLevel: &permission,
	}

	err = client.ProtectRepositoryTags(owner+"/"+name, protectedTagOpt)

	return err
}

func (g *gitlabSource) InitialTag(ctx context.Context, accessToken *AccessToken, fullName, workflowFileName string) error {
	client, err := g.interactionsFunc(accessToken.Token)

	if err != nil {
		return errors.Wrap(err, "failed to create Gitlab client")
	}

	if strings.Count(fullName, "/") == 0 {
		return errors.Errorf("invalid full gitlab repo name '%s', should be in the form owner/repo", fullName)
	}

	owner := fullName[:strings.LastIndex(fullName, "/")]
	name := fullName[strings.LastIndex(fullName, "/")+1:]

	_, proj, err := g.getSccRepoWithGitlabProj(accessToken, owner, name)

	if err != nil {
		return err
	}

	if len(proj.TagList) > 0 {
		return nil
	}

	opt := &gitlab.CreateTagOptions{
		Ref:     &proj.DefaultBranch,
		TagName: &defaultTag,
		Message: &defaultTag,
	}

	err = client.CreateTag(proj.ID, opt)

	return err
}

func (g *gitlabSource) hasSecret(client interactions.GitlabIntr, orgName, repoName, secretName string) (bool, error) {
	variable, resp, err := client.GetProjectVariable(orgName+"/"+repoName, secretName)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}

	if variable != nil {
		return true, nil
	}

	return false, nil
}

func (g *gitlabSource) HasSecret(ctx context.Context, token *AccessToken, owner, repo, secretName string) (bool, error) {
	client, err := g.interactionsFunc(token.Token)

	if err != nil {
		return false, errors.Wrap(err, "failed to create Gitlab client")
	}

	return g.hasSecret(client, owner, repo, secretName)
}

func (g *gitlabSource) AddSecretToRepo(ctx context.Context, token *AccessToken, orgName, repoName, secretName, value string, overrideSecret bool) error {
	client, err := g.interactionsFunc(token.Token)

	if err != nil {
		return errors.Wrap(err, "failed to create Gitlab client")
	}

	masked := true

	hasSecret, err := g.hasSecret(client, orgName, repoName, secretName)
	if err != nil {
		return err
	}

	if !overrideSecret && hasSecret {
		return errx.ErrRepoAlreadyConnected.Msg("you're trying to link to an existing repository that already has a secret. Please consider overwriting the Aserto push secret.").Str("repo", orgName+"/"+repoName)
	}

	repo := orgName + "/" + repoName

	if hasSecret {
		opt := &gitlab.UpdateProjectVariableOptions{
			Value:     &value,
			Masked:    &masked,
			Protected: &masked,
		}
		err = client.UpdateProjectVariable(repo, secretName, opt)
	} else {
		opt := &gitlab.CreateProjectVariableOptions{
			Key:       &secretName,
			Value:     &value,
			Masked:    &masked,
			Protected: &masked,
		}
		err = client.CreateProjectVariable(repo, opt)
	}

	return err
}

func (g *gitlabSource) CreateCommitOnBranch(ctx context.Context, accessToken *AccessToken, commit *Commit) error {
	client, err := g.interactionsFunc(accessToken.Token)

	if err != nil {
		return errors.Wrap(err, "failed to create Gitlab client")
	}

	var actions []*gitlab.CommitActionOptions

	repo := commit.Owner + "/" + commit.Repo

	for filePath, content := range commit.Content {
		act := gitlab.FileUpdate

		err := client.GetProjectFile(repo, filePath, &gitlab.GetFileOptions{Ref: &commit.Branch})

		if err != nil {
			act = gitlab.FileCreate
		}
		c := content
		f := filePath
		action := &gitlab.CommitActionOptions{
			Action:   &act,
			Content:  &c,
			FilePath: &f,
		}
		actions = append(actions, action)
	}

	opt := &gitlab.CreateCommitOptions{
		Branch:        &commit.Branch,
		CommitMessage: &commit.Message,
		Actions:       actions,
	}

	err = client.CreateCommit(repo, opt)

	return err
}

func (g *gitlabSource) GetDefaultBranch(ctx context.Context, accessToken *AccessToken, owner, repo string) (string, error) {
	_, proj, err := g.getSccRepoWithGitlabProj(accessToken, owner, repo)
	if err != nil {
		return "", err
	}

	return proj.DefaultBranch, nil
}

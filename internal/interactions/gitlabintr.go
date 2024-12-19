package interactions

import (
	"github.com/pkg/errors"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

//go:generate mockgen -source=gitlabintr.go -destination=mock_gitlabintr.go -package=interactions --build_flags=--mod=mod

type GlIntr func(token string) (GitlabIntr, error)

type GitlabIntr interface {
	// GetClient(token string) (GitlabIntr, error)
	CurrentUser() (*gitlab.User, *gitlab.Response, error)
	ListUserProjects(uid interface{}, opt *gitlab.ListProjectsOptions) ([]*gitlab.Project, *gitlab.Response, error)
	ListGroupProjects(gid interface{}, opt *gitlab.ListGroupProjectsOptions) ([]*gitlab.Project, *gitlab.Response, error)
	ListGroups(opt *gitlab.ListGroupsOptions) ([]*gitlab.Group, *gitlab.Response, error)
	GetProject(pid interface{}) (*gitlab.Project, *gitlab.Response, error)
	GetNamespace(id interface{}) (*gitlab.Namespace, error)
	CreateProject(opt *gitlab.CreateProjectOptions) (*gitlab.Project, error)
	ProtectRepositoryTags(pid interface{}, opt *gitlab.ProtectRepositoryTagsOptions) error
	CreateTag(pid interface{}, opt *gitlab.CreateTagOptions) error
	GetProjectVariable(pid interface{}, key string) (*gitlab.ProjectVariable, *gitlab.Response, error)
	UpdateProjectVariable(pid interface{}, key string, opt *gitlab.UpdateProjectVariableOptions) error
	CreateProjectVariable(pid interface{}, opt *gitlab.CreateProjectVariableOptions) error
	GetProjectFile(pid interface{}, fileName string, opt *gitlab.GetFileOptions) error
	CreateCommit(pid interface{}, opt *gitlab.CreateCommitOptions) (string, error)
}

type gitlabInteraction struct {
	Client *gitlab.Client
}

func NewGitlabInteraction() GlIntr {
	return func(token string) (GitlabIntr, error) {
		client, err := gitlab.NewClient(token)

		if err != nil {
			return nil, errors.Wrap(err, "failed to create Gitlab client")
		}

		return &gitlabInteraction{Client: client}, nil
	}
}

func (gi *gitlabInteraction) GetClient(token string) (GitlabIntr, error) {
	client, err := gitlab.NewClient(token)

	if err != nil {
		return nil, errors.Wrap(err, "failed to create Gitlab client")
	}

	return &gitlabInteraction{Client: client}, nil
}

func (gi *gitlabInteraction) CurrentUser() (*gitlab.User, *gitlab.Response, error) {
	return gi.Client.Users.CurrentUser()
}

func (gi *gitlabInteraction) ListUserProjects(uid interface{}, opt *gitlab.ListProjectsOptions) ([]*gitlab.Project, *gitlab.Response, error) {
	return gi.Client.Projects.ListUserProjects(uid, opt)
}

func (gi *gitlabInteraction) ListGroupProjects(gid interface{}, opt *gitlab.ListGroupProjectsOptions) ([]*gitlab.Project, *gitlab.Response, error) {
	return gi.Client.Groups.ListGroupProjects(gid, opt)
}

func (gi *gitlabInteraction) ListGroups(opt *gitlab.ListGroupsOptions) ([]*gitlab.Group, *gitlab.Response, error) {
	return gi.Client.Groups.ListGroups(opt)
}

func (gi *gitlabInteraction) GetProject(pid interface{}) (*gitlab.Project, *gitlab.Response, error) {
	return gi.Client.Projects.GetProject(pid, nil)
}

func (gi *gitlabInteraction) GetNamespace(id interface{}) (*gitlab.Namespace, error) {
	namespace, _, err := gi.Client.Namespaces.GetNamespace(id)
	return namespace, err
}

func (gi *gitlabInteraction) CreateProject(opt *gitlab.CreateProjectOptions) (*gitlab.Project, error) {
	proj, _, err := gi.Client.Projects.CreateProject(opt)
	return proj, err
}

func (gi *gitlabInteraction) ProtectRepositoryTags(pid interface{}, opt *gitlab.ProtectRepositoryTagsOptions) error {
	_, _, err := gi.Client.ProtectedTags.ProtectRepositoryTags(pid, opt)
	return err
}

func (gi *gitlabInteraction) CreateTag(pid interface{}, opt *gitlab.CreateTagOptions) error {
	_, _, err := gi.Client.Tags.CreateTag(pid, opt)
	return err
}

func (gi *gitlabInteraction) GetProjectVariable(pid interface{}, key string) (*gitlab.ProjectVariable, *gitlab.Response, error) {
	return gi.Client.ProjectVariables.GetVariable(pid, key, nil)
}

func (gi *gitlabInteraction) UpdateProjectVariable(pid interface{}, key string, opt *gitlab.UpdateProjectVariableOptions) error {
	_, _, err := gi.Client.ProjectVariables.UpdateVariable(pid, key, opt)
	return err
}

func (gi *gitlabInteraction) CreateProjectVariable(pid interface{}, opt *gitlab.CreateProjectVariableOptions) error {
	_, _, err := gi.Client.ProjectVariables.CreateVariable(pid, opt)
	return err
}

func (gi *gitlabInteraction) GetProjectFile(pid interface{}, fileName string, opt *gitlab.GetFileOptions) error {
	_, _, err := gi.Client.RepositoryFiles.GetFile(pid, fileName, opt)
	return err
}

func (gi *gitlabInteraction) CreateCommit(pid interface{}, opt *gitlab.CreateCommitOptions) (string, error) {
	commit, _, err := gi.Client.Commits.CreateCommit(pid, opt)
	if err != nil {
		return "", err
	}
	return commit.ID, err
}

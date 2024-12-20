package interactions

import (
	"context"
	"errors"
	"time"

	"github.com/aserto-dev/scc-lib/errx"
	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"
)

//go:generate mockgen -source=githubintr.go -destination=mock_githubintr.go -package=interactions --build_flags=--mod=mod

type GhIntr func(ctx context.Context, token, tokenType string, rateLimitTimeout, retryCount int) GithubIntr

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
	ListRepositoryWorkflowRuns(context.Context, string, string, *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, error)
	CreateWorkflowDispatchEventByFileName(context.Context, string, string, string, github.CreateWorkflowDispatchEventRequest) error
	CreateFile(ctx context.Context, owner, repo, path string, opts *github.RepositoryContentFileOptions) (*github.RepositoryContentResponse, error)
	GetCommit(ctx context.Context, owner, repo, sha string) (*github.Commit, error)
}

type githubInteraction struct {
	Client            *github.Client
	retryLimitTimeout int
	retryCount        int
}

func NewGithubInteraction() GhIntr {
	return func(ctx context.Context, token, tokenType string, retryLimitTimeout, retryCount int) GithubIntr {
		tokenSource := oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: token,
				TokenType:   tokenType,
			},
		)
		clientWithToken := oauth2.NewClient(ctx, tokenSource)

		githubClient := github.NewClient(clientWithToken)

		return &githubInteraction{
			Client:            githubClient,
			retryLimitTimeout: retryLimitTimeout,
			retryCount:        retryCount,
		}
	}
}

func (gh *githubInteraction) GetCommit(ctx context.Context, owner, repo, sha string) (*github.Commit, error) {
	var err error
	var commit *github.Commit

	err = gh.withSecondaryRateLimitRetry(func() error {
		commit, _, err = gh.Client.Git.GetCommit(ctx, owner, repo, sha)
		return err
	})

	return commit, err
}

func (gh *githubInteraction) GetUsers(ctx context.Context, username string) (*github.User, *github.Response, error) {
	var user *github.User
	var resp *github.Response
	var err error

	err = gh.withSecondaryRateLimitRetry(func() error {
		user, resp, err = gh.Client.Users.Get(ctx, username)
		return err
	})

	return user, resp, err
}

func (gh *githubInteraction) ListRepoSecrets(ctx context.Context, owner, repo string, opts *github.ListOptions) (*github.Secrets, error) {
	var secrets *github.Secrets
	var err error

	err = gh.withSecondaryRateLimitRetry(func() error {
		secrets, _, err = gh.Client.Actions.ListRepoSecrets(ctx, owner, repo, opts)
		return err
	})
	return secrets, err
}

func (gh *githubInteraction) GetRepoPublicKey(ctx context.Context, org, repo string) (*github.PublicKey, error) {
	var key *github.PublicKey
	var err error

	err = gh.withSecondaryRateLimitRetry(func() error {
		key, _, err = gh.Client.Actions.GetRepoPublicKey(ctx, org, repo)
		return err
	})

	return key, err
}

func (gh *githubInteraction) CreateOrUpdateRepoSecret(ctx context.Context, org, repo string, secret *github.EncryptedSecret) (*github.Response, error) {
	var response *github.Response
	var err error

	err = gh.withSecondaryRateLimitRetry(func() error {
		response, err = gh.Client.Actions.CreateOrUpdateRepoSecret(ctx, org, repo, secret)
		return err
	})
	return response, err
}

func (gh *githubInteraction) GetRepo(ctx context.Context, owner, repo string) (*github.Repository, error) {
	var repoResult *github.Repository
	var err error

	err = gh.withSecondaryRateLimitRetry(func() error {
		repoResult, _, err = gh.Client.Repositories.Get(ctx, owner, repo)
		return err
	})
	return repoResult, err
}

func (gh *githubInteraction) CreateRepo(ctx context.Context, owner string, repo *github.Repository) error {
	var err error

	err = gh.withSecondaryRateLimitRetry(func() error {
		_, _, err = gh.Client.Repositories.Create(ctx, owner, repo)
		return err
	})
	return err
}

func (gh *githubInteraction) ListRepoTags(ctx context.Context, owner, repo string, opts *github.ListOptions) ([]*github.RepositoryTag, error) {
	var tags []*github.RepositoryTag
	var err error
	err = gh.withSecondaryRateLimitRetry(func() error {
		tags, _, err = gh.Client.Repositories.ListTags(ctx, owner, repo, opts)
		return err
	})
	return tags, err
}

func (gh *githubInteraction) GetRepoRef(ctx context.Context, owner, repo, ref string) (*github.Reference, *github.Response, error) {
	var reference *github.Reference
	var response *github.Response
	var err error
	err = gh.withSecondaryRateLimitRetry(func() error {
		reference, response, err = gh.Client.Git.GetRef(ctx, owner, repo, ref)
		return err
	})
	return reference, response, err
}

func (gh *githubInteraction) CreateRepoTag(ctx context.Context, owner, repo string, tag *github.Tag) (*github.Tag, error) {
	var tagResult *github.Tag
	var err error
	err = gh.withSecondaryRateLimitRetry(func() error {
		tagResult, _, err = gh.Client.Git.CreateTag(ctx, owner, repo, tag)
		return err
	})
	return tagResult, err
}

func (gh *githubInteraction) CreateRepoRef(ctx context.Context, owner, repo string, ref *github.Reference) error {
	var err error
	err = gh.withSecondaryRateLimitRetry(func() error {
		_, _, err = gh.Client.Git.CreateRef(ctx, owner, repo, ref)
		return err
	})
	return err
}

func (gh *githubInteraction) ListRepositoryWorkflowRuns(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, error) {
	var runs *github.WorkflowRuns
	var err error
	err = gh.withSecondaryRateLimitRetry(func() error {
		runs, _, err = gh.Client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
		return err
	})
	return runs, err
}

func (gh *githubInteraction) CreateWorkflowDispatchEventByFileName(ctx context.Context, owner, repo, fileNameWorkflow string, event github.CreateWorkflowDispatchEventRequest) error {
	var err error
	err = gh.withSecondaryRateLimitRetry(func() error {
		_, err = gh.Client.Actions.CreateWorkflowDispatchEventByFileName(ctx, owner, repo, fileNameWorkflow, event)
		return err
	})
	return err
}

func (gh *githubInteraction) CreateFile(ctx context.Context, owner, repo, path string, opts *github.RepositoryContentFileOptions) (*github.RepositoryContentResponse, error) {
	var contentResponse *github.RepositoryContentResponse
	var err error
	err = gh.withSecondaryRateLimitRetry(func() error {
		contentResponse, _, err = gh.Client.Repositories.CreateFile(ctx, owner, repo, path, opts)
		return err
	})
	return contentResponse, err
}

func (gh *githubInteraction) withSecondaryRateLimitRetry(f func() error) (err error) {
	timeout := time.Duration(gh.retryLimitTimeout) * time.Second
	tryCount := 0
retryLoop:
	for t := time.After(timeout); ; {
		tryCount++
		err = f()
		if err == nil {
			return nil
		}

		var ghErr *github.AbuseRateLimitError
		if errors.As(err, &ghErr) {
			time.Sleep(*ghErr.RetryAfter)
		} else {
			return err
		}

		if tryCount >= gh.retryCount {
			return errx.ErrRetryTimeout.Msg("reached retry limit")
		}

		select {
		case <-t:
			break retryLoop
		default:
		}

	}

	return errx.ErrRetryTimeout.Err(err)
}

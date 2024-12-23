package sources

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aserto-dev/go-grpc/aserto/api/v1"
	scc "github.com/aserto-dev/go-grpc/aserto/tenant/scc/v1"
	"github.com/aserto-dev/scc-lib/errx"
	"github.com/aserto-dev/scc-lib/internal/interactions"
	"github.com/aserto-dev/scc-lib/retry"
	"github.com/google/go-github/v66/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/oauth2"
	"k8s.io/utils/ptr"
)

var (
	_        Source = &githubSource{}
	githubCI        = "/actions"

	ErrEmptyRepo      = errors.New("repository is not initialized")
	ErrCommitNotFound = errors.New("commit not found")
)

// githubSource deals with source management on github.com.
type githubSource struct {
	logger           *zerolog.Logger
	cfg              *Config
	interactionsFunc interactions.GhIntr
	graphqlFunc      interactions.GqlIntr
}

func (g *githubSource) ValidateConnection(ctx context.Context, accessToken *AccessToken, requiredScopes []string) error {
	githubClient := g.interactionsFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)

	_, response, err := githubClient.GetUsers(ctx, "")

	if err != nil {
		return errors.Wrap(err, "failed to connect to Github")
	}

	if response.StatusCode != http.StatusOK {
		return errx.ErrProviderVerification.
			Str("status", response.Status).
			Int("status-code", response.StatusCode).
			FromReader("github-response", response.Body).
			Msg("unexpected reply from GitHub")
	}

	if len(requiredScopes) == 0 {
		return nil
	}

	foundScopes := map[string]bool{}
	scopeSlice := strings.Split(response.Header.Get("X-OAuth-Scopes"), ",")
	for _, es := range scopeSlice {
		for _, rs := range requiredScopes {
			r, err := regexp.Compile(rs)
			if err != nil {
				return errx.ErrProviderVerification.Err(err).Msgf("failed to compile regexp: %s", err.Error())
			}
			if r.MatchString(strings.TrimSpace(es)) {
				foundScopes[rs] = true
				break
			}
		}
	}
	if len(foundScopes) != len(requiredScopes) {
		return errx.ErrProviderVerification.
			Interface("provided-scopes", scopeSlice).
			Interface("required-scopes", requiredScopes).
			Msg("github access token is missing scopes")
	}

	return nil
}

// Profile returns the username of the user that owns the token, and its associated repos.
func (g *githubSource) Profile(ctx context.Context, accessToken *AccessToken) (string, []*scc.Repo, error) {
	client := g.graphqlFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)

	repos := []*scc.Repo{}
	username := ""
	var cursor graphql.String

	for {
		var query struct {
			Viewer struct {
				Login        graphql.String
				Repositories struct {
					Nodes []struct {
						Name  graphql.String
						Owner struct {
							Login graphql.String
						}
						URL graphql.String
					}
					PageInfo struct {
						HasNextPage graphql.Boolean
						EndCursor   graphql.String
					}
				} `graphql:"repositories(first: $first after: $after ownerAffiliations:[OWNER])"`
			}
		}
		vars := map[string]interface{}{
			"first": graphql.Int(100),
		}

		if cursor != "" {
			vars["after"] = cursor
		} else {
			vars["after"] = (*graphql.String)(nil)
		}

		err := client.Query(ctx, &query, vars)
		if err != nil {
			return "", nil, errors.Wrap(err, "error running query against github graphql server")
		}

		username = string(query.Viewer.Login)
		for _, r := range query.Viewer.Repositories.Nodes {
			repos = append(repos, &scc.Repo{
				Name:  string(r.Name),
				Org:   string(r.Owner.Login),
				Url:   string(r.URL),
				CiUrl: string(r.URL) + githubCI,
			})
		}

		if !query.Viewer.Repositories.PageInfo.HasNextPage {
			break
		}

		cursor = query.Viewer.Repositories.PageInfo.EndCursor
	}

	return username, repos, nil
}

func (g *githubSource) HasSecret(ctx context.Context, accessToken *AccessToken, owner, repo, secretName string) (bool, error) {
	githubClient := g.interactionsFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)

	return g.hasSecret(ctx, githubClient, owner, repo, secretName)
}

func (g *githubSource) AddSecretToRepo(ctx context.Context, accessToken *AccessToken, orgName, repoName, secretName, value string, overrideSecret bool) error {
	githubClient := g.interactionsFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)

	if orgName == "" {
		return errors.New("No org name was provided")
	}

	if repoName == "" {
		return errors.New("No repo name was provided")
	}

	var pk *github.PublicKey
	var err error
	err = retry.Retry(time.Duration(g.cfg.CreateRepoTimeoutSeconds)*time.Second, func(i int) error {
		pk, err = githubClient.GetRepoPublicKey(ctx, orgName, repoName)
		return err
	})

	if err != nil {
		return errors.Wrap(err, "failed to get public repo key for encryption")
	}

	encryptedString, err := encryptSecretWithPublicKey(pk, value)
	if err != nil {
		return errors.Wrap(err, "failed to encrypt secret for github actions")
	}

	if !overrideSecret {
		hasSecret, err := g.hasSecret(ctx, githubClient, orgName, repoName, secretName)
		if err != nil {
			return err
		}
		if hasSecret {
			return errx.ErrRepoAlreadyConnected.Msg("you’re trying to link to an existing repository that already has a secret. Please consider overwriting the Aserto push secret.").Str("repo", orgName+"/"+repoName)
		}
	}

	var response *github.Response
	err = retry.Retry(time.Duration(g.cfg.CreateRepoTimeoutSeconds)*time.Second, func(i int) error {
		response, err = githubClient.CreateOrUpdateRepoSecret(ctx, orgName, repoName, &github.EncryptedSecret{
			Name:           secretName,
			EncryptedValue: encryptedString,
			KeyID:          pk.GetKeyID(),
		})

		return err
	})

	if err != nil {
		return errx.ErrGithubSecret.Err(err).Str("repo", orgName+"/"+repoName).Str("secret-name", secretName).FromReader("github-response", response.Body)
	}

	return nil
}

// ListOrgs lists all orgs the user is a part of.
func (g *githubSource) ListOrgs(ctx context.Context, accessToken *AccessToken, page *api.PaginationRequest) ([]*api.SccOrg, *api.PaginationResponse, error) {
	if page == nil {
		return nil, nil, errors.New("page must not be empty")
	}
	client := g.graphqlFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)

	var result []*api.SccOrg

	var query struct {
		Viewer struct {
			Organizations struct {
				Nodes []struct {
					Login graphql.String
				}
				PageInfo struct {
					HasNextPage graphql.Boolean
					EndCursor   graphql.String
				}
				TotalCount graphql.Int
			} `graphql:"organizations(first: $first after: $after)"`
		}
	}

	if page.Size < -1 || page.Size > 100 {
		return nil, nil, errors.New("page size must be >= -1 and <= 100")
	}

	vars := map[string]interface{}{
		"first": graphql.Int(page.Size),
	}

	if page.Token != "" {
		vars["after"] = graphql.String(page.Token)
	} else {
		vars["after"] = (*graphql.String)(nil)
	}

	if page.Size == -1 {
		vars["first"] = graphql.Int(100)
	}

	for {
		err := client.Query(ctx, &query, vars)
		if err != nil {
			return nil, nil, errors.Wrap(err, "error running query against github graphql server")
		}

		for _, o := range query.Viewer.Organizations.Nodes {
			org := string(o.Login)
			sccOrg := &api.SccOrg{
				Name: org,
				Id:   org,
			}
			result = append(result, sccOrg)
		}

		resp := &api.PaginationResponse{
			NextToken:  string(query.Viewer.Organizations.PageInfo.EndCursor),
			ResultSize: int32(len(result)), // nolint: gosec
			TotalSize:  int32(query.Viewer.Organizations.TotalCount),
		}

		if page.Size != -1 {
			return result, resp, nil
		}

		if !query.Viewer.Organizations.PageInfo.HasNextPage {
			break
		}
		vars["after"] = query.Viewer.Organizations.PageInfo.EndCursor
	}

	resp := &api.PaginationResponse{
		NextToken:  "",
		ResultSize: int32(len(result)), // nolint: gosec
		TotalSize:  int32(len(result)), // nolint: gosec
	}

	return result, resp, nil
}

// ListRepos lists all repos for an owner.
func (g *githubSource) ListRepos(ctx context.Context, accessToken *AccessToken, owner string, page *api.PaginationRequest) ([]*scc.Repo, *api.PaginationResponse, error) {
	if page == nil {
		return nil, nil, errors.New("page must not be empty")
	}
	if page.Size < -1 || page.Size > 100 {
		return nil, nil, errors.New("page size must be >= -1 and <= 100")
	}
	result := []*scc.Repo{}

	client := g.graphqlFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)

	var query struct {
		Search struct {
			PageInfo struct {
				HasNextPage graphql.Boolean
				EndCursor   graphql.String
			}
			RepositoryCount graphql.Int
			Edges           []struct {
				Node struct {
					Repository struct {
						Name  graphql.String
						Owner struct {
							Login graphql.String
						}
						URL graphql.String
					} `graphql:"... on Repository"`
				}
			}
		} `graphql:"search(query:$org type:REPOSITORY first:$first after:$after)"`
	}

	vars := map[string]interface{}{
		"first": graphql.Int(page.Size),
		"org":   "org:" + graphql.String(owner),
	}

	if page.Token != "" {
		vars["after"] = graphql.String(page.Token)
	} else {
		vars["after"] = (*graphql.String)(nil)
	}

	if page.Size == -1 {
		vars["first"] = graphql.Int(100)
	}

	for {

		err := client.Query(ctx, &query, vars)

		if err != nil {
			return nil, nil, errors.Wrap(err, "error running query against github graphql server")
		}

		for _, r := range query.Search.Edges {
			result = append(result, &scc.Repo{
				Name:  string(r.Node.Repository.Name),
				Org:   string(r.Node.Repository.Owner.Login),
				Url:   string(r.Node.Repository.URL),
				CiUrl: string(r.Node.Repository.URL) + githubCI,
			})
		}

		resp := &api.PaginationResponse{
			NextToken:  string(query.Search.PageInfo.EndCursor),
			ResultSize: int32(len(result)), // nolint: gosec
			TotalSize:  int32(query.Search.RepositoryCount),
		}

		if page.Size != -1 {
			return result, resp, nil
		}

		if !query.Search.PageInfo.HasNextPage {
			break
		}
		vars["after"] = query.Search.PageInfo.EndCursor
	}

	resp := &api.PaginationResponse{
		NextToken:  "",
		ResultSize: int32(len(result)), // nolint: gosec
		TotalSize:  int32(len(result)), // nolint: gosec
	}

	return result, resp, nil
}

func (g *githubSource) GetRepo(ctx context.Context, accessToken *AccessToken, owner, repo string) (*scc.Repo, error) {
	result := &scc.Repo{}

	githubClient := g.interactionsFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)

	gitRepo, err := githubClient.GetRepo(ctx, owner, repo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get repo")
	}

	result.Name = *gitRepo.Name
	result.Org = *gitRepo.Owner.Login
	result.Url = *gitRepo.HTMLURL
	result.CiUrl = *gitRepo.HTMLURL + githubCI

	return result, err
}

func (g *githubSource) CreateRepo(ctx context.Context, accessToken *AccessToken, owner, name string) error {
	githubClient := g.interactionsFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)

	user, _, err := githubClient.GetUsers(ctx, "")
	if err != nil {
		return errors.Wrap(err, "failed to read user from github")
	}

	if *user.Login == owner {
		owner = ""
	}

	err = githubClient.CreateRepo(ctx, owner, &github.Repository{
		Name:     &name,
		AutoInit: ptr.To(true),
	})
	if err != nil {
		return errors.Wrap(err, "failed to create repo")
	}

	return nil
}

// InitialTag creates a tag for a repo, if no other tags are defined for it.
func (g *githubSource) InitialTag(ctx context.Context, accessToken *AccessToken, fullName, workflowFileName, commitSha string) error {
	githubClient := g.interactionsFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)
	repoPieces := strings.Split(fullName, "/")
	if len(repoPieces) != 2 {
		return errors.Errorf("invalid full github repo name '%s', should be in the form owner/repo", fullName)
	}

	owner := repoPieces[0]
	name := repoPieces[1]

	client := g.graphqlFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)

	repo, err := githubClient.GetRepo(ctx, owner, name)
	if err != nil {
		return errors.Wrap(err, "failed to get repo")
	}

	if commitSha == "" {
		tags, err := githubClient.ListRepoTags(ctx, owner, name, &github.ListOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to list tags for repo '%s/%s'", owner, name)
		}

		if len(tags) > 0 {
			return nil
		}

		ref, response, err := githubClient.GetRepoRef(ctx, owner, name, "heads/"+*repo.DefaultBranch)
		if err != nil {
			return errors.Wrapf(err, "repo seems to be empty; response code from github [%d]", response.StatusCode)
		}
		commitSha = *ref.Object.SHA
	}

	var mutation struct {
		CreateRef struct {
			Ref struct {
				ID string
			}
		} `graphql:"createRef(input: $input)"`
	}

	input := githubv4.CreateRefInput{
		RepositoryID: githubv4.ID(repo.NodeID),
		Name:         githubv4.String("refs/tags/" + defaultTag),
		Oid:          githubv4.GitObjectID(commitSha),
	}

	err = client.Mutate(ctx, &mutation, input, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create commit")
	}

	if workflowFileName != "" {
		g.logger.Warn().Msgf("trigger manual dispatch for [%s] if a workflow run doesn't exist", workflowFileName)
		return g.forceRerunWorkflow(ctx, githubClient, owner, name, workflowFileName)
	}
	return nil
}

func (g *githubSource) forceRerunWorkflow(ctx context.Context, githubClient interactions.GithubIntr, owner, name, workflowFileName string) error {
	err := retry.Retry(time.Second*time.Duration(g.cfg.WaitTagTimeoutSeconds), func(i int) error {
		runs, err := githubClient.ListRepositoryWorkflowRuns(ctx, owner, name, &github.ListWorkflowRunsOptions{})
		if err != nil {
			return err
		}
		if runs == nil || len(runs.WorkflowRuns) == 0 {
			return errors.New("No workflows were triggered")
		}
		return nil
	})

	if err != nil {
		event := github.CreateWorkflowDispatchEventRequest{
			Ref: defaultTag,
		}
		g.logger.Debug().Msgf("triggering workflow dispatch event for [%s]", workflowFileName)
		err = githubClient.CreateWorkflowDispatchEventByFileName(ctx, owner, name, workflowFileName, event)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *githubSource) CreateCommitOnBranch(ctx context.Context, accessToken *AccessToken, commit *Commit) (string, error) {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: accessToken.Token,
			TokenType:   accessToken.Type,
		},
	)
	httpClient := oauth2.NewClient(ctx, src)
	client := githubv4.NewClient(httpClient)

	var filePath, content string
	for path, cont := range commit.Content {
		filePath = path
		content = cont
		break
	}

	var query struct {
		Repository struct {
			Ref struct {
				Target struct {
					Oid githubv4.String
				}
			} `graphql:"ref(qualifiedName: $qualifiedName)"`
			Object struct {
				Blob struct {
					Text githubv4.String
				} `graphql:"... on Blob"`
			} `graphql:"object(expression: $expression)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	variables := map[string]interface{}{
		"owner":         githubv4.String(commit.Owner),
		"repo":          githubv4.String(commit.Repo),
		"qualifiedName": githubv4.String(commit.Branch),
		"expression":    githubv4.String(fmt.Sprintf("HEAD:%s", filePath)),
	}

	var mutation struct {
		CreateCommitOnBranch struct {
			Commit struct {
				OID string
			}
		} `graphql:"createCommitOnBranch(input: $input)"`
	}

	err := retry.Retry(time.Second*time.Duration(g.cfg.CreateRepoTimeoutSeconds), func(i int) error {
		err := client.Query(ctx, &query, variables)
		if err != nil {
			return errors.Wrap(err, "failed to query latest commit")
		}

		ref := query.Repository.Ref.Target.Oid
		if ref == "" {
			return errors.Wrapf(ErrEmptyRepo, "%s/%s", commit.Owner, commit.Repo)
		}

		configContent := query.Repository.Object.Blob.Text

		mutationVariables := createCommitOnBranchInput(ref, commit)

		if configContent != "" && configContent == githubv4.String(content) {
			return nil
		}

		err = client.Mutate(ctx, &mutation, mutationVariables, nil)
		if err != nil {
			return errors.Wrap(err, "failed to create commit")
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	if mutation.CreateCommitOnBranch.Commit.OID == "" {
		return "", nil
	}

	return g.waitForCommit(ctx, accessToken, commit.Owner, commit.Repo, mutation.CreateCommitOnBranch.Commit.OID)
}

func createCommitOnBranchInput(ref githubv4.String, commit *Commit) githubv4.CreateCommitOnBranchInput {
	branch := githubv4.String(commit.Branch)
	repoNameWithOwner := githubv4.String(fmt.Sprintf("%s/%s", commit.Owner, commit.Repo))

	oid := githubv4.GitObjectID(ref)
	input := githubv4.CreateCommitOnBranchInput{
		Branch: githubv4.CommittableBranch{
			BranchName:              &branch,
			RepositoryNameWithOwner: &repoNameWithOwner,
		},
		ExpectedHeadOid: oid,
		Message: githubv4.CommitMessage{
			Headline: githubv4.String(commit.Message),
		},
		FileChanges: &githubv4.FileChanges{},
	}

	var adds []githubv4.FileAddition

	for filePath, content := range commit.Content {
		encodeContent := base64.StdEncoding.EncodeToString([]byte(content))
		adds = append(adds, githubv4.FileAddition{
			Path:     githubv4.String(filePath),
			Contents: githubv4.Base64String(encodeContent),
		})
	}

	if len(adds) > 0 {
		input.FileChanges.Additions = &adds
	}

	return input
}

func (g *githubSource) hasSecret(ctx context.Context, githubClient interactions.GithubIntr, owner, repo, secretName string) (bool, error) {
	var existingSecrets *github.Secrets
	err := retry.Retry(time.Duration(g.cfg.CreateRepoTimeoutSeconds)*time.Second, func(i int) error {
		var err error
		existingSecrets, err = githubClient.ListRepoSecrets(ctx, owner, repo, &github.ListOptions{})
		return err
	})
	if err != nil {
		return false, errors.Wrap(err, "failed to list repo secrets")
	}

	for _, secret := range existingSecrets.Secrets {
		if secret.Name == secretName {
			return true, nil
		}
	}

	return false, nil
}

func (g *githubSource) GetDefaultBranch(ctx context.Context, accessToken *AccessToken, owner, repo string) (string, error) {
	githubClient := g.interactionsFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)

	gitRepo, err := githubClient.GetRepo(ctx, owner, repo)
	if err != nil {
		return "", errors.Wrap(err, "failed to get repo")
	}

	return *gitRepo.DefaultBranch, nil
}

func (g *githubSource) waitForCommit(ctx context.Context, accessToken *AccessToken, owner, repo, sha string) (string, error) {
	githubClient := g.interactionsFunc(ctx, accessToken.Token, accessToken.Type, g.cfg.RateLimitTimeoutSeconds, g.cfg.RateLimitRetryCount)

	err := retry.Retry(time.Duration(g.cfg.WaitTagTimeoutSeconds)*time.Second, func(i int) error {
		commit, err := githubClient.GetCommit(ctx, owner, repo, sha)
		if err != nil {
			return err
		}

		if *commit.SHA != sha {
			return errors.Wrapf(ErrCommitNotFound, "last commit is not %s", sha)
		}

		return nil
	})

	return sha, err
}

var errUnableToDecodePKey = errors.New("base64.StdEncoding.DecodeString was unable to decode public key")

func encryptSecretWithPublicKey(publicKey *github.PublicKey, secretValue string) (string, error) {
	decodedPublicKey, err := base64.StdEncoding.DecodeString(publicKey.GetKey())
	if err != nil {
		return "", fmt.Errorf("%w : %v", errUnableToDecodePKey, err)
	}

	var pkRaw [32]byte

	// publicKey.GetKey() can return empty string here, should I return err in this case?
	if len(decodedPublicKey) >= 32 {
		copy(pkRaw[:], decodedPublicKey[0:32])
	}
	copy(pkRaw[:], decodedPublicKey)

	encryptedBytes, err := box.SealAnonymous(nil, []byte(secretValue), &pkRaw, nil)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt secret: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(encryptedBytes)

	return encoded, nil
}

package sources

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aserto-dev/go-grpc/aserto/api/v1"
	scc "github.com/aserto-dev/go-grpc/aserto/tenant/scc/v1"
	"github.com/aserto-dev/go-utils/cerr"
	"github.com/aserto-dev/go-utils/retry"
	"github.com/google/go-github/v33/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/oauth2"
	"k8s.io/utils/pointer"
)

var _ Source = &githubSource{}

// githubSource deals with source management on github.com
type githubSource struct {
	logger *zerolog.Logger
	cfg    *Config
}

// NewGithub creates a new Github
func NewGithub(log *zerolog.Logger, cfg *Config) Source {
	ghLogger := log.With().Str("component", "github-provider").Logger()

	return &githubSource{
		cfg:    cfg,
		logger: &ghLogger,
	}
}

func (g *githubSource) ValidateConnection(ctx context.Context, accessToken *AccessToken) error {
	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: accessToken.Token,
			TokenType:   accessToken.Type,
		},
	)
	clientWithToken := oauth2.NewClient(ctx, tokenSource)

	githubClient := github.NewClient(clientWithToken)
	_, response, err := githubClient.Users.Get(ctx, "")
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		return cerr.ErrProviderVerification.
			Str("status", response.Status).
			Int("status-code", response.StatusCode).
			FromReader("github-response", response.Body).
			Err(err).
			Msgf("unexpected reply from GitHub: %s", err.Error())
	}

	return nil
}

// Profile returns the username of the user that owns the token, and its associated repos
func (g *githubSource) Profile(ctx context.Context, accessToken *AccessToken) (string, []*scc.Repo, error) {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: accessToken.Token,
			TokenType:   accessToken.Type,
		},
	)
	httpClient := oauth2.NewClient(ctx, src)
	client := graphql.NewClient("https://api.github.com/graphql", httpClient)

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
				Name: string(r.Name),
				Org:  string(r.Owner.Login),
				Url:  string(r.URL),
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
	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: accessToken.Token,
			TokenType:   accessToken.Type,
		},
	)
	clientWithToken := oauth2.NewClient(ctx, tokenSource)

	githubClient := github.NewClient(clientWithToken)

	return g.hasSecret(ctx, githubClient, owner, repo, secretName)
}

func (g *githubSource) AddSecretToRepo(ctx context.Context, accessToken *AccessToken, orgName, repoName, secretName, value string, overrideSecret bool) error {
	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: accessToken.Token,
			TokenType:   accessToken.Type,
		},
	)
	clientWithToken := oauth2.NewClient(ctx, tokenSource)

	githubClient := github.NewClient(clientWithToken)

	if orgName == "" {
		return errors.New("No org name was provided")
	}

	if repoName == "" {
		return errors.New("No repo name was provided")
	}

	var pk *github.PublicKey
	var err error
	err = retry.Retry(time.Duration(g.cfg.CreateRepoTimeoutSeconds)*time.Second, func(i int) error {
		pk, _, err = githubClient.Actions.GetRepoPublicKey(ctx, orgName, repoName)
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
			return cerr.ErrRepoAlreadyConnected.Msg("you’re trying to link to an existing repository that already has a secret. Please consider overwriting the Aserto push secret.").Str("repo", orgName+"/"+repoName)
		}
	}

	var response *github.Response
	err = retry.Retry(time.Duration(g.cfg.CreateRepoTimeoutSeconds)*time.Second, func(i int) error {
		response, err = githubClient.Actions.CreateOrUpdateRepoSecret(ctx, orgName, repoName, &github.EncryptedSecret{
			Name:           secretName,
			EncryptedValue: encryptedString,
			KeyID:          pk.GetKeyID(),
		})

		return err
	})

	if err != nil {
		return cerr.ErrGithubSecret.Err(err).Str("repo", orgName+"/"+repoName).Str("secret-name", secretName).FromReader("github-response", response.Body)
	}

	return nil
}

// ListOrgs lists all orgs the user is a part of
func (g *githubSource) ListOrgs(ctx context.Context, accessToken *AccessToken, page *api.PaginationRequest) ([]*api.SccOrg, *api.PaginationResponse, error) {
	if page == nil {
		return nil, nil, errors.New("page must not be empty")
	}
	src := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: accessToken.Token,
			TokenType:   accessToken.Type,
		},
	)
	httpClient := oauth2.NewClient(ctx, src)
	client := graphql.NewClient("https://api.github.com/graphql", httpClient)

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
			ResultSize: int32(len(result)),
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
		ResultSize: int32(len(result)),
		TotalSize:  int32(len(result)),
	}

	return result, resp, nil
}

// ListRepos lists all repos for an owner
func (g *githubSource) ListRepos(ctx context.Context, accessToken *AccessToken, owner string, page *api.PaginationRequest) ([]*scc.Repo, *api.PaginationResponse, error) {
	if page == nil {
		return nil, nil, errors.New("page must not be empty")
	}
	if page.Size < -1 || page.Size > 100 {
		return nil, nil, errors.New("page size must be >= -1 and <= 100")
	}
	result := []*scc.Repo{}

	src := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: accessToken.Token,
			TokenType:   accessToken.Type,
		},
	)
	httpClient := oauth2.NewClient(ctx, src)
	client := graphql.NewClient("https://api.github.com/graphql", httpClient)

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
				Name: string(r.Node.Repository.Name),
				Org:  string(r.Node.Repository.Owner.Login),
				Url:  string(r.Node.Repository.URL),
			})
		}

		resp := &api.PaginationResponse{
			NextToken:  string(query.Search.PageInfo.EndCursor),
			ResultSize: int32(len(result)),
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
		ResultSize: int32(len(result)),
		TotalSize:  int32(len(result)),
	}

	return result, resp, nil
}

func (g *githubSource) GetRepo(ctx context.Context, accessToken *AccessToken, owner, repo string) (*scc.Repo, error) {
	result := &scc.Repo{}

	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: accessToken.Token,
			TokenType:   accessToken.Type,
		},
	)
	clientWithToken := oauth2.NewClient(ctx, tokenSource)

	githubClient := github.NewClient(clientWithToken)

	gitRepo, _, err := githubClient.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get repo")
	}

	result.Name = *gitRepo.Name
	result.Org = *gitRepo.Owner.Login
	result.Url = *gitRepo.HTMLURL

	return result, err
}

func (g *githubSource) CreateRepo(ctx context.Context, accessToken *AccessToken, owner, name string) error {
	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: accessToken.Token,
			TokenType:   accessToken.Type,
		},
	)
	clientWithToken := oauth2.NewClient(ctx, tokenSource)
	githubClient := github.NewClient(clientWithToken)

	user, _, err := githubClient.Users.Get(ctx, "")
	if err != nil {
		return errors.Wrap(err, "failed to read user from github")
	}

	if *user.Login == owner {
		owner = ""
	}

	_, _, err = githubClient.Repositories.Create(ctx, owner, &github.Repository{
		Name:     &name,
		AutoInit: pointer.BoolPtr(true),
	})
	if err != nil {
		return errors.Wrap(err, "failed to create repo")
	}

	return nil
}

// InitialTag creates a tag for a repo, if no other tags are defined for it
func (g *githubSource) InitialTag(ctx context.Context, accessToken *AccessToken, fullName string) error {
	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: accessToken.Token,
			TokenType:   accessToken.Type,
		},
	)
	clientWithToken := oauth2.NewClient(ctx, tokenSource)
	githubClient := github.NewClient(clientWithToken)

	repoPieces := strings.Split(fullName, "/")
	if len(repoPieces) != 2 {
		return errors.Errorf("invalid full github repo name '%s', should be in the form owner/repo", fullName)
	}

	owner := repoPieces[0]
	name := repoPieces[1]

	repo, _, err := githubClient.Repositories.Get(ctx, owner, name)
	if err != nil {
		return errors.Wrap(err, "failed to get repo")
	}

	tags, _, err := githubClient.Repositories.ListTags(ctx, owner, name, &github.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list tags for repo '%s/%s'", owner, name)
	}

	if len(tags) > 0 {
		return nil
	}

	err = retry.Retry(time.Second*time.Duration(g.cfg.CreateRepoTimeoutSeconds), func(i int) error {
		ref, response, err := githubClient.Git.GetRef(ctx, owner, name, "heads/"+*repo.DefaultBranch)
		if err != nil {
			return errors.Wrapf(err, "repo seems to be empty; response code from github [%d]", response.StatusCode)
		}

		tag, _, err := githubClient.Git.CreateTag(ctx, owner, name, &github.Tag{
			Tag:     &defaultTag,
			Message: &defaultTag,
			SHA:     ref.Object.SHA,
			Object:  ref.Object,
		})
		if err != nil {
			return errors.Wrap(err, "failed to create repo tag")
		}

		tagRef := "refs/tags/" + *tag.Tag
		_, _, err = githubClient.Git.CreateRef(ctx, owner, name, &github.Reference{
			Ref:    &tagRef,
			Object: tag.Object,
		})
		if err != nil {
			return errors.Wrap(err, "failed to create tag ref")
		}
		return nil
	})

	return err
}

func (g *githubSource) CreateCommitOnBranch(ctx context.Context, accessToken *AccessToken, commit *Commit) error {
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

	err := retry.Retry(time.Second*time.Duration(g.cfg.CreateRepoTimeoutSeconds), func(i int) error {
		err := client.Query(ctx, &query, variables)
		if err != nil {
			return errors.Wrap(err, "failed to query latest commit")
		}
		ref := query.Repository.Ref.Target.Oid
		configContent := query.Repository.Object.Blob.Text

		var mutation struct {
			CreateCommitOnBranch struct {
				Commit struct {
					OID string
				}
			} `graphql:"createCommitOnBranch(input: $input)"`
		}

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

	return err
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

func (g *githubSource) hasSecret(ctx context.Context, githubClient *github.Client, owner, repo, secretName string) (bool, error) {
	var existingSecrets *github.Secrets
	err := retry.Retry(time.Duration(g.cfg.CreateRepoTimeoutSeconds)*time.Second, func(i int) error {
		var err error
		existingSecrets, _, err = githubClient.Actions.ListRepoSecrets(ctx, owner, repo, &github.ListOptions{})
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

	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: accessToken.Token,
			TokenType:   accessToken.Type,
		},
	)
	clientWithToken := oauth2.NewClient(ctx, tokenSource)

	githubClient := github.NewClient(clientWithToken)

	gitRepo, _, err := githubClient.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return "", errors.Wrap(err, "failed to get repo")
	}

	return *gitRepo.DefaultBranch, nil
}

func encryptSecretWithPublicKey(publicKey *github.PublicKey, secretValue string) (string, error) {
	decodedPublicKey, err := base64.StdEncoding.DecodeString(publicKey.GetKey())
	if err != nil {
		return "", fmt.Errorf("base64.StdEncoding.DecodeString was unable to decode public key: %v", err)
	}
	var pkRaw [32]byte
	copy(pkRaw[:], decodedPublicKey[0:32])

	encryptedBytes, err := box.SealAnonymous(nil, []byte(secretValue), &pkRaw, nil)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt secret: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(encryptedBytes)

	return encoded, nil
}

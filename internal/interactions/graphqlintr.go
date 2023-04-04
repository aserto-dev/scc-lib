package interactions

import (
	"context"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

//go:generate mockgen -source=graphqlintr.go -destination=mock_graphqlintr.go -package=interactions --build_flags=--mod=mod
type GqlIntr func(ctx context.Context, token, tokenType string, retryLimitTimeout, retryCount int) GraphqlIntr

type GraphqlIntr interface {
	Query(context.Context, interface{}, map[string]interface{}) error
	Mutate(context.Context, interface{}, githubv4.Input, map[string]interface{}) error
}

type graphqlInteraction struct {
	Client *githubv4.Client
}

func NewGraphqlInteraction() GqlIntr {
	return func(ctx context.Context, token, tokenType string, retryLimitTimeout, retryCount int) GraphqlIntr {
		src := oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: token,
				TokenType:   tokenType,
			},
		)

		retryClient := retryablehttp.NewClient()
		retryClient.Backoff = retryablehttp.DefaultBackoff
		retryClient.RetryWaitMin = time.Millisecond * 5
		retryClient.RetryWaitMax = time.Second * time.Duration(retryLimitTimeout)
		retryClient.RetryMax = retryCount

		httpClient := oauth2.NewClient(
			context.WithValue(ctx, oauth2.HTTPClient, retryClient.StandardClient()),
			src,
		)

		client := githubv4.NewClient(httpClient)

		return &graphqlInteraction{Client: client}
	}
}

func (g *graphqlInteraction) Query(ctx context.Context, query interface{}, vars map[string]interface{}) error {
	return g.Client.Query(ctx, query, vars)
}

func (g *graphqlInteraction) Mutate(ctx context.Context, m interface{}, input githubv4.Input, variables map[string]interface{}) error {
	return g.Client.Mutate(ctx, m, input, variables)
}

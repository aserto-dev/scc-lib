package interactions

import (
	"context"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/shurcooL/graphql"
	"golang.org/x/oauth2"
)

//go:generate mockgen -source=graphqlintr.go -destination=mock_graphqlintr.go -package=interactions --build_flags=--mod=mod
type GraphQLIntr func(ctx context.Context, token, tokenType string, retryLimitTimeout, retryCount int) GraphqlIntr

type GraphqlIntr interface {
	Query(context.Context, interface{}, map[string]interface{}) error
}

type graphqlInteraction struct {
	Client            *graphql.Client
	retryLimitTimeout int
	retryCount        int
}

func NewGraphqInteraction() GraphQLIntr {
	return func(ctx context.Context, token, tokenType string, retryLimitTimeout, retryCount int) GraphqlIntr {
		src := oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: token,
				TokenType:   tokenType,
			},
		)

		retryClient := retryablehttp.NewClient()
		retryClient.Backoff = retryablehttp.DefaultBackoff
		retryClient.RetryWaitMin = time.Second * 1
		retryClient.RetryWaitMax = time.Second * time.Duration(retryLimitTimeout)
		retryClient.RetryMax = retryCount

		httpClient := oauth2.NewClient(
			context.WithValue(ctx, oauth2.HTTPClient, retryClient.StandardClient()),
			src,
		)

		client := graphql.NewClient("https://api.github.com/graphql", httpClient)

		return &graphqlInteraction{Client: client, retryLimitTimeout: retryLimitTimeout, retryCount: retryCount}
	}
}

func (g *graphqlInteraction) Query(ctx context.Context, query interface{}, vars map[string]interface{}) error {
	return g.Client.Query(ctx, query, vars)
}

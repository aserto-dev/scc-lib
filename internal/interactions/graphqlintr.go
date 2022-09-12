package interactions

import (
	"context"

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
		httpClient := oauth2.NewClient(ctx, src)
		client := graphql.NewClient("https://api.github.com/graphql", httpClient)

		return &graphqlInteraction{Client: client, retryLimitTimeout: retryLimitTimeout, retryCount: retryCount}
	}
}

func (g *graphqlInteraction) Query(ctx context.Context, query interface{}, vars map[string]interface{}) error {
	return g.Client.QueryRetry(ctx, query, vars, g.retryLimitTimeout, g.retryCount)
}

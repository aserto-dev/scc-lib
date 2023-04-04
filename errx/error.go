package errx

import (
	"net/http"

	cerr "github.com/aserto-dev/errors"
	"google.golang.org/grpc/codes"
)

var (
	// Returned if an SCC repository has already been referenced in a policy.
	ErrRepoAlreadyConnected = cerr.NewAsertoError("E10022", codes.AlreadyExists, http.StatusConflict, "repo has already been connected to a policy")
	// Returned if there was a problem setting up a Github secret.
	ErrGithubSecret = cerr.NewAsertoError("E10023", codes.Unavailable, http.StatusServiceUnavailable, "failed to setup repo secret")
	// Returned when a provider verification call has failed.
	ErrProviderVerification = cerr.NewAsertoError("E10030", codes.InvalidArgument, http.StatusBadRequest, "verification failed")
	// Returned when an operation timed out after multiple retries.
	ErrRetryTimeout = cerr.NewAsertoError("E10034", codes.DeadlineExceeded, http.StatusRequestTimeout, "timeout after multiple retries")
)

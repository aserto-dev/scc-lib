package retry_test

import (
	"errors"
	"testing"
	"time"

	"github.com/aserto-dev/scc-lib/retry"
	"github.com/stretchr/testify/require"
)

var errNope = errors.New("nope")

func TestRetry(t *testing.T) {
	assert := require.New(t)

	worked := false
	err := retry.Retry(5*time.Second, func(i int) error {
		if i == 2 {
			worked = true
			return nil
		}

		return errNope
	})

	assert.NoError(err)
	assert.True(worked)
}

func TestRetryOnce(t *testing.T) {
	assert := require.New(t)

	var iteration int
	err := retry.Retry(0, func(i int) error {
		iteration = i

		return errNope
	})

	assert.Error(err)
	assert.Equal(iteration, 1)
}

func TestRetryOnceNoErr(t *testing.T) {
	assert := require.New(t)

	var iteration int
	err := retry.Retry(0, func(i int) error {
		iteration = i

		return nil
	})

	assert.NoError(err)
	assert.Equal(iteration, 1)
}

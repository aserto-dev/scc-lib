package retry

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetry(t *testing.T) {
	assert := require.New(t)

	worked := false
	err := Retry(5*time.Second, func(i int) error {
		if i == 2 {
			worked = true
			return nil
		}

		return errors.New("nope")
	})

	assert.NoError(err)
	assert.True(worked)
}

func TestRetryOnce(t *testing.T) {
	assert := require.New(t)

	var iteration int
	err := Retry(0, func(i int) error {
		iteration = i

		return errors.New("nope")
	})

	assert.Error(err)
	assert.Equal(iteration, 1)
}

func TestRetryOnceNoErr(t *testing.T) {
	assert := require.New(t)

	var iteration int
	err := Retry(0, func(i int) error {
		iteration = i

		return nil
	})

	assert.NoError(err)
	assert.Equal(iteration, 1)
}

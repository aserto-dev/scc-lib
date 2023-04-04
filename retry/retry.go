package retry

import (
	"time"

	"github.com/aserto-dev/scc-lib/errx"
	"github.com/jpillora/backoff"
)

// Retry retries to run the given function until it returns no error.
// Returns an error after `timeout`.
// It uses an exponential backoff for retries, with a min of 10ms, max of 5 seconds and a factor of 1.5.
// Uses jitter to randomize sleep durations, to avoid contention. See more here:  github.com/jpillora/backoff
// If the duration is set to 0, it run the given function once.
func Retry(timeout time.Duration, f func(int) error) (err error) {
	b := &backoff.Backoff{
		Min:    10 * time.Millisecond,
		Max:    5 * time.Second,
		Factor: 1.5,
		Jitter: true,
	}

	attempt := 1

	if timeout == 0 {
		err = f(attempt)
		if err != nil {
			return errx.ErrRetryTimeout.Err(err)
		}
		return nil
	}

retryLoop:
	for t := time.After(timeout); ; {
		select {
		case <-t:
			break retryLoop
		default:
		}

		err = f(attempt)
		if err == nil {
			return
		}

		attempt++
		time.Sleep(b.Duration())
	}

	return errx.ErrRetryTimeout.Err(err)
}

package startup

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"
)

type BackoffOpts struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

var DefaultBackoff = BackoffOpts{
	InitialDelay: 1 * time.Second,
	MaxDelay:     30 * time.Second,
}

// RetryConnect calls connect repeatedly with exponential backoff until it
// succeeds or ctx is cancelled. Each failed attempt is logged.
func RetryConnect(ctx context.Context, name string, connect func() error, opts BackoffOpts) error {
	delay := opts.InitialDelay
	if delay <= 0 {
		delay = time.Second
	}
	maxDelay := opts.MaxDelay
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}

	for attempt := 1; ; attempt++ {
		err := connect()
		if err == nil {
			if attempt > 1 {
				log.Printf("%s: connected on attempt %d", name, attempt)
			}
			return nil
		}

		sleep := delay + time.Duration(rand.Int63n(int64(delay/4)))
		log.Printf("%s: attempt %d failed: %v (retrying in %v)", name, attempt, err, sleep)

		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: abandoned after %d attempts: %w", name, attempt, ctx.Err())
		case <-time.After(sleep):
		}

		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

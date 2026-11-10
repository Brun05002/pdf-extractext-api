package services

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultMaxAttempts = 3
	defaultRetryBase   = 200 * time.Millisecond
	maxRetryDelay      = 2 * time.Second
)

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

func retryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func retryDelay(attempt int, retryAfter string) time.Duration {
	if parsed := parseRetryAfter(retryAfter); parsed > 0 {
		return parsed
	}
	delay := defaultRetryBase << max(attempt-1, 0)
	if delay > maxRetryDelay {
		return maxRetryDelay
	}
	return delay
}

func parseRetryAfter(raw string) time.Duration {
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(raw); err == nil {
		if remaining := time.Until(when); remaining > 0 {
			return remaining
		}
	}
	return 0
}

func sleepBeforeRetry(ctx context.Context, attempt int, retryAfter string) error {
	delay := retryDelay(attempt, retryAfter)
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
		return context.DeadlineExceeded
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxUpstreamErrorBytes))
	_ = body.Close()
}

func doWithRetry(ctx context.Context, client *http.Client, newRequest func() (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= defaultMaxAttempts; attempt++ {
		req, err := newRequest()
		if err != nil {
			return nil, err
		}

		resp, err := client.Do(req)
		if err == nil {
			if attempt < defaultMaxAttempts && retryableStatus(resp.StatusCode) {
				retryAfter := resp.Header.Get("Retry-After")
				drainAndClose(resp.Body)
				if sleepErr := sleepBeforeRetry(ctx, attempt, retryAfter); sleepErr != nil {
					return nil, sleepErr
				}
				continue
			}
			return resp, nil
		}

		lastErr = err
		if !retryableError(err) || attempt == defaultMaxAttempts {
			return nil, err
		}
		if err := sleepBeforeRetry(ctx, attempt, ""); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

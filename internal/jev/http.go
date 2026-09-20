package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxAttempts          = 3
	maxResponseBody      = 4 << 20
	defaultRetryDelay    = 500 * time.Millisecond
	maxRetryDelay        = 5 * time.Second
	defaultRetryJitter   = 0.5
	retryJitterReduction = 0.25
)

var errRedirect = errors.New("redirects are not allowed")

type httpConfig struct {
	client *http.Client
	getenv func(string) string
	wait   func(context.Context, time.Duration) error
	jitter func() float64
}

type httpCore struct{ config httpConfig }

func defaultHTTPConfig() httpConfig {
	return httpConfig{
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return errRedirect
		}},
		getenv: func(name string) string { return strings.TrimSpace(os.Getenv(name)) },
		wait: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
		jitter: func() float64 { return defaultRetryJitter },
	}
}

func (core httpCore) postJSON(ctx context.Context, endpoint, credentialEnv string, payload any, decode func([]byte) error) (int, error) {
	key := strings.TrimSpace(core.config.getenv(credentialEnv))
	if key == "" {
		return 0, &Error{Class: ErrorConfiguration, Op: "authenticate", Err: fmt.Errorf("%s is not set", credentialEnv)}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, &Error{Class: ErrorResponse, Op: "encode request", Err: err}
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		response, requestErr := core.doRequest(ctx, endpoint, key, body)
		if requestErr != nil {
			retry, attempts, err := core.handleRequestError(ctx, requestErr, attempt)
			if retry {
				continue
			}

			return attempts, err
		}

		retry, responseErr := core.handleResponse(ctx, response, attempt, decode)
		if responseErr != nil {
			return attempt, responseErr
		}

		if retry {
			continue
		}

		return attempt, nil
	}

	return maxAttempts, &Error{Class: ErrorProvider, Op: "request"}
}

func (core httpCore) handleRequestError(ctx context.Context, requestErr error, attempt int) (bool, int, error) {
	var classified *Error
	if errors.As(requestErr, &classified) {
		return false, attempt - 1, classified
	}

	if errors.Is(requestErr, errRedirect) {
		return false, attempt, &Error{Class: ErrorProvider, Op: "redirect", Err: errRedirect}
	}

	class := contextClass(ctx, requestErr)
	if attempt < maxAttempts && (class == ErrorTransport || (class == ErrorTimeout && ctx.Err() == nil)) {
		waitErr := core.waitRetry(ctx, attempt, "", "")
		if waitErr == nil {
			return true, attempt, nil
		}

		class = contextClass(ctx, waitErr)
		requestErr = waitErr
	}

	return false, attempt, &Error{Class: class, Op: "request", Err: requestErr}
}

func (core httpCore) doRequest(ctx context.Context, endpoint, key string, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, &Error{Class: ErrorConfiguration, Op: "build request", Err: err}
	}

	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	return core.config.client.Do(request)
}

func (core httpCore) handleResponse(
	ctx context.Context,
	response *http.Response,
	attempt int,
	decode func([]byte) error,
) (bool, error) {
	data, readErr := readBounded(response.Body)
	response.Body.Close()

	if readErr != nil {
		return false, &Error{Class: ErrorResponse, Op: "read response", Err: readErr}
	}

	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		if err := decode(data); err != nil {
			return false, &Error{Class: ErrorResponse, Op: "decode response", Err: err}
		}

		return false, nil
	}

	if attempt < maxAttempts && retryableStatus(response.StatusCode) {
		waitErr := core.waitRetry(ctx, attempt, response.Header.Get("Retry-After"), response.Header.Get("retry-after-ms"))
		if waitErr == nil {
			return true, nil
		}

		return false, &Error{Class: contextClass(ctx, waitErr), Op: "retry", Err: waitErr}
	}

	return false, &Error{Class: statusClass(response.StatusCode), Op: "request", Code: response.StatusCode}
}

func (core httpCore) waitRetry(ctx context.Context, attempt int, retryAfter, retryAfterMS string) error {
	delay := retryDelay(time.Now(), retryAfter, retryAfterMS)
	if delay < 0 {
		delay = defaultRetryDelay * time.Duration(1<<(attempt-1))
		if delay > maxRetryDelay {
			delay = maxRetryDelay
		}

		jitter := core.config.jitter()
		if jitter < 0 {
			jitter = 0
		} else if jitter > 1 {
			jitter = 1
		}

		delay -= time.Duration(float64(delay) * retryJitterReduction * jitter)
	}

	if deadline, ok := ctx.Deadline(); ok && time.Now().Add(delay).After(deadline) {
		return context.DeadlineExceeded
	}

	return core.config.wait(ctx, delay)
}

func retryDelay(now time.Time, retryAfter, retryAfterMS string) time.Duration {
	if value := strings.TrimSpace(retryAfterMS); value != "" {
		milliseconds, err := strconv.ParseInt(value, 10, 64)
		if err == nil && milliseconds >= 0 {
			return time.Duration(milliseconds) * time.Millisecond
		}
	}

	if value := strings.TrimSpace(retryAfter); value != "" {
		if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
			return time.Duration(seconds) * time.Second
		}

		if date, err := http.ParseTime(value); err == nil {
			delay := date.Sub(now)
			if delay < 0 {
				return 0
			}

			return delay
		}
	}

	return -1
}

func readBounded(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxResponseBody+1))
	if err != nil {
		return nil, err
	}

	if len(data) > maxResponseBody {
		return nil, errors.New("response exceeds 4 MiB limit")
	}

	return data, nil
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func statusClass(status int) ErrorClass {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrorAuthentication
	case http.StatusTooManyRequests:
		return ErrorRateLimit
	default:
		return ErrorProvider
	}
}

func contextClass(ctx context.Context, err error) ErrorClass {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return ErrorCanceled
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrorTimeout
	}

	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return ErrorTimeout
	}

	return ErrorTransport
}

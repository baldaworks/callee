package evaluation

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type timeoutError struct{}

func (timeoutError) Error() string   { return "temporary timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestHTTPCoreRetriesTransientStatusesAndHonorsHeader(t *testing.T) {
	t.Parallel()

	attempts := 0

	var delays []time.Duration

	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		attempts++

		if got := request.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q", got)
		}

		switch attempts {
		case 1:
			return response(http.StatusInternalServerError, `{}`), nil
		case 2:
			value := response(http.StatusTooManyRequests, `{}`)
			value.Header.Set("retry-after-ms", "25")

			return value, nil
		default:
			return response(http.StatusOK, `{"ok":true}`), nil
		}
	})
	core := httpCore{config: httpConfig{
		client: &http.Client{Transport: transport},
		getenv: func(string) string { return "secret" },
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)

			return nil
		},
		jitter: func() float64 { return 0 },
	}}

	var decoded bool

	gotAttempts, err := core.postJSON(context.Background(), "https://example.test/evaluate", "API_KEY", map[string]any{"x": 1}, func(data []byte) error {
		decoded = string(data) == `{"ok":true}`

		return nil
	})
	if err != nil {
		t.Fatalf("postJSON() error = %v", err)
	}

	if gotAttempts != 3 || attempts != 3 || !decoded {
		t.Fatalf("attempts = (%d, %d), decoded = %v", gotAttempts, attempts, decoded)
	}

	if len(delays) != 2 || delays[0] != 500*time.Millisecond || delays[1] != 25*time.Millisecond {
		t.Fatalf("delays = %v", delays)
	}
}

func TestHTTPCoreRetriesTransientConnectionAndTimeoutErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		failure   error
		wantClass ErrorClass
	}{
		{name: "connection", failure: errors.New("connection reset"), wantClass: ErrorTransport},
		{name: "timeout", failure: timeoutError{}, wantClass: ErrorTimeout},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			core := httpCore{config: httpConfig{
				client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					if calls < maxAttempts {
						return nil, test.failure
					}

					return response(http.StatusOK, `{}`), nil
				})},
				getenv: func(string) string { return "secret" },
				wait:   func(context.Context, time.Duration) error { return nil },
				jitter: func() float64 { return 0 },
			}}

			attempts, err := core.postJSON(context.Background(), "https://example.test/evaluate", "API_KEY", struct{}{}, func([]byte) error { return nil })
			if err != nil || attempts != maxAttempts || calls != maxAttempts {
				t.Fatalf("postJSON() = (attempts %d, calls %d, error %v)", attempts, calls, err)
			}

			failingCore := core
			failingCore.config.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, test.failure
			})}

			_, err = failingCore.postJSON(context.Background(), "https://example.test/evaluate", "API_KEY", struct{}{}, func([]byte) error { return nil })

			var classified *Error
			if !errors.As(err, &classified) || classified.Class != test.wantClass {
				t.Fatalf("postJSON() terminal error = %v, want class %q", err, test.wantClass)
			}
		})
	}
}

func TestHTTPCoreStopsForCancellationAndRetryBeyondDeadline(t *testing.T) {
	t.Parallel()

	t.Run("canceled request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		calls := 0
		core := httpCore{config: httpConfig{
			client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++

				return nil, request.Context().Err()
			})},
			getenv: func(string) string { return "secret" },
			wait: func(context.Context, time.Duration) error {
				t.Fatal("canceled request must not retry")

				return nil
			},
			jitter: func() float64 { return 0 },
		}}

		_, err := core.postJSON(ctx, "https://example.test/evaluate", "API_KEY", struct{}{}, func([]byte) error { return nil })

		var classified *Error
		if !errors.As(err, &classified) || classified.Class != ErrorCanceled || calls != 1 {
			t.Fatalf("postJSON() = (error %v, calls %d), want canceled after one call", err, calls)
		}
	})

	t.Run("retry delay exceeds deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		calls := 0
		core := httpCore{config: httpConfig{
			client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				value := response(http.StatusTooManyRequests, `{}`)
				value.Header.Set("Retry-After", "10")

				return value, nil
			})},
			getenv: func(string) string { return "secret" },
			wait: func(context.Context, time.Duration) error {
				t.Fatal("retry outside the deadline must not wait")

				return nil
			},
			jitter: func() float64 { return 0 },
		}}

		_, err := core.postJSON(ctx, "https://example.test/evaluate", "API_KEY", struct{}{}, func([]byte) error { return nil })

		var classified *Error
		if !errors.As(err, &classified) || classified.Class != ErrorTimeout || calls != 1 {
			t.Fatalf("postJSON() = (error %v, calls %d), want timeout after one call", err, calls)
		}
	})
}

func TestHTTPCoreFailsClosedWithoutLeakingBodiesOrCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		credential string
		status     int
		body       string
		wantClass  ErrorClass
		wantCalls  int
	}{
		{name: "missing credential", wantClass: ErrorConfiguration, wantCalls: 0},
		{name: "authentication", credential: "top-secret", status: http.StatusUnauthorized, body: "sensitive-provider-body", wantClass: ErrorAuthentication, wantCalls: 1},
		{name: "client error", credential: "top-secret", status: http.StatusBadRequest, body: "sensitive-provider-body", wantClass: ErrorProvider, wantCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			core := httpCore{config: httpConfig{
				client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++

					return response(test.status, test.body), nil
				})},
				getenv: func(string) string { return test.credential },
				wait:   func(context.Context, time.Duration) error { return nil },
				jitter: func() float64 { return 0 },
			}}

			_, err := core.postJSON(context.Background(), "https://example.test/evaluate", "TEST_API_KEY", struct{}{}, func([]byte) error { return nil })

			var classified *Error
			if !errors.As(err, &classified) || classified.Class != test.wantClass {
				t.Fatalf("postJSON() error = %v, want class %q", err, test.wantClass)
			}

			if calls != test.wantCalls {
				t.Fatalf("calls = %d, want %d", calls, test.wantCalls)
			}

			message := err.Error()
			if (test.body != "" && strings.Contains(message, test.body)) || (test.credential != "" && strings.Contains(message, test.credential)) {
				t.Fatalf("error leaks sensitive value: %q", message)
			}
		})
	}
}

func TestHTTPCoreRejectsOversizedResponse(t *testing.T) {
	t.Parallel()

	core := httpCore{config: httpConfig{
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(io.LimitReader(strings.NewReader(strings.Repeat("x", maxResponseBody+1)), maxResponseBody+1))}, nil
		})},
		getenv: func(string) string { return "secret" },
		wait:   func(context.Context, time.Duration) error { return nil },
		jitter: func() float64 { return 0 },
	}}

	_, err := core.postJSON(context.Background(), "https://example.test/evaluate", "API_KEY", struct{}{}, func([]byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "4 MiB") {
		t.Fatalf("postJSON() error = %v", err)
	}
}

func TestHTTPCoreRejectsRedirectWithoutRetry(t *testing.T) {
	t.Parallel()

	calls := 0

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++

		http.Redirect(writer, request, "/elsewhere", http.StatusFound)
	}))
	defer server.Close()

	config := defaultHTTPConfig()
	config.getenv = func(string) string { return "secret" }
	config.wait = func(context.Context, time.Duration) error {
		t.Fatal("redirect must not retry")

		return nil
	}
	_, err := (httpCore{config: config}).postJSON(context.Background(), server.URL, "API_KEY", struct{}{}, func([]byte) error { return nil })

	var classified *Error
	if !errors.As(err, &classified) || classified.Class != ErrorProvider || calls != 1 {
		t.Fatalf("postJSON() = (%v, calls %d), want provider error after one call", err, calls)
	}
}

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

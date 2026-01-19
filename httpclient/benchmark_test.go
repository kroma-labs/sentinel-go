package httpclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// BenchmarkStandardClient measures the baseline performance of the standard http.Client.
func BenchmarkStandardClient(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	client := ts.Client()
	ctx := context.Background()
	url := ts.URL

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// BenchmarkSentinelClient_Default measures the performance of the Sentinel client with default configuration.
func BenchmarkSentinelClient_Default(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	// Disable chaos, retries, etc. to correct baseline, or use default options
	client := New(
		WithDisableNetworkTrace(), // Disable tracing for pure overhead measurement
	)
	ctx := context.Background()
	url := ts.URL

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := client.Request("Benchmark").Get(ctx, url)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		// httpclient.Response reads the body automatically into memory by default
		// unless we use GetResponse() style or similar.
		// The Response struct has a Body() method that returns []byte.
		_, _ = resp.Body()
	}
}

// BenchmarkSentinelClient_WithBreaker measures overhead of the Circuit Breaker.
func BenchmarkSentinelClient_WithBreaker(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	client := New(
		WithDisableNetworkTrace(),
		WithBreakerConfig(DefaultBreakerConfig()),
	)
	ctx := context.Background()
	url := ts.URL

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := client.Request("Benchmark").Get(ctx, url)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		_, _ = resp.Body()
	}
}

// BenchmarkSentinelClient_WithRateLimit measures overhead of Rate Limiting.
func BenchmarkSentinelClient_WithRateLimit(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	client := New(
		WithDisableNetworkTrace(),
		WithRateLimit(RateLimitConfig{
			RequestsPerSecond: float64(rate.Inf), // Cast to float64
		}),
	)
	ctx := context.Background()
	url := ts.URL

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := client.Request("Benchmark").Get(ctx, url)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		_, _ = resp.Body()
	}
}

// BenchmarkSentinelClient_WithRetry measures overhead of Retry transport (success case).
func BenchmarkSentinelClient_WithRetry(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	client := New(
		WithDisableNetworkTrace(),
		WithRetryConfig(DefaultRetryConfig()),
	)
	ctx := context.Background()
	url := ts.URL

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := client.Request("Benchmark").Get(ctx, url)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		_, _ = resp.Body()
	}
}

// BenchmarkSentinelClient_FullChain measures overhead of the full feature set.
func BenchmarkSentinelClient_FullChain(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	client := New(
		WithDisableNetworkTrace(),
		WithRetryConfig(DefaultRetryConfig()),
		WithBreakerConfig(DefaultBreakerConfig()),
		WithRateLimit(RateLimitConfig{RequestsPerSecond: float64(rate.Inf)}),
	)
	ctx := context.Background()
	url := ts.URL

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := client.Request("Benchmark").
			Header("X-Test", "value").
			Get(ctx, url)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		_, _ = resp.Body()
	}
}

// BenchmarkSentinelClient_Coalescing measures request coalescing overhead/benefit.
func BenchmarkSentinelClient_Coalescing(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	client := New(WithDisableNetworkTrace())
	ctx := context.Background()
	url := ts.URL

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Request("Benchmark").
				Coalesce().
				Get(ctx, url)
			if err != nil {
				continue
			}
			_, _ = resp.Body()
		}
	})
}

// BenchmarkBuilder_Allocation measures allocation overhead of the fluent builder.
func BenchmarkBuilder_Allocation(b *testing.B) {
	client := New(WithDisableNetworkTrace())
	ctx := context.Background()
	url := "http://localhost" // Not actually called

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		client.Request("Benchmark").
			Path("/test").
			Query("q", "value").
			Header("X-Key", "val").
			Body(bytes.NewReader([]byte("body"))).
			Coalesce().
			RateLimit(100).
			Timeout(5 * time.Second)
		_ = ctx
		_ = url
	}
}

// BenchmarkSentinelClient_WithInterceptors measures overhead of request/response interceptors.
func BenchmarkSentinelClient_WithInterceptors(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	client := New(
		WithDisableNetworkTrace(),
		WithRequestInterceptor(
			func(req *http.Request) error {
				req.Header.Set("X-Intercepted", "true")
				return nil
			},
		),
		WithResponseInterceptor(
			func(resp *http.Response, _ *http.Request) error {
				// Simulate some check
				if resp.StatusCode != http.StatusOK {
					return nil
				}
				return nil
			},
		),
	)
	ctx := context.Background()
	url := ts.URL

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := client.Request("Benchmark").Get(ctx, url)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		_, _ = resp.Body()
	}
}

// BenchmarkSentinelClient_WithHedging measures overhead of adaptive hedging checks.
func BenchmarkSentinelClient_WithHedging(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	client := New(WithDisableNetworkTrace())
	ctx := context.Background()
	url := ts.URL
	hedgeCfg := DefaultAdaptiveHedgeConfig()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := client.Request("Benchmark").
			AdaptiveHedge(hedgeCfg).
			Get(ctx, url)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		_, _ = resp.Body()
	}
}

// BenchmarkSentinelClient_ResponseDecoding measures JSON decoding convenience wrapper.
func BenchmarkSentinelClient_ResponseDecoding(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Small JSON payload
		_, _ = w.Write([]byte(`{"id": 123, "name": "benchmark", "active": true}`))
	}))
	defer ts.Close()

	client := New(WithDisableNetworkTrace())
	ctx := context.Background()
	url := ts.URL

	type Data struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var result Data
		_, err := client.Request("Benchmark").
			Decode(&result).
			Get(ctx, url)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

// BenchmarkSentinelClient_KitchenSink measures the FULL feature set in a complex request.
// Tracing + Metrics + Breaker + RateLimit + Retry + Hedging + Coalescing + Interceptors + Decoding.
func BenchmarkSentinelClient_KitchenSink(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	client := New(
		WithRetryConfig(DefaultRetryConfig()),
		WithBreakerConfig(DefaultBreakerConfig()),
		WithRateLimit(RateLimitConfig{RequestsPerSecond: float64(rate.Inf)}),
		WithRequestInterceptor(func(_ *http.Request) error { return nil }),
		WithResponseInterceptor(func(_ *http.Response, _ *http.Request) error { return nil }),
	)
	ctx := context.Background()
	url := ts.URL
	bodyDat := []byte("some payload")
	hedgeCfg := DefaultAdaptiveHedgeConfig()

	type Response struct {
		Status string `json:"status"`
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var res Response
		// mimic a complex real-world call
		_, err := client.Request("ComplexOp").
			Path(url).
			// Use Path as absolute URL if it starts with http, or concat? Helper logic handles it?
			// Wait, Request().Path() appends to BaseURL. If BaseURL is empty, it's just the path.
			// Benchmark logic used Get(ctx, url) before.
			// Let's use Get(ctx, url) and leave Path() for sub-resources if BaseURL was set.
			// But here we want to test Path() overhead too.
			// Let's set BaseURL in client or just use absolute URL in Get.
			// The previous failing test used .Path("/api/v1/resource") and .Get(ctx, url).
			// If Path is set, Get(ctx, url) might overwrite or combine.
			// request.go: if len(path) > 0 { rb.path = path[0] }
			// So .Path() is overwritten by Get arguments.
			// To benchmark Path(), we should rely on it.
			// Let's set BaseURL to ts.URL then use Path.
			// Re-creating client with BaseURL inside benchmark loop is bad.
			// BUT ts.URL is dynamic.
			// We can use Request().Path(ts.URL + "/resource")
			// Or just use .Get(ctx, ts.URL + "/resource")
			// Let's stick to the previous pattern of Get(ctx, url) for simplicity/correctness with mock server.
			Query("filter", "active").
			Header("X-Tenant", "benchmark").
			Body(bytes.NewReader(bodyDat)).
			Coalesce(). // Enable coalescing
			AdaptiveHedge(hedgeCfg).
			EnableTrace(). // Enable detailed trace collection
			Decode(&res).
			Get(ctx, url)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func Test_DeepSeek_accepts_two_dimensions_and_extended_read_only_plans(t *testing.T) {
	command, err := deepSeekResultCommand(`{
		"intent":"analytics","domain":"movement","metric":"movement_quantity","operation":"ranking",
		"group_by":["product","shop"],"movement_type":"sales_outbound","time_range":"date_range",
		"date_from":"2026-07-01","date_to":"2026-07-31","sort":"desc","limit":10,"presentation":"matrix"
	}`)
	if err != nil || command.Analytics == nil {
		t.Fatalf("two-dimensional plan error = %v command=%+v", err, command)
	}
	plan := command.Analytics
	if plan.GroupBy != larkGroupProduct || plan.GroupBy2 != larkGroupShop || plan.DateFrom != "2026-07-01" ||
		plan.DateTo != "2026-07-31" || plan.effectivePresentation() != larkPresentationMatrix {
		t.Fatalf("two-dimensional plan = %+v", plan)
	}

	for name, content := range map[string]string{
		"three dimensions":     `{"intent":"analytics","domain":"movement","metric":"movement_count","operation":"ranking","group_by":["product","shop","operator"],"time_range":"all"}`,
		"duplicate dimensions": `{"intent":"analytics","domain":"movement","metric":"movement_count","operation":"ranking","group_by":["shop","shop"],"time_range":"all"}`,
		"bad date":             `{"intent":"analytics","domain":"movement","metric":"movement_count","operation":"total","group_by":[],"time_range":"date_range","date_from":"2026-08-02","date_to":"2026-08-01"}`,
		"wrong presentation":   `{"intent":"analytics","domain":"movement","metric":"movement_count","operation":"trend","group_by":[],"time_range":"current_month","time_bucket":"day","presentation":"ranking"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, planErr := deepSeekResultCommand(content)
			var diagnostic *deepSeekIntentError
			if !errors.As(planErr, &diagnostic) || diagnostic.Stage != "plan_validate" {
				t.Fatalf("plan error = %#v", planErr)
			}
		})
	}
}

func Test_DeepSeek_retries_only_transient_failures_and_records_diagnostics(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var requests int32
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				if atomic.AddInt32(&requests, 1) == 1 {
					response.WriteHeader(status)
					return
				}
				_, _ = response.Write([]byte(deepSeekCompletion(`{"intent":"help"}`)))
			}))
			defer server.Close()
			parser := &deepSeekIntentParser{apiKey: "secret", model: "model", endpoint: server.URL, client: server.Client()}

			command, err := parser.Parse(context.Background(), "你会什么")
			if err != nil || command.Action != larkActionHelp || command.AIAttempts != 2 || command.AIStage != "http_status" ||
				command.AIStatus != status || atomic.LoadInt32(&requests) != 2 {
				t.Fatalf("retry result = command=%+v err=%v requests=%d", command, err, requests)
			}
		})
	}

	t.Run("transport timeout then success", func(t *testing.T) {
		var requests int32
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			if atomic.AddInt32(&requests, 1) == 1 {
				time.Sleep(30 * time.Millisecond)
				return
			}
			_, _ = response.Write([]byte(deepSeekCompletion(`{"intent":"inventory_summary"}`)))
		}))
		defer server.Close()
		parser := &deepSeekIntentParser{
			apiKey: "secret", model: "model", endpoint: server.URL, client: &http.Client{Timeout: 10 * time.Millisecond},
		}
		command, err := parser.Parse(context.Background(), "库存概览")
		if err != nil || command.Action != larkActionSummary || command.AIAttempts != 2 || command.AIStage != "transport" || atomic.LoadInt32(&requests) != 2 {
			t.Fatalf("timeout retry = command=%+v err=%v requests=%d", command, err, requests)
		}
	})

	for name, response := range map[string]struct {
		status int
		body   string
	}{
		"non-retryable 400": {status: http.StatusBadRequest, body: "PRIVATE_RESPONSE"},
		"bad response":      {status: http.StatusOK, body: "{"},
		"bad plan":          {status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"movement_count","group_by":["shop","operator","product"]}`)},
	} {
		t.Run(name, func(t *testing.T) {
			var requests int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&requests, 1)
				writer.WriteHeader(response.status)
				_, _ = writer.Write([]byte(response.body))
			}))
			defer server.Close()
			parser := &deepSeekIntentParser{apiKey: "PRIVATE_KEY", model: "model", endpoint: server.URL, client: server.Client()}
			_, err := parser.Parse(context.Background(), "PRIVATE_INPUT")
			var diagnostic *deepSeekIntentError
			if !errors.As(err, &diagnostic) || diagnostic.Attempts != 1 || atomic.LoadInt32(&requests) != 1 {
				t.Fatalf("diagnostic = %#v requests=%d", err, requests)
			}
			for _, secret := range []string{"PRIVATE_KEY", "PRIVATE_INPUT", "PRIVATE_RESPONSE"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("diagnostic leaked %q: %v", secret, err)
				}
			}
		})
	}
}

func Test_DeepSeek_respects_caller_deadline_within_total_budget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()
	parser := &deepSeekIntentParser{apiKey: "secret", model: "model", endpoint: server.URL, client: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := parser.Parse(ctx, "查库存")
	elapsed := time.Since(started)
	var diagnostic *deepSeekIntentError
	if !errors.As(err, &diagnostic) || diagnostic.Stage != "transport" || elapsed >= time.Second {
		t.Fatalf("deadline error = %#v elapsed=%s", err, elapsed)
	}
}

package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStalwartClient_getAccountID(t *testing.T) {
	var sessionCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jmap/session" {
			atomic.AddInt32(&sessionCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jmapSession{
				PrimaryAccounts: map[string]string{
					"urn:ietf:params:jmap:mail": "test-acc-id",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &StalwartClient{
		Config: StalwartConfig{
			URL:  server.URL,
			User: "testuser",
			Pass: "testpass",
		},
	}

	ctx := context.Background()

	// First call
	accID, err := client.getAccountID(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if accID != "test-acc-id" {
		t.Errorf("expected accID to be 'test-acc-id', got '%s'", accID)
	}
	if calls := atomic.LoadInt32(&sessionCalls); calls != 1 {
		t.Errorf("expected 1 session call, got %d", calls)
	}

	// Second call - should be cached
	accID, err = client.getAccountID(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if accID != "test-acc-id" {
		t.Errorf("expected accID to be 'test-acc-id', got '%s'", accID)
	}
	if calls := atomic.LoadInt32(&sessionCalls); calls != 1 {
		t.Errorf("expected 1 session call (cached), got %d", calls)
	}
}

func TestStalwartClient_FetchEmailMetadata_Retry(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jmap/session" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jmapSession{
				PrimaryAccounts: map[string]string{
					"urn:ietf:params:jmap:mail": "test-acc-id",
				},
			})
			return
		}
		if r.URL.Path == "/jmap" {
			count := atomic.AddInt32(&attempts, 1)
			if count < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jmapResponse{
				MethodResponses: [][]any{
					{"Email/get", map[string]any{
						"list": []any{
							map[string]any{
								"id":         "mail1",
								"messageId":  []any{"test-msg-id"},
								"subject":    "Test Subject",
								"from":       []any{map[string]any{"name": "Sender", "email": "sender@example.com"}},
								"to":         []any{map[string]any{"name": "Receiver", "email": "receiver@example.com"}},
								"preview":    "Hello world",
								"receivedAt": time.Now().Format(time.RFC3339),
							},
						},
					}, "g1"},
				},
			})
			return
		}
	}))
	defer server.Close()

	client := &StalwartClient{
		Config: StalwartConfig{
			URL:  server.URL,
			User: "testuser",
			Pass: "testpass",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// The retry logic has a 2s delay. So 2 retries = 4s.
	metadata, _, err := client.FetchEmailMetadata(ctx, "test-msg-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subject := metadata["subject"].(string); subject != "Test Subject" {
		t.Errorf("expected subject 'Test Subject', got '%s'", subject)
	}
	if count := atomic.LoadInt32(&attempts); count != 3 {
		t.Errorf("expected 3 attempts, got %d", count)
	}
}

func TestStalwartClient_FetchEmailMetadata_Formatting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jmap/session" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jmapSession{
				PrimaryAccounts: map[string]string{
					"urn:ietf:params:jmap:mail": "test-acc-id",
				},
			})
			return
		}
		if r.URL.Path == "/jmap" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jmapResponse{
				MethodResponses: [][]any{
					{"Email/get", map[string]any{
						"list": []any{
							map[string]any{
								"id":        "mail1",
								"messageId": []any{"<test-msg-id@example.com>"},
								"subject":   "Test Subject",
								"from": []any{
									map[string]any{"name": "Sender One", "email": "sender1@example.com"},
									map[string]any{"name": "", "email": "sender2@example.com"},
								},
								"to": []any{
									map[string]any{"name": "Receiver One", "email": "receiver1@example.com"},
								},
								"preview":    "Hello world",
								"receivedAt": "2023-10-27T10:00:00Z",
							},
						},
					}, "g1"},
				},
			})
			return
		}
	}))
	defer server.Close()

	client := &StalwartClient{
		Config: StalwartConfig{
			URL:  server.URL,
			User: "testuser",
			Pass: "testpass",
		},
	}

	ctx := context.Background()
	metadata, receivedAt, err := client.FetchEmailMetadata(ctx, "test-msg-id@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if from := metadata["from"].(string); from != "Sender One <sender1@example.com>, sender2@example.com" {
		t.Errorf("unexpected from: %s", from)
	}
	if to := metadata["to"].(string); to != "Receiver One <receiver1@example.com>" {
		t.Errorf("unexpected to: %s", to)
	}
	if ts := receivedAt.Format(time.RFC3339); ts != "2023-10-27T10:00:00Z" {
		t.Errorf("unexpected receivedAt: %s", ts)
	}
}

func TestStalwartClient_FetchEmailMetadata_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jmap/session" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jmapSession{
				PrimaryAccounts: map[string]string{
					"urn:ietf:params:jmap:mail": "test-acc-id",
				},
			})
			return
		}
		if r.URL.Path == "/jmap" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jmapResponse{
				MethodResponses: [][]any{
					{"Email/get", map[string]any{
						"list": []any{},
					}, "g1"},
				},
			})
			return
		}
	}))
	defer server.Close()

	client := &StalwartClient{
		Config: StalwartConfig{
			URL:  server.URL,
			User: "testuser",
			Pass: "testpass",
		},
	}

	ctx := context.Background()
	_, _, err := client.FetchEmailMetadata(ctx, "missing-id")
	if err == nil {
		t.Fatal("expected error for missing email, got nil")
	}
	if !strings.Contains(err.Error(), "not found in recent emails") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestFormatAddresses(t *testing.T) {
	tests := []struct {
		name     string
		addrs    []emailAddress
		expected string
	}{
		{
			name: "Single address with name",
			addrs: []emailAddress{
				{Name: "Alice", Email: "alice@example.com"},
			},
			expected: "Alice <alice@example.com>",
		},
		{
			name: "Single address without name",
			addrs: []emailAddress{
				{Name: "", Email: "bob@example.com"},
			},
			expected: "bob@example.com",
		},
		{
			name: "Multiple addresses",
			addrs: []emailAddress{
				{Name: "Alice", Email: "alice@example.com"},
				{Name: "", Email: "bob@example.com"},
			},
			expected: "Alice <alice@example.com>, bob@example.com",
		},
		{
			name:     "Empty list",
			addrs:    []emailAddress{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAddresses(tt.addrs)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParseEmailAddresses(t *testing.T) {
	input := []any{
		map[string]any{"name": "Alice", "email": "alice@example.com"},
		map[string]any{"email": "bob@example.com"},
	}

	expected := []emailAddress{
		{Name: "Alice", Email: "alice@example.com"},
		{Name: "", Email: "bob@example.com"},
	}

	result := parseEmailAddresses(input)

	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}

	for i := range expected {
		if result[i].Name != expected[i].Name || result[i].Email != expected[i].Email {
			t.Errorf("at index %d: expected %+v, got %+v", i, expected[i], result[i])
		}
	}
}

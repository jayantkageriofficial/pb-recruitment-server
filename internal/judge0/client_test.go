package judge0

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchRejections(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   []error
	}{
		{"queue full", 503, `{"error":"queue is full"}`, []error{ErrDispatchRejected, ErrDispatchRejected}},
		{"invalid request", 422, `[{"memory_limit":["exceeds maximum"]}]`, []error{ErrDispatchRejected, ErrDispatchRejected}},
		{"mixed validation", 201, `[{"token":"accepted"},{"memory_limit":["exceeds maximum"]}]`, []error{nil, ErrDispatchRejected}},
		{"empty entry", 201, `[{"token":"accepted"},{}]`, []error{nil, ErrDispatchRejected}},
		{"missing entry", 201, `[{"token":"accepted"}]`, []error{nil, ErrInvalidResponse}},
		{"internal error", 500, `{}`, []error{ErrUnavailable, ErrUnavailable}},
		{"bad gateway", 502, `{}`, []error{ErrUnavailable, ErrUnavailable}},
		{"gateway timeout", 504, `{}`, []error{ErrUnavailable, ErrUnavailable}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			t.Setenv("JUDGE0_URL", server.URL)
			t.Setenv("JUDGE0_BATCH_SIZE", "20")
			results, _ := NewClient().CreateBatch(context.Background(), make([]SubmissionRequest, 2))
			if len(results) != len(tc.want) {
				t.Fatalf("results = %+v", results)
			}
			for i, want := range tc.want {
				if !errors.Is(results[i].Error, want) {
					t.Errorf("result %d error = %v, want %v", i, results[i].Error, want)
				}
				if want == nil && results[i].Token != "accepted" {
					t.Errorf("accepted token lost: %+v", results[i])
				}
			}
		})
	}
}

func TestURLTrailingSlashes(t *testing.T) {
	for _, suffix := range []string{"", "/", "///"} {
		t.Run(suffix, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "//") {
					t.Errorf("double slash in Judge0 request: %s", r.URL.Path)
				}
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusCreated)
					w.Write([]byte(`[{"token":"accepted"}]`))
				} else {
					w.Write([]byte(`{"submissions":[{"token":"accepted","status":{"id":3}}]}`))
				}
			}))
			defer server.Close()
			t.Setenv("JUDGE0_URL", server.URL+suffix)
			t.Setenv("JUDGE0_CALLBACK_BASE_URL", "https://example.test"+suffix)
			t.Setenv("JUDGE0_CALLBACK_SECRET", "test-secret")
			client := NewClient()
			id := "e032ee69-cfd1-4350-b083-cc6e9822e9ec"
			callback, err := url.Parse(client.CallbackURL(id))
			if err != nil {
				t.Fatal(err)
			}
			if callback.Path != "/internal/judge0/callback/"+id || !client.VerifyCallback(id, callback.Query().Get("sig")) {
				t.Fatalf("invalid callback path or signature: %s", callback.Path)
			}
			if _, err := client.CreateBatch(context.Background(), []SubmissionRequest{{}}); err != nil {
				t.Fatal(err)
			}
			if _, err := client.GetSubmissions(context.Background(), []string{"accepted"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBatchBodyReadFailureIsAmbiguous(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`[{"token":"`))
	}))
	defer server.Close()
	t.Setenv("JUDGE0_URL", server.URL)
	t.Setenv("JUDGE0_AUTH_TOKEN", "")
	results, err := NewClient().CreateBatch(context.Background(), []SubmissionRequest{{LanguageID: 54}})
	if !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("body-read error = %v", err)
	}
	if len(results) != 1 || results[0].Token != "" || !errors.Is(results[0].Error, ErrUnavailable) {
		t.Fatalf("ambiguous result = %+v", results)
	}
}

func TestBatchResultValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		invalid    bool
	}{
		{"mixed found and missing", `{"submissions":[{"token":"a","status":{"id":3}},null]}`, false},
		{"missing entries", `{"submissions":[]}`, true},
		{"wrong token", `{"submissions":[{"token":"other","status":{"id":3}},null]}`, true},
		{"invalid status", `{"submissions":[{"token":"a","status":{"id":0}},null]}`, true},
		{"malformed body", `{`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/submissions/batch" || r.URL.Query().Get("tokens") != "a,b" || r.Header.Get("X-Auth-Token") != "test-token" {
					t.Error("invalid batch request or authentication")
				}
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			t.Setenv("JUDGE0_URL", server.URL)
			t.Setenv("JUDGE0_AUTH_TOKEN", "test-token")
			results, err := NewClient().GetSubmissions(context.Background(), []string{"a", "b"})
			if tc.invalid {
				if !errors.Is(err, ErrInvalidResponse) {
					t.Fatalf("invalid response error = %v", err)
				}
			} else if err != nil || len(results) != 2 || results[0] == nil || results[1] != nil {
				t.Fatalf("results = %+v, err = %v", results, err)
			}
		})
	}
}

func TestRunUsesCustomInputWithoutExpectedOutput(t *testing.T) {
	var polls atomic.Int32
	judge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != "secret" {
			t.Errorf("auth token = %q", r.Header.Get("X-Auth-Token"))
		}
		if r.URL.Query().Get("base64_encoded") != "false" {
			t.Errorf("base64_encoded = %q", r.URL.Query().Get("base64_encoded"))
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/submissions":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode submission: %v", err)
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if _, exists := payload["expected_output"]; exists {
				t.Error("run submission unexpectedly set expected_output")
			}
			if _, exists := payload["callback_url"]; exists {
				t.Error("run submission unexpectedly set callback_url")
			}
			if payload["stdin"] != "42\nβ\n" || payload["source_code"] != "print(input())" {
				t.Errorf("run input = %#v", payload)
			}
			if payload["language_id"] != float64(71) {
				t.Errorf("language_id = %v", payload["language_id"])
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "run-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/submissions/run-token":
			count := polls.Add(1)
			if count == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": map[string]any{"id": 1, "description": "In Queue"},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": map[string]any{"id": 3, "description": "Accepted"},
				"stdout": "42\n", "time": "0.01", "memory": 1024,
			})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	t.Cleanup(judge.Close)

	client := &Client{
		baseURL:    judge.URL,
		authToken:  "secret",
		httpClient: &http.Client{Timeout: 2 * time.Second},
	}
	result, err := client.Run(context.Background(), SubmissionRequest{
		SourceCode:   "print(input())",
		LanguageID:   71,
		Stdin:        "42\nβ\n",
		CPUTimeLimit: 1,
		MemoryLimit:  256 * 1024,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status.Description != "Accepted" || result.Stdout != "42\n" || result.Time != "0.01" || result.Memory != 1024 {
		t.Fatalf("run result = %#v", result)
	}
	if polls.Load() < 2 {
		t.Fatalf("polls = %d, want at least 2", polls.Load())
	}
}

func TestDispatchTimeout(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"", 10 * time.Second},
		{"1000", time.Second},
		{"30000", 10 * time.Second},
		{"0", 10 * time.Second},
		{"-1", 10 * time.Second},
		{"invalid", 10 * time.Second},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("JUDGE0_TIMEOUT_MS", tc.value)
			client := NewClient()
			if client.Timeout() != tc.want || client.httpClient.Timeout != tc.want {
				t.Fatalf("timeouts = %v/%v, want %v", client.Timeout(), client.httpClient.Timeout, tc.want)
			}
		})
	}
	if timeout := (*Client)(nil).Timeout(); timeout != 10*time.Second {
		t.Fatalf("fallback timeout = %v", timeout)
	}
}

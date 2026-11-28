package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestSentEmailQueueEndpoint(t *testing.T) {
	const token = "test-email-queue-token-1234567890"
	const path = "/pub/testing/email-queue"
	m, _, _ := newAuthTestModule(t)
	attachTestEmail(t, m)
	queue := newSentEmailQueue(token)
	m.emailCodes.sentQueue = queue
	for i := range sentEmailQueueCapacity + 1 {
		queue.record(sentEmail{
			Recipient:      "tester@example.test",
			VerificationID: fmt.Sprintf("ticket-%d", i),
			Code:           "012345",
		})
	}
	for _, headers := range [][]string{nil, {"Bearer wrong"}, {"Bearer " + token, "Bearer " + token}} {
		response := emailHTTPHeader(t, m, t.Context(), http.MethodGet, path, nil, "Authorization", headers...)
		if response.Code != http.StatusUnauthorized || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected unauthenticated response: %d %s", response.Code, response.Body)
		}
	}
	read := func(query string) []sentEmail {
		t.Helper()
		response := emailHTTPHeader(t, m, t.Context(), http.MethodGet, path+query, nil, "Authorization", "Bearer "+token)
		var result struct {
			Items []sentEmail `json:"items"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || result.Items == nil {
			t.Fatalf("unexpected queue response: %d %s, decode: %v", response.Code, response.Body, err)
		}
		for _, field := range []string{`"subject":`, `"plain":`, `"html":`} {
			if strings.Contains(response.Body.String(), field) {
				t.Fatalf("queue response includes %s", field)
			}
		}
		return result.Items
	}
	if items := read(""); len(items) != sentEmailQueueCapacity || items[0].VerificationID != fmt.Sprintf("ticket-%d", sentEmailQueueCapacity) || items[len(items)-1].VerificationID != "ticket-1" {
		t.Fatal("queue should retain the newest deliveries in reverse chronological order")
	}
	if items := read("?verification_id=ticket-1&recipient=TESTER%40example.test"); len(items) != 1 || items[0].Code != "012345" {
		t.Fatalf("combined filter returned %d items", len(items))
	}
	if items := read("?verification_id=ticket-1&recipient=someone%40example.test"); len(items) != 0 {
		t.Fatal("queue exposed a message for another recipient")
	}
	response := emailHTTPHeader(t, m, t.Context(), http.MethodGet, path+"?verification_id=one&verification_id=two", nil, "Authorization", "Bearer "+token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("duplicate filter returned %d", response.Code)
	}
}

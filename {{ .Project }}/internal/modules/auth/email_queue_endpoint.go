package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/server"
)

const sentEmailQueueCapacity = 25

type sentEmail struct {
	Queue          string `json:"queue"`
	VerificationID string `json:"verification_id,omitempty"`
	Recipient      string `json:"recipient"`
	Purpose        string `json:"purpose,omitempty"`
	Code           string `json:"code,omitempty"`
	SentAt         int64  `json:"sent_at"`
}

type sentEmailQueue struct {
	token    *[sha256.Size]byte
	mu       sync.Mutex
	messages [sentEmailQueueCapacity]sentEmail
	next     int
	count    int
}

func newSentEmailQueue(token string) *sentEmailQueue {
	queue := &sentEmailQueue{}
	if token != "" {
		digest := sha256.Sum256([]byte(token))
		queue.token = &digest
	}
	return queue
}

func (q *sentEmailQueue) record(message sentEmail) {
	q.mu.Lock()
	defer q.mu.Unlock()
	message.SentAt = time.Now().UnixMilli()
	q.messages[q.next] = message
	q.next = (q.next + 1) % len(q.messages)
	q.count = min(q.count+1, len(q.messages))
}

func (q *sentEmailQueue) list(recipient, verificationID string) []sentEmail {
	q.mu.Lock()
	defer q.mu.Unlock()
	items := make([]sentEmail, 0, q.count)
	for offset := 1; offset <= q.count; offset++ {
		message := q.messages[(q.next+len(q.messages)-offset)%len(q.messages)]
		if (recipient == "" || strings.EqualFold(recipient, message.Recipient)) && (verificationID == "" || verificationID == message.VerificationID) {
			items = append(items, message)
		}
	}
	return items
}

func (q *sentEmailQueue) authorized(headers []string) bool {
	if q.token == nil {
		return true
	}
	if len(headers) != 1 {
		return false
	}
	scheme, value, ok := strings.Cut(headers[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return false
	}
	token := sha256.Sum256([]byte(value))
	return subtle.ConstantTimeCompare(token[:], q.token[:]) == 1
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) sentEmails(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) sentEmails(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	queue := m.emailCodes.sentQueue
	if !queue.authorized(r.Header.Values("Authorization")) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="email-queue"`)
		server.WriteError(w, r, http.StatusUnauthorized, apperror.Unauthorized)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query["recipient"]) > 1 || len(query["verification_id"]) > 1 {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	server.WriteOK(w, struct {
		Items []sentEmail `json:"items"`
	}{Items: queue.list(query.Get("recipient"), query.Get("verification_id"))})
}

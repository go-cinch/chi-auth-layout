package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/redact"
)

const auditPayloadLimit = 64 << 10

// auditHeaders snapshots request headers before handlers run. Host is stored
// separately by net/http and must be included explicitly.
func auditHeaders(r *http.Request, policy redact.Policy) json.RawMessage {
	values := make(url.Values, len(r.Header)+1)
	size := 0
	for name, items := range r.Header {
		name = http.CanonicalHeaderKey(name)
		size += len(name)
		for _, value := range items {
			size += len(value)
			if size > auditPayloadLimit {
				return json.RawMessage(`{"_omitted":"size limit"}`)
			}
			if policy.IsSensitive(name) {
				value = "[redacted]"
			} else {
				value = auditHeaderURL(name, value, policy)
			}
			values[name] = append(values[name], value)
		}
	}
	if r.Host != "" {
		values["Host"] = []string{r.Host}
	}
	result, _ := json.Marshal(auditQuery(values, policy))
	if len(result) > auditPayloadLimit {
		return json.RawMessage(`{"_omitted":"size limit"}`)
	}
	return result
}

func auditHeaderURL(name, value string, policy redact.Policy) string {
	switch strings.ToLower(name) {
	case "referer", "location", "content-location", "x-original-uri", "x-original-url", "x-rewrite-url", "x-forwarded-uri", "x-permission-uri":
	default:
		return value
	}
	u, err := url.Parse(value)
	if err != nil {
		return "[redacted]"
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "[redacted]"
	}
	changed := false
	for key := range query {
		if policy.IsSensitive(key) {
			query[key] = []string{"[redacted]"}
			changed = true
		}
	}
	if changed {
		u.RawQuery = query.Encode()
	}
	if u.User != nil {
		u.User = url.User("[redacted]")
	}
	if u.Fragment != "" {
		u.Fragment = "[redacted]"
	}
	return u.String()
}

type auditBuffer struct {
	data     []byte
	overflow bool
}

func (b *auditBuffer) Write(p []byte) (int, error) {
	n := len(p)
	available := auditPayloadLimit - len(b.data)
	if n > available {
		b.overflow = true
		p = p[:available]
	}
	b.data = append(b.data, p...)
	return n, nil
}

type auditBody struct {
	io.ReadCloser
	buffer *auditBuffer
}

func (b *auditBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	_, _ = b.buffer.Write(p[:n])
	return n, err
}

func auditFile(header http.Header) bool {
	media, _, _ := mime.ParseMediaType(header.Get("Content-Type"))
	disposition, parameters, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	return disposition == "attachment" || parameters["filename"] != "" || strings.HasPrefix(media, "multipart/") ||
		(media != "" && media != "application/json" && !strings.HasSuffix(media, "+json") && media != "application/x-www-form-urlencoded" && media != "text/plain")
}

func auditPayload(header http.Header, b *auditBuffer, policy redact.Policy) any {
	if auditFile(header) {
		return "file"
	}
	if b.overflow {
		return "[omitted: size limit]"
	}
	if len(bytes.TrimSpace(b.data)) == 0 {
		return nil
	}
	media, _, _ := mime.ParseMediaType(header.Get("Content-Type"))
	if media == "application/x-www-form-urlencoded" {
		values, err := url.ParseQuery(string(b.data))
		if err != nil {
			return "[omitted: invalid form]"
		}
		return auditQuery(values, policy)
	}
	decoder := json.NewDecoder(bytes.NewReader(b.data))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
		return "[omitted: unstructured body]"
	}
	return policy.Structured(value)
}

func auditQuery(values url.Values, policy redact.Policy) any {
	result := make(map[string]any, len(values))
	for key, values := range values {
		if len(values) == 1 {
			result[key] = values[0]
		} else {
			items := make([]any, len(values))
			for i, v := range values {
				items[i] = v
			}
			result[key] = items
		}
	}
	return policy.Structured(result)
}

func auditRequest(r *http.Request, b *auditBuffer, policy redact.Policy) json.RawMessage {
	if auditFile(r.Header) {
		return json.RawMessage(`"file"`)
	}
	var query any
	if len(r.URL.RawQuery) > auditPayloadLimit {
		query = "[omitted: size limit]"
	} else {
		values, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			query = "[omitted: invalid query]"
		} else if len(values) > 0 {
			query = auditQuery(values, policy)
		}
	}
	result, _ := json.Marshal(map[string]any{"query": query, "body": auditPayload(r.Header, b, policy)})
	if len(result) > auditPayloadLimit {
		return json.RawMessage(`"[omitted: size limit]"`)
	}
	return result
}

func auditResponse(header http.Header, b *auditBuffer, policy redact.Policy) json.RawMessage {
	result, _ := json.Marshal(auditPayload(header, b, policy))
	if len(result) > auditPayloadLimit {
		return json.RawMessage(`"[omitted: size limit]"`)
	}
	return result
}

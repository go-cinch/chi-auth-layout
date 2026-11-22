package mailtemplate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListSecondPageHTTPContract(t *testing.T) {
	m, mock := testModule(t)
	mock.ExpectQuery("SELECT count").WithArgs("notice", "notification").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(101))
	mock.ExpectQuery("SELECT id, created_at").
		WithArgs("notice", "notification", int32(100), int64(100)).
		WillReturnRows(recordRow(false, 1, 0, true, "notice.101"))

	response := httptest.NewRecorder()
	testRouter(m).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?p=2&s=100&q=notice&category=notification", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("page two: status %d, body %s", response.Code, response.Body)
	}
	var got struct {
		P     int
		S     int
		T     int
		Items []struct {
			TemplateID string `json:"template_id"`
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.P != 2 || got.S != 100 || got.T != 101 || len(got.Items) != 1 || got.Items[0].TemplateID != "notice.101" {
		t.Fatalf("unexpected second page: %+v", got)
	}
}

func TestContentSizeLimits(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		set   func(*Translation, int)
	}{
		{"HTML bytes", 200000, func(v *Translation, n int) { v.HTML = strings.Repeat("a", n) }},
		{"plain-text bytes", 100000, func(v *Translation, n int) { v.Text = strings.Repeat("a", n) }},
		{"preview key bytes", 100, func(v *Translation, n int) { v.PreviewValues = map[string]string{strings.Repeat("a", n): "sample"} }},
		{"preview value bytes", 10000, func(v *Translation, n int) { v.PreviewValues = map[string]string{"sample": strings.Repeat("a", n)} }},
		{"preview map entries", 100, func(v *Translation, n int) {
			v.PreviewValues = make(map[string]string, n)
			for i := 0; i < n; i++ {
				v.PreviewValues[fmt.Sprintf("key_%d", i)] = "sample"
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, n := range []int{c.limit - 1, c.limit, c.limit + 1} {
				content := sampleContent()
				value := content["en-US"]
				c.set(&value, n)
				content["en-US"] = value
				err := validate("Test", "notification", content, false)
				if n <= c.limit && err != nil {
					t.Errorf("size %d should be accepted: %v", n, err)
				}
				if n > c.limit && !errors.Is(err, ErrInvalid) {
					t.Errorf("size %d should be rejected: %v", n, err)
				}
			}
		})
	}
}

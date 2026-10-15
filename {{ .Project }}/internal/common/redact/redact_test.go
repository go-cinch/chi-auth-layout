package redact

import (
	"net/url"
	"strings"
	"testing"
)

func TestPolicyAndMask(t *testing.T) {
	policy := New("tenantCode")
	for _, name := range []string{"password", "accessToken", "SERVICE_DATABASE_DSN", "tenant_code"} {
		if !policy.IsSensitive(name) {
			t.Errorf("%q is not sensitive", name)
		}
	}
	if policy.IsSensitive("monkey") {
		t.Fatal("monkey matched key")
	}
	for input, expected := range map[string]string{
		"abcdefgh": "abc***fgh",
		"abcdef":   "abc***",
		"abcde":    "abc***",
		"abcd":     "abc***",
		"abc":      "***",
		"ab":       "***",
		"a":        "***",
		"密":        "***",
		"密码":       "***",
		"密a🔑":      "***",
		"密码甲乙":     "密码甲***",
		"":         "***",
	} {
		if actual := Mask(input); actual != expected {
			t.Errorf("Mask(%q) = %q, want %q", input, actual, expected)
		}
		if actual := policy.Value("password", input); actual != expected {
			t.Errorf("Policy.Value(password, %q) = %q, want %q", input, actual, expected)
		}
	}
	for input, expected := range map[string]string{
		"redis://127.0.0.1:6379/0":               "redis://127.0.0.1:6379/0",
		"redis://root:password@127.0.0.1:6379/0": "redis://root:pas***ord@127.0.0.1:6379/0",
		"root:password@tcp(127.0.0.1:3306)/app":  "root:pas***ord@tcp(127.0.0.1:3306)/app",
		"root@tcp(127.0.0.1:3306)/app":           "root@tcp(127.0.0.1:3306)/app",
		"redis://root@127.0.0.1:6379/0":          "redis://root@127.0.0.1:6379/0",
		"not-a-dsn":                              "not-a-dsn",
	} {
		if actual := DSN(input); actual != expected {
			t.Errorf("DSN(%q) = %q, want %q", input, actual, expected)
		}
	}
	if value := policy.Value("username", "plain"); value != "plain" {
		t.Fatalf("plain value = %q", value)
	}
}

func TestDSNShortCredentials(t *testing.T) {
	for _, secret := range []struct{ name, value string }{
		{"empty", ""},
		{"one_ascii", "a"},
		{"two_ascii", "ab"},
		{"three_ascii", "abc"},
		{"one_unicode", "密"},
		{"two_unicode", "密码"},
		{"three_mixed_unicode", "密a🔑"},
	} {
		t.Run(secret.name, func(t *testing.T) {
			for _, test := range []struct{ name, input, want string }{
				{"url", "postgres://" + url.UserPassword("root", secret.value).String() + "@localhost/app", "postgres://root:***@localhost/app"},
				{"query", "postgres://localhost/app?password=" + url.QueryEscape(secret.value), "postgres://localhost/app?password=***"},
				{"keyword", "host=localhost password=" + secret.value, "host=localhost password='***'"},
				{"quoted_keyword", "host=localhost password='" + secret.value + "' dbname=app", "host=localhost password='***' dbname=app"},
			} {
				t.Run(test.name, func(t *testing.T) {
					if got := DSN(test.input); got != test.want {
						t.Fatalf("DSN() = %q, want %q", got, test.want)
					}
					if got := New().Value("database.dsn", test.input); got != test.want {
						t.Fatalf("policy DSN redaction = %q, want %q", got, test.want)
					}
				})
			}
		})
	}
}

func TestDSNCredentialsInQueriesAndOptions(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{
			name:  "url query without userinfo",
			input: "postgresql://localhost/app?sslmode=disable&password=abcdefghijk",
			want:  "postgresql://localhost/app?password=abc***ijk&sslmode=disable",
		},
		{
			name:  "url userinfo and repeated query credentials",
			input: "redis://root:abcdefghijk@localhost/0?token=secret123&token=secret456&db=1",
			want:  "redis://root:abc***ijk@localhost/0?db=1&token=sec***123&token=sec***456",
		},
		{
			name:  "encoded url credential",
			input: "postgres://root:abc%40defghijk@localhost/app?password=abc%26defghijk",
			want:  "postgres://root:abc***ijk@localhost/app?password=abc***ijk",
		},
		{
			name:  "plain keyword password",
			input: "host=localhost user=root password=abcdefghijk dbname=app sslmode=disable",
			want:  "host=localhost user=root password='abc***ijk' dbname=app sslmode=disable",
		},
		{
			name:  "quoted keyword and whitespace",
			input: " host = localhost\tpassword = 'abc defghijk' dbname='my db' ",
			want:  " host = localhost\tpassword = 'abc***ijk' dbname='my db' ",
		},
		{
			name:  "keyword password containing mysql syntax",
			input: "host=localhost password=abc@tcp(defghijk) dbname=app",
			want:  "host=localhost password='abc***jk)' dbname=app",
		},
		{
			name:  "keyword password containing url syntax",
			input: "host=localhost password=abc://defghijk dbname=app",
			want:  "host=localhost password='abc***ijk' dbname=app",
		},
		{
			name:  "first keyword password containing url syntax",
			input: " password = 'abc://defghijk' host=localhost dbname=app",
			want:  " password = 'abc***ijk' host=localhost dbname=app",
		},
		{
			name:  "escaped keyword credential",
			input: `host=localhost password=abc\ defghijk sslpassword='abc\'defghijk' dbname=app`,
			want:  `host=localhost password='abc***ijk' sslpassword='abc***ijk' dbname=app`,
		},
		{
			name:  "repeated keyword credentials",
			input: "password=abcdefghijk password=secret123 host=localhost",
			want:  "password='abc***ijk' password='sec***123' host=localhost",
		},
		{
			name:  "empty keyword credential",
			input: "host=localhost password=",
			want:  "host=localhost password='***'",
		},
		{
			name:  "unchanged keywords",
			input: "host=localhost dbname='my db' sslmode=disable",
			want:  "host=localhost dbname='my db' sslmode=disable",
		},
		{
			name:  "malformed query",
			input: "postgres://localhost/app?password=abcdefghijk%ZZ&sslmode=disable",
			want:  "postgres://localhost/app?***",
		},
		{
			name:  "malformed url",
			input: "postgres://root:abcdefghijk%ZZ@localhost/app",
			want:  "***",
		},
		{
			name:  "unterminated quoted credential",
			input: "host=localhost password='abcdefghijk",
			want:  "***",
		},
		{
			name:  "unterminated escape",
			input: `host=localhost password=abcdefghijk\`,
			want:  "***",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := DSN(test.input)
			if got != test.want || strings.Contains(got, "abcdefghijk") {
				t.Fatalf("DSN() = %q, want %q", got, test.want)
			}
			if got := New().Value("database.dsn", test.input); got != test.want {
				t.Fatalf("policy DSN redaction = %q, want %q", got, test.want)
			}
		})
	}
}

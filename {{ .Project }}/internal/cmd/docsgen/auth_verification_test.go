package main

import (
	"go/ast"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTOTPResponseContracts(t *testing.T) {
	output := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := run("../..", "../../../conf", output, "Verification contracts"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct{ Headers map[string]any }
		}
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		statuses     []string
		verificationHeaders   []string
	}{
		{"get", "/totp", []string{"200", "400", "401", "403", "500", "503"}, nil},
		{"post", "/totp/challenge", []string{"200", "400", "401", "403", "500", "503"}, nil},
		{"post", "/totp/setup", []string{"200", "400", "401", "403", "409", "429", "500", "503"}, []string{"400", "403", "429"}},
		{"post", "/totp/confirm", []string{"200", "400", "401", "403", "409", "429", "500", "503"}, []string{"400", "403", "429"}},
		{"post", "/totp/test", []string{"200", "400", "401", "403", "429", "500", "503"}, []string{"403"}},
		{"post", "/pub/login", []string{"200", "400", "401", "403", "429", "500", "503"}, []string{"400", "403", "429"}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			responses := doc.Paths[tc.path][tc.method].Responses
			var statuses []string
			for status, response := range responses {
				statuses = append(statuses, status)
				_, hasHeader := response.Headers["X-Verification-Required"]
				if hasHeader != slices.Contains(tc.verificationHeaders, status) {
					t.Errorf("%s: Verification header = %v", status, hasHeader)
				}
				if status == "400" || status == "429" {
					if _, ok := response.Headers["X-MFA-Enrollment-Required"]; ok {
						t.Errorf("%s: invalid codes and rate limits do not require enrollment", status)
					}
				}
			}
			slices.Sort(statuses)
			if !reflect.DeepEqual(statuses, tc.statuses) {
				t.Fatalf("statuses = %v; want %v", statuses, tc.statuses)
			}
		})
	}
}

func TestPasswordChangeDocumentsSensitiveVerificationUnderAuthPrefixes(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/user/auth", "/dictionary/auth"} {
		r := &route{method: http.MethodPatch, path: prefix + "/change/pwd", tag: "auth", authenticated: true, handler: &ast.FuncDecl{Name: ast.NewIdent("changePassword")}, responses: map[int]response{http.StatusOK: {}}}
		g := &generator{components: map[string]*schema{}}
		g.describeVerificationHeaders(r)
		if len(r.parameters) != 3 || r.parameters[0].name != "X-TOTP-Code" || r.parameters[1].name != "X-Step-Up-Challenge" || r.parameters[2].name != "X-Step-Up-Credential" {
			t.Fatal("password change lacks same-endpoint verification header", prefix)
		}
		for _, status := range []int{400, 403, 429, 503} {
			if _, ok := r.responses[status]; !ok {
				t.Fatalf("password change missing Verification failure status %d under %s", status, prefix)
			}
		}
	}
}

func TestTOTPTestDocumentationWithSensitiveRootMount(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/user/auth", "/dictionary/auth"} {
		r := &route{method: http.MethodPost, path: prefix + "/totp/test", tag: "auth", authenticated: true, handler: &ast.FuncDecl{Name: ast.NewIdent("totpTest")}, responses: map[int]response{http.StatusOK: {}}}
		g := &generator{}
		g.describeVerificationHeaders(r)
		if len(r.parameters) != 0 {
			t.Fatal("test was documented as requiring a sensitive header", prefix)
		}
		if len(r.responses) != 1 || len(r.responses[http.StatusOK].headers) != 0 {
			t.Fatal("test was documented as granting or requiring sensitive authorization", prefix)
		}
		if !strings.Contains(r.responses[http.StatusOK].description, "shared account and credential-version rate limit") {
			t.Fatal("test lost rate-limit contract", prefix)
		}
	}
}

func TestAuthOnboardingDocumentationIgnoresBusinessMount(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/user/auth", "/dictionary/auth"} {
		for _, endpoint := range []struct{ handler, method, path string }{
			{"totpStatus", "GET", "/totp"},
			{"totpChallenge", "POST", "/totp/challenge"},
			{"totpSetup", "POST", "/totp/setup"},
			{"totpConfirm", "POST", "/totp/confirm"},
			{"logout", "POST", "/logout"},
			{"resetPassword", "PATCH", "/reset/pwd"},
			{"passwordChangeChallenge", "POST", "/challenge"},
		} {
			r := &route{method: endpoint.method, path: prefix + endpoint.path, tag: "auth", authenticated: true, handler: &ast.FuncDecl{Name: ast.NewIdent(endpoint.handler)}, responses: map[int]response{http.StatusOK: {}}}
			g := &generator{components: map[string]*schema{}}
			g.describeVerificationHeaders(r)
			if endpoint.handler == "totpSetup" || endpoint.handler == "totpConfirm" {
				if len(r.parameters) != 3 || len(r.responses) != 5 {
					t.Fatal("binding lacks sensitive verification contract", r.path)
				}
				continue
			}
			if len(r.parameters) != 0 || len(r.responses) != 1 {
				t.Fatalf("onboarding %s %s incorrectly documents step-up", r.method, r.path)
			}
		}
	}
}

func TestTOTPRecoveryHeadersDocumentation(t *testing.T) {
	output := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := run("../..", "../../../conf", output, "authenticator recovery"); err != nil {
		t.Fatal(err)
	}
	body, components := readRequestSchema(t, output, "/pub/login")
	if len(body.AnyOf) != 2 || body.Nullable || !strings.Contains(body.Description, "With X-MFA-Ticket, send only {}") {
		t.Fatal("login must document encrypted credentials and ticket continuation", body)
	}
	empty, encrypted := body.AnyOf[0], body.AnyOf[1]
	if empty.Type != "object" || empty.Nullable || empty.AdditionalProperties != false || len(empty.Properties) != 0 {
		t.Fatal("ticket continuation must be exactly an empty JSON object", empty)
	}
	if encrypted.Ref != "#/components/schemas/EncryptedLoginInput" {
		t.Fatal("password login lost its encrypted input schema", encrypted)
	}
	input := components["EncryptedLoginInput"]
	if len(input.Required) != 2 || input.Required[0] != "challenge_id" || input.Required[1] != "credential" || input.Properties["challenge_id"].Type != "string" || input.Properties["credential"].Type != "string" {
		t.Fatal("encrypted login fields changed", input)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name, In, Description string
			}
			Responses map[string]struct {
				Description string
				Headers     map[string]any
				Content     map[string]struct {
					Schema struct {
						Ref string `yaml:"$ref"`
					}
				}
			}
		}
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	login := doc.Paths["/pub/login"]["post"]
	testOperation, ok := doc.Paths["/totp/test"]["post"]
	if !ok {
		t.Fatal("authenticated test route missing from OpenAPI")
	}
	for _, detail := range []string{"shared account and credential-version rate limit", "Does not create, read, refresh, extend, or clear sensitive-operation authorization", "cannot be reused for testing, login, or sensitive operations"} {
		if !strings.Contains(testOperation.Responses["200"].Description, detail) {
			t.Errorf("authenticator test contract missing %q", detail)
		}
	}
	for _, status := range []string{"400", "401", "403", "429", "503"} {
		if testOperation.Responses[status].Content["application/json"].Schema.Ref != "#/components/schemas/ErrorResponse" {
			t.Errorf("test %s must retain the standard error schema", status)
		}
	}
	for _, status := range []string{"200", "400", "429"} {
		if _, ok := testOperation.Responses[status].Headers["X-Verification-Required"]; ok {
			t.Errorf("test %s must not trigger a sensitive verification retry", status)
		}
	}
	for _, parameter := range testOperation.Parameters {
		if parameter.Name == "X-TOTP-Code" || parameter.Name == "X-MFA-Ticket" {
			t.Error("test route incorrectly documented as a sensitive operation or login")
		}
	}
	descriptions := make(map[string]string)
	for path, methods := range doc.Paths {
		for method, operation := range methods {
			for _, parameter := range operation.Parameters {
				if parameter.Name == "X-Recovery-Code" && (path != "/pub/login" || method != "post" || parameter.In != "header") {
					t.Fatalf("recovery code exposed outside password-verified login: %s %s", method, path)
				}
				if parameter.Name == "X-TOTP-Code" && (!strings.Contains(parameter.Description, "Six-digit TOTP only") || strings.Contains(strings.ToLower(parameter.Description), "recovery")) {
					t.Errorf("%s %s does not limit ordinary Verification verification to TOTP", method, path)
				}
				if path == "/pub/login" && method == "post" {
					descriptions[parameter.Name] = parameter.Description
				}
			}
		}
	}
	if !strings.Contains(descriptions["X-MFA-Ticket"], "exactly one of X-TOTP-Code or X-Recovery-Code") {
		t.Fatal("login ticket must describe mutually exclusive verification methods")
	}
	for _, detail := range []string{"only with X-MFA-Ticket", "empty JSON object", "after password verification", "do not combine with X-TOTP-Code", "all recovery codes and existing sessions", "enrollment-restricted session", "does not complete login or authorize sensitive operations"} {
		if !strings.Contains(descriptions["X-Recovery-Code"], detail) {
			t.Errorf("recovery header documentation missing %q", detail)
		}
	}
	for _, status := range []string{"400", "403", "429", "503"} {
		if login.Responses[status].Content["application/json"].Schema.Ref != "#/components/schemas/ErrorResponse" {
			t.Errorf("login %s must retain the standard error schema", status)
		}
	}
	for _, header := range []string{"X-Verification-Required", "X-MFA-Enrollment-Required", "X-Password-Reset-Required"} {
		if _, ok := login.Responses["200"].Headers[header]; ok {
			t.Errorf("login session response still exposes retired session restriction header %s", header)
		}
	}
	for _, status := range []string{"200", "400", "403", "429", "503"} {
		if _, ok := login.Responses[status].Headers["X-Recovery-Code"]; ok {
			t.Errorf("login %s must not expose a recovery credential in response headers", status)
		}
	}
}

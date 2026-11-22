package main

import (
	"net/http"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/genfile"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
)

type generator struct {
	audit          config.AuditConfig
	bodyLimits     config.HTTPBodyLimitsConfig
	loginTicketTTL time.Duration
	limits         pagination.Limits
	components     map[string]*schema
	visiting       map[string]bool
	schemaNames    map[*sourcePackage]map[string]string
}

func run(source, configDir, output, title string) error {
	cfg, limits, err := loadSettings(configDir)
	if err != nil {
		return err
	}
	var servers []docServer
	for _, value := range cfg.HTTP.Docs.Servers {
		servers = append(servers, docServer{URL: value.URL, Description: value.Description})
	}
	packages, err := loadPackages(source)
	if err != nil {
		return err
	}
	if err := configureModulePaths(packages, configDir); err != nil {
		return err
	}
	routes := discoverRoutes(packages)
	gen := &generator{audit: cfg.Audit, bodyLimits: cfg.HTTP.BodyLimits, loginTicketTTL: cfg.Auth.MFA.LoginTicketTTL, limits: limits, components: make(map[string]*schema), visiting: make(map[string]bool)}
	gen.registerSchemaNames(packages, source)
	for index := range routes {
		// Infer the base contract before applying business-specific supplements.
		gen.describeRoute(&routes[index])
		gen.describeEmailRoute(&routes[index])
		gen.describeStatusRoute(&routes[index])
		gen.describeAuditRoute(&routes[index])
		gen.describeSummaryRoute(&routes[index])
		gen.describeUserGroupRoute(&routes[index])
		gen.describeListFields(&routes[index])
		gen.describeMailTemplateRoute(&routes[index])
		gen.describeDetailFields(&routes[index])
		gen.describeCatalogRoute(&routes[index])
		// Security supplements depend on the inferred and business responses.
		if routes[index].idempotent && routes[index].method == http.MethodPost {
			routes[index].parameters = append(routes[index].parameters, parameter{
				in: "header", name: "X-Idempotent", schema: schema{typeName: "string"},
			})
			gen.ensureErrorResponse()
			addErrorResponse(&routes[index], http.StatusBadRequest, "invalid idempotency key")
			if routes[index].path == "/msg" {
				routes[index].requestBody.description = "The same sender and X-Idempotent key replay the original publication for matching content. A different payload conflicts. A withdrawn publication returns 404."
			} else {
				addErrorResponse(&routes[index], http.StatusConflict, "idempotency key has already been used")
			}
			addErrorResponse(&routes[index], http.StatusInternalServerError, "internal server error")
		}

		if routeRequiresAuthentication(routes[index].path) {
			routes[index].authenticated = true
			routes[index].parameters = append(routes[index].parameters, parameter{in: "header", name: "X-Auth-Scope", schema: schema{typeName: "string", example: "admin"}})
			gen.ensureErrorResponse()
			if _, exists := routes[index].responses[http.StatusBadRequest]; !exists {
				addErrorResponse(&routes[index], http.StatusBadRequest, "invalid permission scope header")
			}
			if _, exists := routes[index].responses[http.StatusForbidden]; !exists {
				addErrorResponse(&routes[index], http.StatusForbidden, "permission denied")
			}
			routes[index].responses[http.StatusUnauthorized] = response{
				description: http.StatusText(http.StatusUnauthorized),
				schema:      &schema{ref: "#/components/schemas/ErrorResponse"},
			}
		}

		// Verification needs authentication state; final descriptions append to earlier ones.
		gen.describeVerificationHeaders(&routes[index])
		gen.describeOnboardingResponse(&routes[index])
		gen.describeBodyLimit(&routes[index])
	}
	data := render(title, servers, routes, gen.components)
	return genfile.Write(output, data)
}

func loadSettings(configDir string) (*config.Config, pagination.Limits, error) {
	cfg, _, err := config.LoadDir(configDir)
	if err != nil {
		return nil, pagination.Limits{}, err
	}
	limits, err := pagination.New(cfg.Pagination.MaxP, cfg.Pagination.MaxS)
	return cfg, limits, err
}

func routeRequiresAuthentication(path string) bool {
	return path != "/healthz" && path != "/pub" &&
		!strings.Contains(path, "/pub/") && !strings.HasSuffix(path, "/pub")
}

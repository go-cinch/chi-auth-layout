package authn

// SetRestrictedHTTPPolicy installs a capability-owned policy during startup.
// A missing policy denies all requests by identities restricted to recovery.
func (m *Manager) SetRestrictedHTTPPolicy(allow func(method, path string) bool) {
	m.restrictedHTTPAllowed = allow
}

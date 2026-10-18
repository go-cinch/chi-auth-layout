# Runtime localization and resource catalog

`I18N_CATALOG` uses `schema_version: 2`. Top-level keys are:

- `default_locale`, `locales`, `messages`: language registration and translations.
- `scopes`: independent permission scopes in display order.
- `operations`: exactly `["read", "create", "update", "delete"]` in that order.
- `resource_catalog.resources`: a flat resource map, keyed by `scope:module` for directories and `scope:module:resource` for resources. Each node has `kind`, optional `parent`, and ascending sibling `sort`. Directory nodes grant nothing. Resource nodes own `entries` and `operations`.
- `resource_catalog.buttons`: logical feature mappings keyed by `scope:module:resource:control`, with explicit `btn`, `http`, and `grpc` arrays. `http` entries contain `method,path`; gRPC paths identify full methods. A logical feature belongs to one resource operation. Several features may intentionally share an endpoint.
- `resource_catalog.defaults`: required empty compatibility object. Default actions explicitly select `config.resource_keys` and `config.feature_keys`.

Resource `entries` map menu identifiers to `{path,parent_menu_key?}`. Scope defines the application boundary; entries have no legacy type. Each resource operation defines explicit `entry_keys` and `button_keys` arrays. Supported operations are a subset of the four top-level operations. Page-entry grants are independent of selected features. Feature selection expands only the selected features' frontend controls and exact HTTP/RPC rules. Empty arrays grant no feature access; HTTP methods do not determine business operation categories.

Display labels live in `messages.<locale>.frontend.scopes`, `operations`, `resources`, and `features`. Existing `menus` and `buttons` namespaces label actual frontend identifiers. Translation text is never an authorization rule. H5, mini, and app directories are independent; empty scopes stay empty rather than copying Admin resources.

Dictionary writes validate the entire resource graph and all references before commit: no missing/cyclic/cross-scope parents, duplicate entries, unsupported operations, out-of-resource features, wildcard routes, or malformed endpoints. Language tags must be canonical and unique; enabled languages require messages, and the default language must be enabled. Invalid writes roll back without replacing the active snapshot.

The application installs this dictionary after migrations. Dictionary mutations synchronously replace the local immutable snapshot; a five-second worker detects changes from other replicas. Request authorization resolves from that same snapshot without reloading the dictionary from SQL. Configuration changes therefore affect subsequent requests without renewing tokens. The full catalog is returned as detached data to prevent callers mutating the active snapshot.

`GET /dictionary/pub/i18n` supports `f=default_locale,locales,scopes,operations,group_order,messages,frontend_messages` with `Cache-Control: no-cache`. Omitted `f` returns only `schema_version`, `default_locale`, and `locales`. `schema_version` is always present. `frontend_messages` returns `messages.<locale>.frontend`, including projected menu labels, without backend/shared messages; it cannot be combined with `messages`, which selects every message namespace. Unselected fields are omitted and selected empty collections remain arrays or objects. Metadata-only reads do not copy or transform messages or resource mappings. Vben requests `default_locale,locales,group_order,frontend_messages`. The public response omits `resource_catalog`. Authenticated administrators read mappings through `GET /action/catalog?f=scopes,operations,resource_catalog,defined_actions`. `GET /action/effective?scope=admin&f=btn,http,grpc` reports the signed-in user's effective access, including direct, role, group and built-in grants. The `scope` query selects the inspected scope; the request's own authorization scope remains unchanged.

Full mutations of `I18N_CATALOG`, including rename, disable, and deletion in a mixed batch, require the explicit developer publish capability or scoped all authority and join the permission-mutation transaction lock. Ordinary dictionary edits do not require that authority. Resource features are developer-maintained configuration. The permission editor submits feature references; the server generates actual rules and never treats submitted paths as authority. Backend errors and labels use the request language, then the configured default, then bootstrap messages or stable identifiers.

Permission-group renames use the dictionary PATCH request’s top-level
`group_renames: {"old:group:key": "new:group:key"}` with `value` and
`expected_value`. This one-time instruction is validated against the previous
and submitted group definitions, then migrates action membership in the same
permission-locked transaction. It is never stored in I18N_CATALOG. Published
catalogs omit `group_aliases`; old group identifiers are not resolved as aliases.

Resource moves use top-level PATCH `resource_renames` and `feature_renames`
with `value` and `expected_value`. Pending moves are not catalog fields. The
transaction validates the old/new definitions, updates ordinary bindings and
default resource/feature selections, and rejects dangling final references.
Published catalogs keep no resource or feature aliases. The four-file initial
baseline stores only canonical resource and feature identifiers.

Optimistic updates limited to `group_order` and `frontend.groups` may be made by permission-grant managers. The expected JSON value is rechecked under the row lock; resource, feature, language and other message edits require developer publication authority.

The application owns one `localization.Module` and injects its catalog into auth,
action and dictionary modules. Permission rules and resource graphs live in that
business package; common i18n contains bootstrap messages and a translator interface.
HTTP routers obtain the translator through the shared module contract. Independent
application instances never share mutable language or permission catalog state.

Action owns resource-reference migration SQL. Dictionary calls its consumer-owned
resource-permission interface with the transaction callback context, so catalog,
action references and group changes commit or roll back together. Action obtains
authoritative definition catalogs through the dictionary interface in the same
transaction rather than querying dictionary storage directly.

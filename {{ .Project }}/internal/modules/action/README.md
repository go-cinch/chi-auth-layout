# Scoped permission catalog

`t_action` stores stable assignment codes, `scope:operation:module:resource` words,
groups based on the selected resource, explicit `config.bindings` entries with
`resource_key`, `operation` and `feature_keys`, and generated
display projections (`ui,endpoints`). Permission words are editable identities;
renaming a word does not change its resource, operation, features or group.
`I18N_CATALOG.resource_catalog` is the mapping authority. Reads and request
authorization resolve the current snapshot; stored transport projections are never
used to enlarge access. An omitted feature selection means the operation's complete
registered set; explicit `[]` selects none and remains empty after JSON persistence.
The four-file initialization baseline stores these associations directly. Runtime
reads, writes and authorization require explicit bindings and never derive an
ordinary resource or operation from the action word.
Every endpoint stores an explicit `method`: HTTP verbs for `protocol=http`, and
`GRPC` for `protocol=grpc`. The derived `resource` display joins `method|path`
entries with newlines; gRPC paths are stored without separator prefixes.
Resource moves update binding keys and groups while preserving permission words
and their action-name translations. Word scope must match the binding scope.
The editor toggles multiple resources and directory descendants explicitly. Linked
resources use their canonical keys, appear in catalog child order, and synchronize
selection with the original tree location. Removing one binding preserves the
operation and features of other bindings; an empty list grants no resource access.
One resource may have several bindings with different operations. The schema
requires each resource/operation pair to be unique. The
editor's All shortcut selects every registered operation and feature for that
resource explicitly; it does not select other resources or a built-in capability.
Technical information previews this action's generated mappings only and does not
request or compare the signed-in user's effective permissions.

Business operations are exactly `read/create/update/delete`. Frontend button IDs
such as `:edit` or `:send` remain control identifiers. Export is a selectable feature
under read, and sending a message is a feature under create. Built-in all/default
and the explicit grant capability are separate system mechanisms.

Display names come from `frontend.actions[word]`, then the actual bindings' resource
and operation labels, then the stable word. Multiple binding labels retain binding
order, deduplicate identical names, and join with ` / `. Renaming the permission
word never changes this fallback. Lists, details, associations, and keyword/name
searches use the same locale fallback and binding-derived labels. Name-only reads
load binding identities without expanding UI or endpoint mappings. Keyword searches
use parameterized SQL; only requests with name/keyword filters load candidate names.

`GET /action/{id}` supports `f=id,created_at,updated_at,code,name,names,group,word,scope,config,resource,menu,button`.
Omitting `f` preserves the complete detail response. An explicit selection returns
exactly those fields without always-present metadata. `name` loads the hidden
word/config identities and localized labels; `names` loads only the word and
registered-language overrides. Mapping fields load word/config and the resource
catalog. Unselected mappings and name transformations are skipped. Empty, unknown,
duplicate selectors and repeated `f` parameters return HTTP 400; an empty selected
`names` map remains `{}`.

Create and update accept optional `names: {"<registered-locale>": "<name>"}`.
Details return the explicit names for every registered catalog language. Names
are stored only in `I18N_CATALOG.messages.<locale>.frontend.actions[word]`;
an empty name removes that language's override. Unknown languages are rejected.
Word changes move existing name keys, including translations omitted from the
request. The action and name changes commit in one permission-locked transaction,
then refresh the dictionary cache and localization snapshot. Seeded ordinary actions remain
editable under the administrative grant rules and configured protection switch.

`GET /action/catalog` supports `f=scopes,operations,resource_catalog,defined_actions`;
omitted `f` returns only scopes and operations. Definition references are queried
only when selected. The editor detects existing resource-operation definitions and
opens the existing record instead of attempting a duplicate insert.

`GET /action/effective?scope=...` supports `f=btn,http,grpc`; omitted `f` returns btn.
The always-present `scope` describes the inspected scope. The result is the signed-in
user's current effective access after the same administrative restrictions used by
authorization. Catalog reads require the administrative grant or scoped all;
effective-access diagnostics require scoped all authority.

The built-in `default` action supplies only its explicit rules to authenticated
users. A scoped `all` action expands to the current catalog in that scope, including
future entries; it never grants an unrestricted HTTP wildcard or another scope.
User, role and group assignment codes are unioned and deduplicated on every request,
so changes take effect without renewing a session. Assignment management requires either a scoped `builtin=all` action or the exact
`scope:grant:system:permission-admin` capability, together with each resource's ordinary
read/write/delete endpoint permission. The grant capability has no UI bindings or
endpoints of its own. Holders may assign existing permissions, including all/grant,
to themselves or other accounts, as an explicit administrative privilege.
The qualification is checked in the request scope: an admin management grant may
manage existing permissions for mini/h5/app accounts from the admin entry point.
It cannot be reused as a mini-scope management qualification, and does not itself
grant the business API permissions of any scope.

The same qualification governs user role/action assignments, role/group action
assignments, all group membership changes, and deletion of roles/groups containing
permissions or members. Ordinary account/group/role creation and non-grant fields
remain governed by their existing endpoint permissions. Empty roles/groups remain
deletable without assignment qualification.

Action definition create/update/delete and changing an existing account's username
or password require the administrative grant or scoped all plus the matching
resource endpoint permission. Built-in all definitions remain exclusive to scoped
all; built-in defaults additionally require developer publication authority.
Self-service password changes retain their existing authenticated-user rules. New
accounts may receive initial credentials without assignment qualification if they
receive no role or explicit actions.

These guards run inside the mutation transaction and use a shared PostgreSQL
transaction advisory lock before checking the current caller's grants. A queued
operation rechecks after a preceding revocation commits. Production wiring injects
the policy into every management module; a missing policy fails closed for any
verified HTTP identity. Info omits action-definition and group-membership controls
without all/grant, matching the backend policy.

Protected HTTP requests select one scope using `X-Auth-Scope`; omission means
`admin` for existing clients. Empty, malformed or repeated headers are rejected.
The permission gateway uses the same scope. Permissions from different scopes
are never unioned, even when they map to the same endpoint. The header also selects
the scope returned by info.

`GET /auth/info` returns permissions for the header-selected scope. It accepts no
scope query selector; legacy query values are ignored. Empty, malformed or repeated
scope headers return HTTP 400. The response contains `scope`, `permission.words`
and `permission.menus`, whose entries have `menu_key`, optional `parent_menu_key`,
`menu_path`, and `btns`. HTTP/RPC mappings never appear in this response.

A menu is returned only when at least one granted binding has `page_entry: true`.
Buttons from other granted operations on that same menu are merged. Possessing an
edit action alone does not enable entry to the page. Buttons are a UI control layer;
backend business operations remain governed by their words and endpoint mappings.
Several controls, such as query/reset/refresh, may share the same GET endpoint. Removing one control does not revoke an endpoint still provided by another selected feature.

HTTP authorization uses the registered router's static-route precedence before
comparing canonical templates. `/user/{id}` cannot grant the registered static
`/user/summary` route. Parameters match a single segment; legacy glob rules are not
accepted. RPC mappings, when enabled, match the complete RPC method exactly.

Action group identifiers must be registered in the current I18N_CATALOG.
Create and update recheck the database catalog under the permission mutation
lock so an old form cannot write a renamed or deleted identifier back. Group
renames migrate existing action membership once; no runtime aliases are kept.

The default roles are Super (scoped all), Guest (overview), Admin (explicit codes
except scoped all, developer and dictionary), Developer (all explicit non-all
codes), and Operator (default plus workspace). Role names do not grant authority;
only their assigned action codes do. Existing account and group links are preserved.

Selected `config`, `resource`, `menu` and `button` fields are resolved from the same
application catalog snapshot as details and authorization. Sparse reads fetch only
the hidden word/config dependencies needed by those selected fields.

Query `scope` remains a data selector on management reads: `/action` filters
permission definitions and `/action/effective` chooses the scope being inspected.
Their caller authorization always uses `X-Auth-Scope`. Message-list `scope` selects
`all` versus `targeted` delivery and is unrelated to permission domains.

Catalog metadata and effective permission diagnostics expand only the requested
results and their necessary authorization dependencies. HTTP-only diagnostics do
not build menu/button or gRPC projections. Nullable `feature_keys` retains its
selection meaning: null selects the registered set; an empty array selects none.
OpenAPI declares that nullability explicitly.

Publishing resource mappings refreshes ordinary/default action projections in the
same transaction as the dictionary. Built-in all remains an empty stored marker;
its full mappings are resolved for reads, including edits submitted from details.
Resource/menu/button filters resolve candidate definitions from the same captured
catalog as the response, then pass matching words as bound SQL parameters before
counting and pagination. This also handles older persisted projections and scoped
all definitions. Requests without these filters skip the candidate query. Selected
mapping results reuse that resolution instead of expanding permissions again.

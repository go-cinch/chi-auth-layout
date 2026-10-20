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
resource explicitly. A resource with `permission_scope: "*"` additionally exposes the explicit `*` selection; ordinary resources do not imply wildcard authority.
Technical information previews this action's generated mappings only and does not
request or compare the signed-in user's effective permissions.

Catalog publication reconciles resource-to-directory conversions using the new
catalog's children and operation-filtered references. Existing bindings to the
converted resource become explicit selections of the reachable canonical
resources and their current features for each previously selected operation.
Overlaps are merged; an empty directory produces an empty selection. Default
resource selections expand only read operations. The conversion does not depend
on any resource key or action word, does not recreate removed features or special
capabilities, and does not add new nodes to the submitted catalog. Future
directory changes do not expand these saved selections automatically. Catalog
and action changes commit together, retaining action IDs, codes and assignments.

Business operations are exactly `read/create/update/delete`. Frontend button IDs
such as `:edit` or `:send` remain control identifiers. Export is a selectable feature
under read, and sending a message is a feature under create. A wildcard selection is separate from CRUD. The explicit default marker describes baseline rules; only assigned default actions grant them.

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
authorization. Catalog reads and effective-access diagnostics require the selected
`action:read` collection capability or scoped all authority. HTTP route authorization
also applies; the inspected scope never replaces the request authorization scope.

The built-in `default` action supplies its explicit rules only when assigned to
the authenticated user directly or through a role/group. A scoped `all` action expands to the current catalog in that scope, including
future entries; it never grants an unrestricted HTTP wildcard or another scope.
User, role and group assignment codes are unioned and deduplicated on every request,
so changes take effect without renewing a session. Assignment management follows the selected CRUD bindings of the current user/role/action/user-group resources. The bound operation must match the requested operation, and a selected feature must map to that resource's exact collection or item HTTP endpoint. For example, user update with `PATCH /user/{id}` permits changing that user's assignments; a menu-only selection, empty feature set, another resource, another CRUD operation, or a static subroute does not. Stored endpoint caches, action words, group labels, and the legacy `permission_resource` marker never grant authority. Mutations recheck the current catalog and assignments after acquiring the permission transaction lock.

Resources contain menus, reusable page/button references, transport mappings, and optional `permission_scope: "*"`. Actions select resources through `config.bindings`; resource definitions never reference action words. Selecting a wildcard resource with operation `*` grants the current authentication scope, including newly registered endpoints. Action words remain identifiers and can be renamed without changing authorization. Maintaining default or wildcard definitions requires catalog publishing authority as well as the specific action CRUD capability; existing wildcard authority satisfies both. The developer action explicitly references the current user, role, action and user-group CRUD bindings plus catalog publishing. It can maintain those definitions and assignments without receiving global wildcard authority.

The resource-scope migrations preserve action codes and existing assignments while converting historical built-in-all and named-management rows to explicit bindings. The legacy built-in-all marker remains read-compatible during upgrades; no action word grants authority. Default rules remain explicit assignments and are never injected during authorization.

Features may reference canonical same-scope page entries with `entry_keys` and their button IDs with `btn`. Shared references reuse existing menu definitions without associating a resource with an action.

### Resource selections on assignments

User, role and user-group create/PATCH accept `action_bindings` as an alternative to `action_codes`. Each item contains `resource_key`, `operation` and an explicit `feature_keys` array. Omission preserves assignments on PATCH; `[]` clears them. Do not combine the two assignment fields. Null feature sets, duplicate resource/operation pairs and unregistered selections are rejected.

The owner mutation checks its existing permission-management capability, then resolves selections against the committed catalog under the permission mutation lock. Each resource/operation reuses an action only when its complete resource/operation/feature set matches exactly. A larger feature set or a multi-resource action never satisfies a smaller selection. Missing atomic definitions are created with explicit feature arrays and server-generated codes. Creation and assignment share the owner transaction, so failed saves roll back the generated definitions. Concurrent identical selections reuse the committed action.

The resource editor expands existing action definitions for display. Permissions that cannot be represented by the current catalog block editing rather than being silently dropped. Backend authorization continues to resolve the stored action definitions against the current catalog.

Deleting an action removes its exact code from user, role and user-group assignments atomically with the definition. Other codes and their order are preserved. Legacy missing codes do not block the resource editor: it shows the remaining valid selections and marks the stale references for removal on save. Failed catalog or action requests still block editing; missing records are distinguished from failed requests.

### Default permission groups

`I18N_CATALOG.group_defaults` maps registered group words to Boolean switches.
Only `true` groups supply default permissions. Catalog publishing controls these
switches; group-name/order-only authority cannot change them. User creation and
public registration resolve every current code in these groups under the shared
permission mutation lock and persist the deduplicated union with selected codes.
Explicit selections retain their normal grant checks. Account updates never call
this resolver, and authorization no longer injects built-in defaults. Direct
permissions can therefore be removed later; role/group grants remain independent.
Renames move the switch and deletions remove it. Existing users are unaffected by
later default-group changes. The baseline stores initial users’ default grants as explicit assignments in migration 02.

The current catalog protocol retires `resource_catalog.defaults` and resource `collection`.
New writes reject both fields. Existing hidden navigation and read references are
preserved through explicit `frontend_visible` and `reference_operations` values.
Default actions require explicit `resource_keys`; nonempty legacy default maps
are rejected rather than silently changing authorization. Vben upgrades old
browser drafts containing the empty map and legacy collection marker before save.

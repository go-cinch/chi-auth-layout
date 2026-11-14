# Command Guidelines

Follow the [root guidelines](../../AGENTS.md) and the [module contracts](../modules/AGENTS.md) when generating API documentation or module registrations.

## Organization

- Keep each command in its own package under `internal/cmd/<command>`. Keep `main.go` limited to flags, calling the command, reporting errors and exiting.
- Put a generator's `run` function and orchestration in `generate.go`. Use behavior names such as `rewrite.go` for non-generation commands.
- Split implementation by responsibility within the same package. Use `discover.go` for input discovery, `source.go` for Go source loading, `schema.go` for schema inference, and `render.go` for output serialization when those responsibilities warrant separate files. These are conventions, not required placeholders.
- Preserve cohesive behavior files such as `compile.go`, `output.go`, `list_fields.go`, and `detail_fields.go`. In docsgen, prefix auth-specific supplements with `auth_`.
- Review files approaching 1,000 lines. Do not merge small cohesive files solely to reduce file count or introduce subpackages solely because a package has many files. Avoid generic `utils.go`, `helpers.go`, or `misc.go` files.
- Share generated-file diffing and atomic replacement through `internal/common/genfile`. Keep that package independent of generated configuration and business packages so configgen can bootstrap. Command-specific validation and stale-output deletion stay with the command.
- Keep seedcode independent: the scaffold hook removes it after initialization. Preserve its cross-file code mapping and existing file permissions.

## Documentation Generation

- Keep the supplement order explicit in `generate.go`: base inference, business supplements, security rules, then final descriptions. Supplements may overwrite earlier descriptions or depend on authentication state; moving files must not reorder calls.
- Keep rendering separate from source analysis and business supplements. Place serialization helpers with the renderer.

## Verification

- Move tests with their implementations and keep filenames aligned. Keep full generation tests in `generate_test.go`; put focused parsing, schema and rendering tests beside those implementations.
- For structural refactors, render temporary projects for both routers with gRPC enabled and disabled. Run affected tests and compare generated configuration, module registries, protobuf bindings and OpenAPI against the pre-refactor output under the same configuration.
- Verify repeated generation leaves content unchanged. Exercise genfile creation, updates, unchanged files and output errors when modifying the shared writer.

# Architecture

FQL is a developer-time compiler and generated SDK integration layer. One project schema is authored across recursively discovered `.fql` and `.fQL` files in `fQL/schemas/`. Dart and TypeScript targets share schema/storage semantics. TypeScript uses the modular Firebase JavaScript SDK; see [its contract](typescript.md).

## Repository boundaries

| Path | Responsibility |
| --- | --- |
| `cmd/fql` | Process entry, build version, signal cancellation |
| `internal/cli` | Command parsing and watch lifecycle |
| `internal/project` | Safe project initialization, configuration, bounded discovery, fingerprints |
| `internal/syntax` | Lexer/parser and source positions |
| `internal/schema` | Language-independent schema graph and diagnostics |
| `internal/validate` | Semantic checks and normalization |
| `internal/generate` | Dart/TypeScript, rules, indexes, artifact ownership, staged writes and rollback |
| `tools/release` | Reproducible cross-platform binary archives and checksums |
| `tests` | Dedicated Go, Dart, and emulator verification |
| `examples/flutter` | Runnable emulator-backed example and committed generated artifacts |

The pipeline is source discovery → syntax tree → semantic validation → typed schema graph → generators → staged output. The internal graph is Go data, not a user-authored JSON schema. Only validated project schemas reach generation through the CLI. No schema definitions execute user code.

## Generated integration

A generated client library contains shared runtime validation/SDK delegation, model-specific codecs/getters, typed field predicates, repository classes, and create/update builders. Each target’s runtime source is embedded in the Go binary so generation works without a separate network download or runtime package. This keeps runtime/generator compatibility together in v0.1.

Models retain presence separately from null and expose immutable decoded state. Builders validate supplied operations; reads validate all returned data. The official SDK owns persistence, transaction retries, queries, listeners, and synchronization. Metadata and SDK failures remain visible.

Rules enforce supported resulting-document structure and explicit operation authorization. Defaults are client-side create conveniences. Privileged writers remain outside the rules boundary. See [enforcement](enforcement.md).

## Generation and compatibility

Identical configuration, source, and compiler produce identical artifacts. Sorted discovery and models, stable JSON metadata, and generated headers avoid machine-specific output. Output is versioned and owned through `fQL/fql.lock`; hand-edited or unowned conflicting files are protected. A project-local generation lock prevents concurrent generation; staged writes and rollback protect previous artifacts on ordinary failures. Multi-file replacement is not crash-atomic.

Language/configuration version `0.1`, lockfile format `2`, and compiler compatibility metadata are explicit. Unsupported versions are rejected. Watch mode rechecks bounded project inputs and retries after source/configuration errors; cancellation is controlled by the process context.

## Release boundary

Enums, embedded objects/bounded lists, managed timestamps, typed atomic operations, validating formatting, schema diffs, and read-only auditing are implemented; see [extensions](extensions.md). No migrations, cross-document integrity constraints, arbitrary validation expressions, alternate runtime backends, or generated server integrations are implied. Add features through specified semantics, generated-code checks, and appropriate runtime/rules tests rather than through speculative abstractions.

Authorization syntax is parsed into source-located expressions, expanded and typechecked against the combined schema, then rendered into Firestore rules without client-side policy evaluation. Named-policy calls are acyclic and bounded. Claims and reusable policy definitions participate in effective-schema diffs. See [authorization](authorization.md).

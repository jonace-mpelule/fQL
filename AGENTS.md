# FQL development rules

These instructions apply to the entire repository. More specific `AGENTS.md` files may add local guidance. Explicit user instructions take precedence.

## Product and architecture

- FQL is a schema compiler and generated, typed integration layer for Cloud Firestore. The core compiler and CLI are written in Go.
- The intended developer experience is inspired by Ent: explicit schemas, generated models, typed predicates, and controlled data operations.
- End users author the FQL language in `.fQL` files. Dart and TypeScript are generation targets, not separate schema-authoring languages.
- End-user projects organize models in `fQL/schemas/`, for example `users.fQL`, `messages.fQL`, and `conversations.fQL`. Discover schema files recursively and compile them as one schema. Do not require a single schema file.
- Keep project configuration outside individual model definitions. Generator targets and output paths belong in project configuration.
- Resolve model references across files and diagnose duplicate definitions and unresolved references with source locations. Discovery order must not change semantics.
- Use a Go parser, a syntax tree with source locations, semantic validation, and a typed internal schema representation before generation. JSON is not the schema-authoring format; serialization of the internal representation requires a demonstrated need.
- Generated clients use official Firestore SDKs. The Go engine runs during development or CI, not inside mobile applications. Do not introduce a mandatory runtime server or a competing synchronization engine.
- Dart/Flutter is the first implementation target; TypeScript follows once the shared semantics and first integration are proven. Do not tie the framework to a particular application.
- Schema grammar, generated API examples, configuration format, lockfile format, and exact CLI flags discussed during planning remain proposals until specified and tested. Do not present them as existing capabilities.

## Scope and design decisions

- Deliver small, complete vertical slices with observable acceptance criteria. Production readiness includes failure behavior, enforcement, compatibility, and verification for the capabilities actually shipped.
- Preserve Firestore's capabilities and limits. An Ent-like experience does not imply SQL semantics, arbitrary joins, foreign keys, or native uniqueness constraints.
- Keep parsing, semantic validation, code generation, and runtime integration separate. Generators consume a validated schema; they must not independently reinterpret its meaning.
- Document consequential decisions, alternatives, and compatibility implications near the relevant specification. Keep documentation proportionate to the change.
- Do not add speculative extension systems, migration frameworks, dependencies, or language targets to solve hypothetical future needs.
- Changes to agreed schema organization, enforcement guarantees, or runtime architecture require an explicit design discussion before implementation. Routine implementation choices do not require repeated permission.

## Schema and data semantics

- Specify each feature before implementing it: accepted values, storage representation, generated language types, read/write behavior, enforcement locations, and failure behavior.
- Distinguish missing fields, explicit nulls, and zero values. Preserve that distinction in create inputs, update inputs, encoders, decoders, and generated types.
- Distinguish setting a field, setting it to null, leaving it unchanged, and deleting it in partial updates.
- Validate writes before submission and validate decoded data before exposing typed results, including subscription events and cached reads.
- Never silently coerce malformed values, drop invalid documents, or invent missing required values. Return structured errors with model, field, and document context where available; exclude sensitive values from error output.
- Define defaults explicitly, including whether they apply on create or update. Defaults must not silently repair existing documents during reads.
- Define calendar dates, timestamps, timezone handling, server-generated values, numeric ranges, and precision explicitly. Account for differences between Go, Dart, TypeScript, and Firestore.
- Validate special operations such as server timestamps, increments, array transforms, batches, and transactions deliberately. Do not let them bypass constraints accidentally or assume unresolved server values are ordinary decoded values.
- Define behavior for unknown fields, older documents, incompatible schema changes, and invalid subscription events. Regeneration does not migrate stored data.
- Reject unsupported features with actionable diagnostics. Do not silently weaken a declared constraint.

## Security and enforcement

- Generated client validation improves correctness; it is not a security boundary. Clients can be modified or bypassed.
- Generate Firestore Security Rules for accurately representable document constraints and explicit authorization policies. Maintain a documented enforcement matrix for supported features.
- Treat authorization separately from structural validity. Default access must be denied unless an explicit policy grants it. Review overlapping rules so broader grants do not defeat intended restrictions.
- Validate resulting document state for creates and updates in rules, including field allowlists, required fields, nullability, and protected fields where supported.
- Privileged server SDKs bypass Firestore Security Rules. Any supported trusted backend integration must validate and authorize separately; never claim client generation protects those writes.
- Do not claim uniqueness, referential integrity, or cross-document invariants without a specified, tested enforcement mechanism and clear concurrency/offline limits.
- Make rules artifacts reviewable. Do not automatically deploy rules, modify production data, or merge generated rules with handwritten grants without an explicit supported workflow and user authorization.
- Never commit credentials, access tokens, service-account keys, or real user data. Do not print local environment file contents. Use synthetic fixtures and redact diagnostics.

## Generated clients and Firestore behavior

- Preserve official SDK behavior for listeners, caching, offline writes, transactions, and cancellation. Expose meaningful pending-write and server-confirmation states where relevant.
- Local validation success or an optimistic local event must not be described as server acceptance. Surface later permission and synchronization failures.
- Generate only query operations supported by the selected SDK and Firestore. Validate combinations where possible and return actionable errors otherwise.
- Generate indexes from explicit supported declarations or known query definitions. Do not claim to infer every index needed by arbitrary runtime queries.
- Do not add hidden reads, writes, retries, or network calls. Explain operations that materially affect cost, atomicity, latency, or offline behavior.
- Propagate cancellation and clean up listeners and other resources. Respect transaction retry behavior; do not introduce irreversible side effects in retried callbacks.
- Keep generated artifacts clearly marked and separate from handwritten extension points. Fix generators instead of patching generated files manually.
- Keep Firestore serialization and semantics shared in intent across targets while using idiomatic Dart and TypeScript APIs.

## Compiler and CLI quality

- Use the repository's module path and declared Go toolchain. Keep Go code formatted, idiomatic, and organized around proven package boundaries.
- Preserve file paths and source spans through parsing and validation. Diagnostics should identify the problem and a practical correction.
- Treat schema and configuration input as untrusted. Bound resource use and handle malformed input without panics or arbitrary code execution.
- Make generation deterministic: stable file ordering and formatting, no incidental timestamps or machine-specific paths, and identical output for identical inputs and tool versions.
- Validate inputs and prepare outputs before replacing generated files. Preserve existing artifacts on failure and never delete unrelated files. Restrict cleanup to known generated artifacts.
- Reject unsafe output paths and configuration that could overwrite schema sources or unrelated project files.
- Give CLI commands clear exit codes, actionable errors, and useful help. Keep secrets out of logs and machine-readable output.
- Define language, configuration, internal representation, runtime, and generator compatibility intentionally. Diagnose unsupported versions instead of guessing.
- Distribute verified binaries so end users do not need Go installed. Version runtime packages and generated code compatibility together where necessary.

## Branches and pull requests

- Use neutral, task-focused branch names (for example `feature/authorization` or `fix/release-packaging`), commit messages, and pull request titles/descriptions.
- Do not reference the agent, assistant, or automation vendor in branch names, commits, or pull requests, including self-references, signatures, and co-author trailers.

## Verification and delivery

- Keep routine GitHub Actions economical: one Linux runner, no OS matrices, and no duplicate feature-branch push plus pull-request runs. Retain meaningful compiler, generated-output, runtime, installer, and security-enforcement checks.
- Keep Flutter/browser integration, race detection, and fuzz campaigns available locally or through an explicit manual workflow; do not add them to every pull request without a demonstrated need and user agreement. Windows testing and release artifacts are out of scope until explicitly requested.
- Keep verification output concise: capture verbose logs on disk, inspect only failing excerpts, and report short pass/fail summaries. During fixes, rerun targeted tests; run the full applicable suite once at the final verification stage unless new failures justify another run.

- Inspect existing code and repository status before editing. Preserve unrelated user changes and avoid opportunistic cleanup.
- Check current primary documentation when implementation depends on SDK behavior, Security Rules expressiveness, limits, or platform support. Record important compatibility assumptions.
- Add meaningful tests for parser behavior, semantic boundaries, diagnostics, codecs, and security enforcement as those features are implemented.
- Keep test files in dedicated test directories, separate from implementation files. Do not place tests beside production source files. Organize test directories by component and test purpose; keep fixtures and golden files with their tests. For Go, use separate test packages that import the implementation rather than relying on same-directory tests or exposing internals solely for testing.
- Use golden or snapshot tests for generated output where useful, and also compile generated code. Text snapshots alone do not establish a working client.
- Test both accepted and rejected operations in the Firebase Emulator Suite, including invalid types, missing fields, unauthorized writes, protected fields, and partial updates.
- Exercise read validation against malformed and older documents, plus listeners, offline behavior, and server-generated values in appropriate SDK integration tests. Emulator tests alone do not prove device behavior.
- Test deterministic generation and stale-artifact detection. Add fuzz tests for parser and decoder boundaries when those components exist.
- For Go changes, run relevant tests, `go vet`, and formatting checks. Use race detection when shared-state or concurrency behavior changes. Run the applicable Dart/TypeScript checks when those targets change.
- Keep CI checks aligned with implemented features and make external dependencies explicit. Do not claim a test passed if tooling, credentials, networking, or an emulator prevented it from running.
- Report what changed, what was verified, and material remaining limits. Distinguish generated-output checks, local tests, emulator evidence, native-device evidence, and production behavior.
- Never describe planned functionality as shipped, bypass failing checks to declare completion, or expand a documentation-only task into implementation.

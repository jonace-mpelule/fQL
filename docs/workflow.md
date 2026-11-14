# Project workflow and troubleshooting

[Handbook](README.md) · [Quickstart](../README.md#quickstart) · [Data operations](data-operations.md)

Run project commands from your application's root, where the `fQL/` directory lives, or supply `--project /path/to/app`. FQL commands operate on local sources and generated artifacts except for an explicitly requested audit. Generation never deploys Firebase resources or migrates stored data.

## Configure paths and targets

Start a new FQL project with one target:

```sh
fql init --target dart
```

For TypeScript, use `fql init --target typescript` instead. Init creates `fQL/fql.config` and separate User/Message schemas, and refuses to overwrite existing files. It does not generate the client until you run `fql generate`.

The Dart configuration is:

```text
version = "0.1"
target = "dart"
schemas = "schemas"
output = "../lib/data/fql"
firebase = "../firebase/generated"
```

TypeScript initialization sets `target = "typescript"` and `output = "../src/data/fql"`. All paths are relative to `fQL/fql.config`, not the current terminal directory. The schema directory is discovered recursively. Paths must stay inside the project, must not overlap source/configuration/output locations, and cannot contain symlinks in managed paths.

One configuration selects one target. To switch an existing project, edit both `target` and the output path, review the change, and regenerate with a compiler supporting that target. Generation replaces only known, unmodified owned artifacts; do not run init again over an existing configuration. A target switch removes the old owned client, so update application imports too. See [configuration and compatibility contracts](language.md#project-configuration).

Commit `fQL/fql.config`, schema files, `fQL/fql.lock`, the generated client, and generated Firebase artifacts. The lockfile records compatibility, the schema comparison baseline, and owned artifact hashes. Do not edit it or generated files manually. Keep handwritten application helpers outside generated output directories.

## CLI task reference

| Command | Use it to |
| --- | --- |
| `fql init --target dart` / `--target typescript` | Scaffold configuration and schemas |
| `fql validate` | Validate the complete schema without writing output |
| `fql format` | Validate, then format schema files while preserving comments |
| `fql format --check` | Check formatting without writing |
| `fql generate` | Generate the configured client, rules, indexes, and lock metadata |
| `fql generate --check` | Fail on missing/stale generated artifacts without writing |
| `fql generate --watch` | Regenerate after edits; Ctrl+C stops the watcher |
| `fql diff` | Review schema changes against the last generated baseline |
| `fql diff --check` | Exit 3 if changes are detected; useful for explicit review gates |
| `fql diff --against /path/to/other/project --json` | Compare two validated project schemas as structured output |
| `fql audit --firebase-project PROJECT_ID --max-documents 100` | Explicit bounded read-only scan of stored documents |
| `fql help` / `fql version` | Inspect commands or the installed compiler version |

All project commands accept `--project PATH`. Normal exit codes are 0 for success, 1 for operation/validation failure, and 2 for invalid usage. Diff/audit use 3 for detected changes/invalid documents; audit also uses 4 for an incomplete scan. `diff` without `--check` reports changes without making their presence a failure. See [diff/audit contracts](extensions.md#diff-and-audit).

For ongoing development, run `fql generate --watch` in a separate terminal. Fix reported source errors and the watcher can recover; run `format` and review `diff` deliberately before starting the watcher when you need to assess schema compatibility, since every successful generation advances the diff baseline.

## Deploy rules and indexes

FQL emits complete Rules for its managed collections and denies other access by omission. Review the generated files before deployment. An application that also has handwritten collection rules needs an explicit reviewed integration plan: broad overlapping grants can defeat restrictions. FQL does not merge those rules for you.

For an app using the generated artifacts as its complete Firestore rules/index configuration, add this `firestore` section to the app-root `firebase.json`. Preserve existing sections for Hosting, Functions, emulators, or other services:

```json
{
  "firestore": {
    "rules": "firebase/generated/firestore.rules",
    "indexes": "firebase/generated/firestore.indexes.json"
  }
}
```

Use the paths from your FQL configuration if you customized them. This example uses the default Firestore database; see the official [Firebase CLI configuration reference](https://firebase.google.com/docs/cli#the_firebasejson_file) for named databases and project selection.

Install/configure the [Firebase CLI](https://firebase.google.com/docs/cli) and authenticate with the intended account. After generating, reviewing, and testing against a development project/emulator, explicitly deploy to your chosen Firebase project:

```sh
firebase login
firebase deploy --only firestore:rules,firestore:indexes --project YOUR_FIREBASE_PROJECT_ID
```

Replace `YOUR_FIREBASE_PROJECT_ID` deliberately. This command changes the selected project's deployed Rules and indexes; it is separate from FQL generation. Rules deployments can replace rules previously edited in the Firebase console. Confirm indexes finish building before relying on the hosted queries. See Firebase's [partial deployment documentation](https://firebase.google.com/docs/cli#partial_deploys).

For a local-only trial, the [Flutter example](../examples/flutter/README.md) has a synthetic project, an emulator configuration, sign-in, and a complete UI. Emulator enforcement is evidence for local behavior, not evidence of deployed security or production index readiness.

## Change a schema with existing data

1. Edit the appropriate schema files and run `fql format` and `fql validate`.
2. Run `fql diff` **before** generation and review field, policy, index, and named-type changes.
3. Plan any data repair/backfill and compatibility with older installed applications separately. Renames are remove/add; FQL does not infer a migration.
4. Generate and review the resulting client, Rules, indexes, and lockfile. Compile the app and test accepted/rejected requests using your SDK/emulator integration.
5. Coordinate data changes, Rules/index deployment, and app rollout based on the specific compatibility change. FQL does not choose a safe rollout order for you.

Reads are strict: unknown stored fields, missing required fields, or malformed values fail decoding. Defaults do not repair older documents. Even adding an optional field can affect older strict readers once newer clients start writing it. Regeneration alone changes no stored data.

### Audit stored documents

An audit is optional and read-only. Against a locally running Firestore emulator:

```sh
fql audit --firebase-project demo-fql --emulator 127.0.0.1:8080 --max-documents 100 --json
```

This deliberately uses privileged emulator reads and permits only loopback endpoints. To audit a hosted database, provision a short-lived OAuth token through your normal trusted workflow into `FQL_ACCESS_TOKEN`, using read-only IAM permissions; then run:

```sh
fql audit --firebase-project YOUR_FIREBASE_PROJECT_ID --max-documents 100 --json
```

Do not put a token in the command, commit it, or paste it into reports. Production audits incur Firestore reads. Add `--model Message` to restrict the scan or `--database DATABASE_ID` for a nondefault database. Results give safe model/document/field context without document values. A capped scan is incomplete, not clean. Audit does not repair documents or test client authorization; privileged reads bypass Rules. See [audit details and exit codes](extensions.md#diff-and-audit).

## Check generated artifacts in CI

After installing the intended compiler version and checking out committed generated output:

```sh
fql format --check
fql validate
fql generate --check
```

These local checks make no deployment. They do not establish that your application compiles or that its actual queries satisfy Rules. Run the application's own language checks and meaningful emulator tests as well. FQL's compiler development commands live in [contributing](../CONTRIBUTING.md), not in the application setup path.

## Troubleshooting

| Symptom | What to check or do |
| --- | --- |
| `fql: command not found` | Follow the installer's PATH command for the current terminal, or open a new terminal after its shell-profile change. Default destination is `~/.local/bin`. |
| Init refuses to overwrite files | The project may already be initialized. Inspect `fQL/fql.config` and existing schemas, then validate/generate; do not delete your schema sources to rerun init. |
| Unsupported language/configuration/lock compatibility | Check `fql version` and install a compiler that supports the feature/target. Do not hand-edit versions to bypass the check. |
| Duplicate or unresolved definition | Check every file under the recursive schema tree, including both `.fql` and `.fQL` spellings. Names must be unique; named types need no imports. |
| Unsafe/overlapping path | Keep sources and output separate, inside the project, with no symlinks in managed paths. Paths are relative to `fQL/fql.config`. |
| Generation reports modified owned files | Preserve any needed manual edits outside generated output, restore the generated files from source control, put the intended change in schemas/generator, and regenerate. Do not delete the lockfile to bypass ownership protection. |
| Generation says artifacts are stale | Run `format`, review `diff`, then `generate` with the intended compiler and commit the resulting artifacts. `--check` does not write them. |
| Permission denied | Confirm sign-in, matching UID, explicit policies, the intended Firebase project, and deployed Rules. Owner queries require the owner equality predicate. Rules can mask a missing document as denial. |
| Query fails on the server | Check the query shape, policy, generated/deployed indexes and index readiness. FQL supports a subset of SDK queries and does not infer all indexes. |
| Read/listener reports `decoding` | Inspect the reported model/document/field against the schema. Unknown keys, invalid values, and missing required fields are errors. Repair data through a deliberate workflow; defaults never repair reads. |
| `read-after-write` after create | The write committed but resolving its managed timestamps failed. Inspect the reported document ID and read permission before deciding what to retry. |
| Explicit-ID create fails offline or with `already-exists` | It is a create-only transaction requiring an existence read and connectivity. Update an existing document instead if that is your intent. |
| A write waits while offline | SDK acknowledgement is still pending. Local listener events are optimistic; use metadata and later error handling. |
| A nullable field remains after setting null | Null is a stored value. To remove an optional key, call its generated `removeFieldName()` method. |
| React Native SDK type mismatch | The generated TypeScript client accepts the modular Firebase JS SDK, not `@react-native-firebase/firestore`. JS SDK persistent caching is unavailable there. |

For structured runtime error codes and examples, see [error handling](data-operations.md#handle-errors). For a compiler or generator bug, include the compiler version, target, a minimal synthetic schema, the failing command, and redacted diagnostics. Do not include credentials or real user data.

## Command output and statistics

`init`, `validate`, `generate`, `format`, `diff`, and `audit` print a consistent
summary with status, relevant counts, and elapsed milliseconds. Generation reports
schema files, models, declared fields, enums, embedded object types, claims,
access policies, named policies, composite indexes, and output files. Field counts
include model fields and named object fields once per declaration, rather than
expanding each use of an object. Output counts include the client, Rules, index
configuration, and lockfile: four files, even when their contents are unchanged.
`generate --check` reports files checked instead of files generated.

```sh
fql generate
fql generate --json
fql validate --json
fql generate --watch --json
```

Use `--json` for scripts. Each report contains `command`, `status`, `exit_code`,
`duration_ms`, and `stats`; `target`, `mode`, `timings_ms`, `message`, and `error`
are included when applicable. Existing diff/audit report fields remain at the top
level. In JSON mode diagnostics are included in the report on stdout, without
additional human output on stderr. Exit codes retain their existing meanings.
Command help and version retain their conventional text output.

`duration_ms` measures command handling, including project loading and work, but
excludes process/toolchain startup and printing the final report. Generation also
reports `load_validate` and `generate` phases in `timings_ms`; the latter includes
rendering and artifact writing, or freshness comparison in check mode. Timings
are observations and never enter generated files or compatibility metadata.
Watch mode emits one report per rebuild or distinct recoverable error, each with
its own duration; JSON reports are newline-delimited. Idle polls emit nothing.
Failures report only completed measurements and never claim successful generation.

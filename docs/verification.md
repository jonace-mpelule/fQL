# v0.1 verification record

[User handbook](README.md) · [Quickstart](../README.md#quickstart)

Verified locally on macOS arm64, 2026-11-10. This record describes development-release evidence, not a production deployment or native-device certification.

## Environment

| Component | Version |
| --- | --- |
| Go | 1.26.2 |
| Flutter / Dart | 3.41.9 / 3.11.5 |
| Flutter `cloud_firestore` | 7.1.0 |
| Flutter `firebase_core` / `firebase_auth` | 5.1.0 / 7.0.1 |
| Node | 24.12.0 |
| Java | OpenJDK 21.0.13 |
| Firebase CLI / Firestore emulator | 15.35.0 / 1.22.0 |
| Playwright / Chromium | 1.65.0 / 158.0.8127.18 |

Dependency versions are locked in npm and Flutter lockfiles. Go uses only the standard library.

## Passing checks

- Go formatting, `go vet`, compiler/CLI/project/generator tests, and race detection.
- Parser fuzzing, including invalid tokens, incomplete schemas, and bounded inputs.
- Deterministic generation, stale-artifact detection without writes, CRLF portability, safe output ownership, path/symlink checks, and watch cancellation/error recovery.
- Dart analysis and 14 runtime/codec tests: presence/null/zero values, strict decoding, calendar dates, timestamps, literal defaults, Unicode, CRUD, duplicate protection, query pagination, malformed results, and listener recovery.
- 9 real Firestore emulator rules tests: owner-ID and owner-field access, authenticated/default-deny policies, malformed fields, defaults/allowed values, numeric limits, UTF-8 byte limits, leap dates, immutable fields, required query ownership predicates, and privileged-writer bypass.
- 6 real Flutter web SDK integration tests: CRUD and pagination, cache decoding of malformed legacy data, pending offline writes and server confirmation after reconnect, and authorization rejection after an optimistic offline write.
- Flutter web example compilation, including the toolchain's Wasm compatibility dry run. No separate Wasm runtime execution was performed.
- Five release archive builds and checksum verification; native archive installation and init/validate/generate/check with an empty PATH, demonstrating no Go/runtime tooling dependency for end users.
- Repeated release builds produced identical archive checksums for the same source, version, and toolchain.

## Reproduce

Prerequisites: Go, Make, Flutter, Python 3 for release smoke tests, Node matching `.nvmrc`, and Java 21+. On this machine, Java 21 is installed alongside the existing Java 17 and must be selected explicitly:

```sh
nvm use
export JAVA_HOME="$(brew --prefix openjdk@21)"
make setup-integration
make check-all test-race

go test ./tests/compiler -run='^$' -fuzz=FuzzParser -fuzztime=10s
make dist VERSION=0.1.0-dev.2
```

`make dist` runs the archive installation smoke test. To test an already built set:

```sh
python3 tests/release/verify_archives.py --version 0.1.0-dev.2
```

The browser tests explicitly register Flutter's web plugins because `flutter test` does not run an application's generated web registrant. Tests use real Firebase emulators with the synthetic project `demo-fql`. They never require production credentials. Chromium is only a development test dependency.

## Remaining acceptance boundaries

- GitHub Actions workflows are configured but have not been executed by a hosted runner in this local session.
- Android/iOS SDK behavior, native persistence, app lifecycle, and device UI have not been verified. Web cache tests cover the active session; durable persistence across browser restarts was not tested. The current runnable example targets desktop browsers; native project/host configuration is separate work.
- Cross-built Linux/Windows archives have been inspected; native execution was smoke-tested on macOS arm64 only. Hosted OS jobs provide additional checks once run.
- No production rules/indexes were deployed. Release publishing and checksum-verified installers are documented in the README; release metadata identifies the published versions. Production index readiness is not proved by emulator queries.
- No existing application data was migrated. Incompatible documents fail decoding, as specified.
- Archive checksums establish integrity. Release signing is not implemented. Publication is explicit through `make release` or the manual publishing workflow.
- Generated file replacement is staged with rollback on ordinary errors and guarded by a generation lock; a process crash is not a multi-file atomic commit. Inspect artifacts/lock metadata after an interrupted generation.

The [enforcement matrix](enforcement.md) describes which guarantees belong to clients, rules, and privileged backend integrations.

Extension verification also covers token-preserving/idempotent formatting, cross-file named types, read-only bounded audit scans, baseline schema diffs, recursive Rules validation, timestamp pending states, and typed atomic writes. Browser tests isolate SDK/auth instances per case and force long polling for the local emulator; an explicit within-case reconnect-to-batch scenario checks session continuity. These are test configuration choices, not production runtime changes.

The Go audit CLI was also exercised against malformed synthetic emulator documents: it reported field paths without stored values, exited nonzero, and left the documents unchanged. No production audit was run.

## TypeScript target (2026-11-11)

Focused checks for the added target: strict compilation with TypeScript 5.9.3
against Firebase 13.2.0 (including public API rejection checks), two runtime
checks for codecs/builders/query validation/atomic ownership, Go generator,
project and CLI tests, relevant Go vet/formatting, and unchanged Dart/current
TypeScript artifact checks passed. The new generator workflow test also covers
determinism and safe target switching while retaining handwritten files.

A single synthetic emulator integration scenario is provided as
`npm run test:typescript:emulator` after `npm run test:typescript`. Its local run
was blocked before test execution by the Firestore emulator download, which was
stopped rather than left running. It is available in manual Extended verification but is not claimed as
passing evidence here. No React Native/Expo device run, durable native cache,
production operation, or full existing Dart/emulator suite was run for this
change. The target uses the modular Firebase JavaScript SDK, whose React Native
support excludes persistent Firestore caching; see [TypeScript](typescript.md).

## Declarative authorization and filename compatibility — 2026-11-11

Verified locally on macOS arm64 with the pinned Go/Node/Java/Firestore emulator versions above:

- `make check`: complete Go suite, `go vet`, formatting, CLI build, generated-artifact freshness for Dart and TypeScript projects, and schema formatting checks passed.
- `npm run test:rules`: 18 tests passed (nine existing rule scenarios and nine declarative-policy scenarios). Tests compile rules from a mixed `.fql`/`.fQL` project at runtime, then exercise raw SDK requests so generated client checks cannot mask rules failures.
- Policy scenarios cover accepted device access; anonymous/UID/device impersonation; missing, malformed, stale, or revoked claims; suspension and missing/malformed related documents; block records; expiration, including equality with server request time; the full mailbox query and rejection when each required predicate is omitted; independent get/list grants; create ownership and protected partial updates; negation/disjunction guards; bool/float claims and calendar validity; and coordinated/rejected batch and transaction writes using getAfter.
- Compiler/project/format/diff tests cover source-located semantic errors, cross-file names, recursive policies, bounded expansion/access calls, invalid lookup IDs, mixed extension discovery, lowercase scaffolding, deterministic generation, and source-independent policy/claim diff baselines.
- A bounded five-second `FuzzPolicyParser` run with two workers completed over 113,000 executions without a failure. The manual Extended verification workflow includes this same short fuzz gate.

The mailbox query exposed an emulator limitation with explicit `is timestamp` guards. The generator uses exhaustive ordered timestamp comparisons against `request.time` instead: the complete query succeeds while malformed timestamps still deny, including under negation. The expiration equality test uses a server timestamp transform rather than assuming subsecond agreement between client and emulator clocks.

No Dart/TypeScript runtime code changed; their generated output remains byte-identical, with only compiler compatibility metadata advancing to `0.1.0-dev.4`. Native-device, real credential issuance, hosted CI, index deployment/readiness, and production behavior were not reverified for this extension. The rules suite does not establish physical-device identity, cache erasure, or clock-driven listener eviction. Production use requires the application's trusted device/session issuance mechanism, review of generated rules/indexes, explicit deployment, and application-specific query/device acceptance. See [authorization contracts](authorization.md).


## CI cost and release scope — 2026-11-11

Routine CI now has one Ubuntu job, replacing the three-OS Go and installer matrices and automatic Flutter/browser integration. Automatic CI is now limited to ready pull requests; main/feature-branch pushes and merges do not duplicate it. Explicit manual runs remain available. The retained checks are `make check installer-test`, TypeScript compilation/runtime tests, and Firestore security-rules tests. Flutter analysis/tests/browser integration, race detection, and parser fuzzing remain available in the explicitly dispatched Extended verification workflow. No schedules or automatic extended runs are configured.

Windows binaries, ZIP packaging, the PowerShell installer, and its test have been removed. New release packages contain four macOS/Linux archives (arm64/amd64), install.sh, and SHA256SUMS. Existing published releases were not modified.

Local validation for this change: workflow YAML parsed successfully; `make check installer-test` passed, including all six installer tests; TypeScript compilation and two runtime tests passed; the release builder produced four real archives, verified checksums and archive contents, and ran the native CLI through init/validate/format/generate/check/diff with an empty PATH. No production release was published. Hosted CI for this revision is reported separately in the pull request; expensive manual verification was not rerun for these workflow/packaging changes.


## PR-only CI triggers — 2026-11-11

Removed the main push trigger after confirming it repeated the successful PR suite on the merge commit. CI now runs automatically for opened, synchronized, reopened, and newly ready PRs; draft PR jobs are skipped. Core checks retain their existing single Linux job and test commands. Release and extended verification workflows remain manual. Four additional recent CI runs belonged to distinct Dependabot PRs, not the merge event.

Validation for this workflow-only change: all workflow YAML parsed successfully; CI has only pull_request/workflow_dispatch triggers, explicitly includes ready_for_review, and guards draft jobs. Release/extended workflows retain only workflow_dispatch. No compiler, SDK, emulator, or release tests were rerun locally because their implementation and commands did not change.

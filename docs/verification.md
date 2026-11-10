# v0.1 verification record

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

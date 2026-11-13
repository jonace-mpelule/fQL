# Contributing to FQL

[README](README.md) · [User handbook](docs/README.md)

These instructions are for developing the compiler and maintaining releases. To use FQL in an application, follow the [quickstart](README.md#quickstart).

## Build from source

Use Go 1.26.2 and Make:

```sh
make setup
make build
./bin/fql help
```

## Develop and verify

Routine CI uses one Linux job for compiler checks, generated-artifact freshness, Unix installer tests, TypeScript compilation/runtime tests, and Firestore rules enforcement. It runs when a ready pull request is opened, updated, or reopened, when a draft becomes ready for review, or when explicitly requested manually. Draft PRs skip the job. Branch pushes, tags, and merges to `main` do not start another CI run. Flutter/browser integration, race detection, and fuzzing are available through the manual **Extended verification** workflow and the local commands below. Windows binaries and installation are currently outside the release scope.

The compiler has no third-party Go dependencies. Tests are separate from implementation under `tests/`.

```sh
make check                 # Go formatting, vet, tests, build, stale-artifact checks
make test-race
make coverage
make dart-check            # Requires Flutter 3.41.9 / Dart 3.11.5
make typescript-check      # Strict generated-code compilation + focused runtime checks
```

For integration tests, use Node 24.12.0 (`nvm use`), Java 21+, and Flutter:

```sh
make setup-integration
make emulator-test         # Generated Security Rules with real Firestore emulator
make integration-test      # Generated Dart with real Flutter web SDK and emulators
make audit-test            # Real Go audit CLI against synthetic emulator data
```

`setup-integration` installs locked local npm dependencies, Flutter dependencies, and Playwright Chromium for automated web SDK testing. Chromium is not a runtime dependency of FQL or generated applications. Point `JAVA_HOME` and `PATH` at Java 21 if another Java version is your default.

The [Flutter example](examples/flutter/README.md) demonstrates the complete local workflow. [Verification notes](docs/verification.md) distinguish local tests, emulators, device acceptance, and deployment.

## Distribution

```sh
make dist VERSION=0.1.0-dev.2
```

This builds local archives and installers with `SHA256SUMS`. To build, verify, and publish everything to GitHub:

```sh
gh auth login
make release VERSION=0.1.0-dev.3
```

Start from a clean, committed checkout that has been pushed to the official repository. The command runs release/installer checks, builds four macOS/Linux platform archives, verifies the native binary, uploads six assets into a draft, downloads and verifies the uploaded bytes, and only then publishes. Preview versions are marked prereleases. Published versions cannot be overwritten; a failed draft upload can be resumed from the same source commit.

The **Publish release** GitHub Actions workflow provides the same flow with a version input; its GitHub token needs only the configured repository contents permission. Release checks do not run Flutter or Firebase tests. Choose a new version for every published release. Maintainers need Go, Make, Python 3, and authenticated `gh`; users only need the installer and standard OS download/archive tools. Checksums provide integrity checking, not a signing guarantee.

See [extension syntax and semantics](docs/extensions.md) for enums, embedded data, server timestamps, atomic operations, and CLI contracts. Run `fql format`, then review `fql diff`, before generating a changed schema. Production audits use `FQL_ACCESS_TOKEN` and incur reads; no production scan or data change runs automatically.

See [AGENTS.md](AGENTS.md), [architecture](docs/architecture.md), and the [v0.1 checklist](docs/v0.1-features.md).

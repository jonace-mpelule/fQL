# FQL — Firestore Query Language

FQL compiles separate schema files into an Ent-inspired, typed Dart or TypeScript client for Cloud Firestore, Security Rules, and index configuration. The compiler and CLI are Go; application code uses the official Firebase SDK for its target. No runtime server is required.

**Version: 0.1 development release.** Implemented: schemas, validation, Dart generation, CRUD, queries, subscriptions, rules/index generation, and CLI workflows. Includes typed enums, nested objects/bounded lists, managed server timestamps, atomic batches/transactions, schema diffs, read-only audits, and validating formatting. Relationships, uniqueness, migrations, and other field transforms remain outside this release. Read the [enforcement boundaries](docs/enforcement.md) before integration.

## Install the CLI

macOS/Linux (no Go required):

```sh
curl -fsSL https://raw.githubusercontent.com/jonace-mpelule/fQL/main/install.sh | sh
```

The installer selects your OS/CPU, verifies the archive checksum and executable version, and installs into `~/.local/bin`. It adds a PATH entry to the supported shell profile when needed; follow its printed command for the current terminal. It never uses sudo.

Pin a version or choose another directory:

```sh
curl -fsSL https://raw.githubusercontent.com/jonace-mpelule/fQL/main/install.sh | sh -s -- --version 0.1.0-dev.3
```

Windows PowerShell:

```powershell
$installer = Join-Path $env:TEMP 'fql-install.ps1'
Invoke-WebRequest https://raw.githubusercontent.com/jonace-mpelule/fQL/main/install.ps1 -OutFile $installer
& $installer
```

Or download the platform archive from [GitHub Releases](https://github.com/jonace-mpelule/fQL/releases). The default installer chooses the latest stable release, falling back to the newest published preview when there is no stable release. Run it again to upgrade; use `--no-modify-path` (PowerShell: `-NoModifyPath`) to manage PATH yourself. Only the official Firestore package and normal Firebase configuration are needed in your Flutter project.

## Try it

Build the CLI with Go 1.26.2 and Make:

```sh
make setup check
./bin/fql init --target dart --project /path/to/flutter_app
./bin/fql validate --project /path/to/flutter_app
./bin/fql generate --project /path/to/flutter_app
```

In the Flutter app, add `cloud_firestore: ^7.1.0`, configure Firebase through the official FlutterFire workflow, and inject its SDK instance:

```dart
import 'package:cloud_firestore/cloud_firestore.dart';
import 'data/fql/fql.dart';

final db = FqlClient(FirebaseFirestore.instance);
final message = await db.messages.create()
    .setOwnerId(currentUserId)
    .setBody('Hello')
    .setCreatedAt(Timestamp.now())
    .save();

final changes = db.messages.query()
    .where(MessageFields.ownerId.eq(currentUserId))
    .orderBy(MessageFields.createdAt, descending: true)
    .limit(50)
    .watch();
```

Generated files belong in source control and must not be edited by hand. Generation does not deploy rules/indexes or migrate existing data. Review the generated Firebase artifacts and configure your own explicit deployment workflow. The initialized schemas use ownership policies; authenticate before accessing them.

## TypeScript / React Native

```sh
npm install firebase
fql init --target typescript
fql generate
```

TypeScript projects use `src/data/fql/fql.ts` by default:

```ts
import { getFirestore, Timestamp } from 'firebase/firestore';
import { FqlClient, MessageFields } from './data/fql/fql';

const db = new FqlClient(getFirestore(app));
await db.messages.create().setOwnerId(uid).setBody('Hello')
  .setCreatedAt(Timestamp.now()).save();

const unsubscribe = db.messages.query()
  .where(MessageFields.ownerId.eq(uid))
  .orderBy(MessageFields.createdAt, 'desc').limit(50)
  .watch(snapshot => console.log(snapshot.data), error => console.error(error));
// Return unsubscribe from React useEffect to clean up the listener.
```

This target uses the modular Firebase JavaScript SDK, including in React Native
and Expo. It does not accept `@react-native-firebase/firestore` instances.
Firestore persistent caching is unavailable in React Native with this SDK.
See the [TypeScript guide](docs/typescript.md) for semantics, setup, and limits.

## End-user project layout

```text
fQL/
  fql.config
  fql.lock
  schemas/
    users.fQL
    messages.fQL
lib/data/fql/fql.dart
firebase/generated/
  firestore.rules
  firestore.indexes.json
```

The compiler recursively discovers `.fQL` files, including nested folders. Configuration lives outside model files. See the [language specification](docs/language.md) for syntax, types, and presence/default semantics.

## CLI

| Command | Behavior |
| --- | --- |
| `fql init --target dart` / `--target typescript` | Create configuration and separate sample schemas without overwriting files |
| `fql validate` | Validate all schema files without generating output |
| `fql generate` | Generate the configured client, rules, indexes, and compatibility metadata |
| `fql generate --check` | Exit nonzero for missing/stale artifacts; never write |
| `fql generate --watch` | Regenerate on changes and recover after source errors; Ctrl+C stops |
| `fql format` / `fql format --check` | Validate the complete schema, then format files / check without writing |
| `fql diff` | Explain effective changes against the last generated baseline; supports `--against`, `--check`, and `--json` |
| `fql audit --firebase-project ID` | Explicit, bounded, read-only scan; supports `--model`, `--max-documents`, `--emulator`, and `--json` |
| `fql help` / `fql version` | Show usage / build version |

All project commands accept `--project PATH`; the default is the current directory. Exit codes: 0 success, 1 operation/validation failure, 2 invalid usage; diff/audit additionally use 3 for detected changes/invalid documents and audit uses 4 for an incomplete scan. Paths stay within the project; generated outputs are staged and owned through the lockfile. See the specification for failure recovery and safety limits.

## Develop and verify

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

Start from a clean, committed checkout that has been pushed to the official repository. The command runs release/installer checks, builds five platform archives, verifies the native binary, uploads eight assets into a draft, downloads and verifies the uploaded bytes, and only then publishes. Preview versions are marked prereleases. Published versions cannot be overwritten; a failed draft upload can be resumed from the same source commit.

The **Publish release** GitHub Actions workflow provides the same flow with a version input; its GitHub token needs only the configured repository contents permission. Release checks do not run Flutter or Firebase tests. Choose a new version for every published release. Maintainers need Go, Make, Python 3, and authenticated `gh`; users only need the installer and standard OS download/archive tools. Checksums provide integrity checking, not a signing guarantee.

See [extension syntax and semantics](docs/extensions.md) for enums, embedded data, server timestamps, atomic operations, and CLI contracts. Run `fql format`, then review `fql diff`, before generating a changed schema. Production audits use `FQL_ACCESS_TOKEN` and incur reads; no production scan or data change runs automatically.

See [AGENTS.md](AGENTS.md), [architecture](docs/architecture.md), and the [v0.1 checklist](docs/v0.1-features.md).

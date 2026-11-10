# TypeScript target

Set `target = "typescript"` in `fQL/fql.config`, or scaffold with
`fql init --target typescript`. The default output is `src/data/fql/fql.ts`.
The same recursively discovered `.fQL` schema and validated compiler graph
generate TypeScript, Security Rules, indexes, and ownership metadata.

## Runtime contract

The generated library uses the modular `firebase/firestore` client SDK, injected
as `new FqlClient(getFirestore(app))`. Install `firebase` in the application.
It does not use the Admin SDK or `@react-native-firebase/firestore`; those
instances are not interchangeable. No server or additional FQL runtime package
is needed. React Native/Expo use the Firebase JavaScript SDK. Firebase documents
Firestore support there without persistent caching:
https://firebase.google.com/docs/web/environments-js-sdk . Native device behavior
requires separate acceptance; generating TypeScript does not establish it.

Language 0.1 storage and Rules semantics remain unchanged. Strings/booleans map
to `string`/`boolean`; integers and floats to finite safe-range `number` values
(integers additionally require `Number.isSafeInteger`). Dates use `FqlDate`,
timestamps use SDK `Timestamp` with microsecond precision. Enums are frozen
string constants with literal union types. Embedded objects use generated
immutable classes; lists are readonly frozen arrays with the existing bounds.
Defaults apply only to missing create fields. Timestamp defaults preserve
microseconds rather than passing through JavaScript Date precision.

Optional getters include `undefined`, nullable getters include `null`, and
`hasField`/generated presence getters distinguish the two. Supplying `undefined`
is invalid. Omitted update setters leave fields unchanged, nullable setters
accept null, and optional fields have explicit remove methods. Immutable and
managed fields have no update setters. Unknown fields, malformed stored values,
and missing required fields fail with contextual `FqlError` codes; values and
raw SDK messages are excluded. Reads never apply defaults.

Managed timestamps use `FqlServerTimestamp` with `isPending` and `value`.
Unresolved null is accepted only on pending snapshots, including nested read
validation. Managed creates read back from the server after acknowledgement;
a failed read reports `read-after-write` because the write has committed.

CRUD, equality/range queries, ordering, limits, snapshot cursors, listener
metadata, create-only explicit IDs, and atomic operation limits follow the Dart
contracts in `enforcement.md` and `extensions.md`. Reads accept `default`,
`server`, or `cache`. `watch(next, error)` returns the SDK unsubscribe function;
the error callback is required so decoding/synchronization errors are observable.
Invalid result events call `error` without exposing a partial result; later valid
events can recover. Exceptions thrown by application callbacks are not recast
as decoding errors. SDK listener errors may terminate the listener.

Snapshots expose `fromCache` and `hasPendingWrites`; write promises wait for SDK
acknowledgement. FQL adds no cache, retry loop, or implicit deployment.
Transactions use SDK retries; callbacks must tolerate repetition. Batch
operations permit auto-ID creates; explicit-ID creates require transactions
with existence reads. Prepared writes capture validated values before I/O.
Rules and authorization have the same enforcement boundary as Dart; privileged
writers bypass Rules. Regeneration/diff/audit never migrate documents.

TypeScript-specific generated-name conflicts are rejected with source
locations. Language/configuration version and existing Dart output remain
unchanged. Lock format 2 additionally owns `fql.ts`; changing targets replaces
only known, unmodified generated artifacts. Older compilers do not support this
target and must be upgraded rather than used to regenerate these projects.

## Application use

```sh
npm install firebase
fql init --target typescript
fql generate
```

```ts
import { initializeApp } from 'firebase/app';
import { getFirestore, Timestamp } from 'firebase/firestore';
import { FqlClient, MessageFields } from './data/fql/fql';

const app = initializeApp(firebaseConfig);
const db = new FqlClient(getFirestore(app));
// Configure Firebase Authentication separately; policies require authentication.
await db.messages.create().setOwnerId(uid).setBody('Hello')
  .setCreatedAt(Timestamp.now()).save();

const unsubscribe = db.messages.query()
  .where(MessageFields.ownerId.eq(uid))
  .orderBy(MessageFields.createdAt, 'desc').limit(50)
  .watch(snapshot => console.log(snapshot.data), error => console.error(error));
// In React/React Native, return unsubscribe from useEffect for cleanup.
unsubscribe();
```

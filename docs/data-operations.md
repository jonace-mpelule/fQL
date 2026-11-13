# Work with data

[Handbook](README.md) · [Schema guide](schemas.md) · [Project workflow](workflow.md)

These recipes use the unchanged `Message` and `User` models from `fql init`, shown in the [quickstart](../README.md#quickstart). Run them in async application functions after Firebase initialization, sign-in, and loading the reviewed generated Rules. `uid` is the authenticated user's UID; `messageId` is the ID of an existing message that user owns. Each recipe is independent unless it explicitly continues the preceding snippet.

## Set up the client

Dart imports below assume an application file under `lib/`; TypeScript imports assume a file under `src/`. Import the generated types used by a recipe.

**Dart**

```dart
import 'package:cloud_firestore/cloud_firestore.dart';
import 'data/fql/fql.dart';

final db = FqlClient(FirebaseFirestore.instance);
```

**TypeScript**

```ts
import { getFirestore, Timestamp } from 'firebase/firestore';
import { FqlClient, FqlError, MessageFields } from './data/fql/fql';

const db = new FqlClient(getFirestore());
```

Use your configured SDK instance instead if you have a named Firebase app or a deliberately configured emulator connection. FQL does not configure authentication or emulator hosts.

## Create, read, update, and delete

**Dart**

```dart
final message = await db.messages.create()
    .setOwnerId(uid).setBody('Hello').setCreatedAt(Timestamp.now()).save();
final snapshot = await db.messages.get(message.id);
print(snapshot.data.body);

await db.messages.update(message.id).setBody('Updated').setPriority(2).save();
await db.messages.delete(message.id);
```

**TypeScript**

```ts
const message = await db.messages.create()
  .setOwnerId(uid).setBody('Hello').setCreatedAt(Timestamp.now()).save();
const snapshot = await db.messages.get(message.id);
console.log(snapshot.data.body);

await db.messages.update(message.id).setBody('Updated').setPriority(2).save();
await db.messages.delete(message.id);
```

Create returns the model, including its ID. Get returns a snapshot with typed `data`, `fromCache`, and `hasPendingWrites`. Update and delete complete without returning an updated model; read again if your UI needs it. The default priority is inserted on create when omitted. Invalid input fails validation; malformed stored documents fail decoding.

Auto-ID creates submit a write without an existence read. To choose an ID, use `.create(id: chosenId)` in Dart or `.create(chosenId)` in TypeScript. Explicit-ID creates use a transaction with an existence read, require connectivity/read permission, and reject existing documents rather than overwriting them. For the scaffolded User, the chosen ID must be `uid` because its policy is `owner(id)`.

## Leave unchanged, set null, or remove

The scaffolded User has an optional, nullable `bio`. These are separate update intents; the examples assume that the user's profile already exists.

**Dart**

```dart
await db.users.update(uid).setName('Alex').save(); // bio is unchanged.
await db.users.update(uid).setBio('Hello').save(); // Store a string.
await db.users.update(uid).setBio(null).save();    // Store explicit null.
await db.users.update(uid).removeBio().save();    // Delete the key.

final user = (await db.users.get(uid)).data;
print(user.hasBio); // false after removal; true for an explicit null.
```

**TypeScript**

```ts
await db.users.update(uid).setName('Alex').save(); // bio is unchanged.
await db.users.update(uid).setBio('Hello').save(); // Store a string.
await db.users.update(uid).setBio(null).save();    // Store explicit null.
await db.users.update(uid).removeBio().save();    // Delete the key.

const user = (await db.users.get(uid)).data;
console.log(user.hasBio); // false after removal; true for an explicit null.
```

Only optional fields have remove methods. Only nullable fields accept null. Immutable and managed fields have no update setters. Defaults do not reapply on update. Partial updates issue a write without first reading the document; deployed Rules validate the resulting full document. An empty update is invalid unless managed fields supply a change.

## Query and paginate

Filter Message queries by the current owner so they satisfy the ownership policy. These recipes use the index declared in the quickstart schema. `.where(...)` calls combine with AND; the generated predicate methods are `eq`, `gt`, `gte`, `lt`, and `lte`.

**Dart**

```dart
final query = db.messages.query()
    .where(MessageFields.ownerId.eq(uid))
    .orderBy(MessageFields.createdAt, descending: true)
    .limit(20);
final first = await query.all();
for (final message in first.data) {
  print(message.body);
}
final cursor = first.cursor;
if (cursor != null) {
  final next = await query.startAfter(cursor).all();
  print(next.data.length);
}
```

**TypeScript**

```ts
const query = db.messages.query()
  .where(MessageFields.ownerId.eq(uid))
  .orderBy(MessageFields.createdAt, 'desc')
  .limit(20);
const first = await query.all();
for (const message of first.data) {
  console.log(message.body);
}
if (first.cursor) {
  const next = await query.startAfter(first.cursor).all();
  console.log(next.data.length);
}
```

An empty page has no cursor. Keep the same predicates, ordering, limit, collection, and SDK instance when reusing a cursor; cursors stay in memory and are not portable serialized bookmarks. A cursor does not guarantee another nonempty page.

FQL supports up to 10 predicates, 8 order fields, one range field, and limits of 1–1000. When you use range predicates and an explicit order, the first order must match the range field. OR queries, joins, and array transforms are outside the generated API. A valid query can still fail if it lacks authorization or a deployed index. Ordering can omit documents missing the ordered field. See [query contracts](enforcement.md#queries).

## Listen for changes

Query listeners expose snapshot metadata so the UI can distinguish cached results and pending writes. Document listeners are also available via `db.messages.watch(messageId)` in Dart or `db.messages.watch(messageId, next, error)` in TypeScript.

**Dart**

```dart
final subscription = db.messages.query()
    .where(MessageFields.ownerId.eq(uid))
    .orderBy(MessageFields.createdAt, descending: true).limit(20)
    .watch().listen((snapshot) {
      print(snapshot.data.length);
      print(snapshot.fromCache);
      print(snapshot.hasPendingWrites);
    }, onError: (Object error) {
      if (error is FqlError) print(error.code);
    });

// Keep subscription while the view is active; cancel when it is disposed.
await subscription.cancel();
```

**TypeScript**

```ts
const unsubscribe = db.messages.query()
  .where(MessageFields.ownerId.eq(uid))
  .orderBy(MessageFields.createdAt, 'desc').limit(20)
  .watch(snapshot => {
    console.log(snapshot.data.length);
    console.log(snapshot.fromCache);
    console.log(snapshot.hasPendingWrites);
  }, error => {
    console.error(error.code);
  });

// Keep the listener while the view is active; call on unmount/disposal.
unsubscribe();
```

The final cleanup line demonstrates teardown; do not run it immediately if you want ongoing updates. In React, return `unsubscribe` from the effect. In Flutter, retain the subscription and cancel it from your widget/controller lifecycle, or use a `StreamBuilder` as in the [complete example](../examples/flutter/lib/main.dart).

One malformed document fails the entire result event; FQL does not silently drop it or expose a partial list. Subsequent valid decoding events can recover; SDK listener errors may terminate the listener. A deleted document on a document listener produces a `not-found` error.

Local events and validation success are not server acceptance. Watch `hasPendingWrites` and handle later errors. Writes await SDK server acknowledgement, so an offline write may remain pending. FQL adds no cache or retry loop. For deliberate cache/server reads, Dart accepts `get(messageId, source: Source.cache)` / `Source.server`, while TypeScript accepts `get(messageId, 'cache')` / `'server'`; query `all` accepts the same source options for its target. SDK cache support still depends on platform; see [TypeScript compatibility](typescript.md).

## Use batches and transactions

A batch queues validated prepared operations and commits them atomically. Create a fresh `.operation()` value for each atomic unit.

**Dart**

```dart
final create = db.messages.create().setOwnerId(uid)
    .setBody('First message').setCreatedAt(Timestamp.now()).operation();
final batch = db.batch();
batch.add(create);
batch.add(db.messages.update(messageId).setBody('Updated together').operation());
await batch.commit();
print(create.id);
```

**TypeScript**

```ts
const create = db.messages.create().setOwnerId(uid)
  .setBody('First message').setCreatedAt(Timestamp.now()).operation();
const batch = db.batch();
batch.add(create);
batch.add(db.messages.update(messageId).setBody('Updated together').operation());
await batch.commit();
console.log(create.id);
```

Deletes use `db.messages.deleteOperation(messageId)`. Explicit-ID creates need an existence check, so they must use a transaction rather than a batch. Atomic units allow at most 100 distinct documents from one injected SDK instance. Security Rules lookup budgets can impose a lower practical limit.

Use a transaction when a write depends on a read. This example toggles the priority between 0 and 1, within the schema's allowed range:

**Dart**

```dart
await db.transaction((tx) async {
  final current = await tx.get(db.messages, messageId);
  final nextPriority = current.data.priority == 0 ? 1 : 0;
  tx.add(db.messages.update(messageId).setPriority(nextPriority).operation());
});
```

**TypeScript**

```ts
await db.transaction(async tx => {
  const current = await tx.get(db.messages, messageId);
  const nextPriority = current.data.priority === 0 ? 1 : 0;
  tx.add(db.messages.update(messageId).setPriority(nextPriority).operation());
});
```

The SDK may repeat the callback; create prepared operations inside it and keep external side effects out of it. FQL queues writes until the callback completes, including required explicit-ID existence checks before submission. Transactions require connectivity. Batches preserve SDK offline queuing, and both must satisfy each document's Rules. Neither implies foreign keys or uniqueness. See [atomic contracts](extensions.md#atomic-writes).

## Handle errors

Catch failures around builder preparation as well as awaited I/O, since validation may fail before a request is sent.

**Dart**

```dart
try {
  await db.messages.update(messageId).setBody('').save();
} on FqlError catch (error) {
  print('${error.code}: ${error.model}.${error.field}');
}
```

**TypeScript**

```ts
try {
  await db.messages.update(messageId).setBody('').save();
} catch (error) {
  if (error instanceof FqlError) {
    console.error(error.code, error.model, error.field);
  } else {
    throw error;
  }
}
```

| Code | What to check |
| --- | --- |
| `validation` | Input presence, types, nullability, bounds, and immutable/managed fields |
| `decoding` | Stored data differs from the schema; reads do not insert defaults or repair documents |
| `permission` | Authentication, owner UID, query predicates, deployed Rules, and claims |
| `not-found` | The requested document does not exist; Rules can mask absence as permission denial |
| `already-exists` | An explicit-ID create found an existing document; choose update if intended |
| `query` | Invalid query combination, wrong-model field, or mismatched cursor |
| `read-after-write` | A managed-timestamp create committed, but its resolving read failed; inspect the document ID before retrying |
| `sdk` | Other SDK failure, including service/index/connectivity conditions; check configuration and Firebase diagnostics |

Errors include model, field, and document context where available, without rejected values or raw SDK messages. Present an appropriate application message and retain the safe context you need for diagnosis. Do not treat every failure as retryable, especially `read-after-write`. See [runtime behavior](enforcement.md) and [troubleshooting](workflow.md#troubleshooting).

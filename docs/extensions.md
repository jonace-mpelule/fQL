# v0.1 extensions: semantics and contracts

[User handbook](README.md) · [Quickstart](../README.md#quickstart)

These extensions retain the `0.1` language/configuration version and require compiler compatibility `0.1.0-dev.2`. Generation upgrades lock metadata to format `2` with an effective-schema baseline for diffs. Old format-1 manifests remain readable for artifact ownership, but have no diff baseline.

## Named enums, objects, and bounded lists

```fql
schema "0.1"
enum Delivery { draft sent }
object Address {
  fields {
    street string.required()
    city string.required()
  }
}
model Parcel {
  collection = "parcels"
  fields {
    delivery Delivery.required().default("draft")
    address Address.required()
    tags list<string>.required().maxItems(8)
    createdAt timestamp.required().serverCreated()
    updatedAt timestamp.required().serverUpdated()
  }
  access { read = authenticated create = authenticated update = authenticated delete = authenticated }
}
```

Enums store their member names as strings and generate Dart enums. Objects store maps with exact declared keys and generate immutable values with typed builders (`Address.create().setStreet(...).setCity(...).build()`). References resolve across files. Cycles and nesting deeper than four type levels are rejected. Objects are embedded data, not document relationships.

Lists contain nonnull scalar, enum, or named-object elements. They require `maxItems(0..16)` and may specify `minItems`. They generate immutable typed Dart lists. The upper bound permits complete unrolled element validation in Security Rules; unsupported/unbounded lists are rejected rather than enforced only in the client. Expanded document validation is limited to 128 values and a conservative expression-cost budget; combinations exceeding either budget are rejected. Unknown nested keys and invalid list elements fail with field paths. Nested updates replace the complete object/list through its typed root setter; dotted partial mutation and array transforms are not exposed. Mark the containing root field immutable when necessary; managed/immutable nested fields are rejected.

## Managed timestamps

`serverCreated()` and `serverUpdated()` apply only to required, nonnullable root timestamp fields without defaults or `oneOf`. Created fields are immutable; updated fields are set on every create/update. Generated builders do not expose setters. Rules require incoming managed timestamps to equal `request.time` on the relevant operation. This enforces stored semantics even for raw SDK callers.

Managed getters use `FqlServerTimestamp`: `isPending` is explicit and `value` is null only while unresolved. Decoding accepts unresolved null only for snapshots marked with pending writes. Committed nulls remain invalid. No local-clock estimate is silently substituted. Pending values cannot be serialized as confirmed data. Direct creates with managed fields perform an explicit server read after acknowledgement to return resolved models; this costs a read and requires read permission. If that read fails after the write commits, `read-after-write` explicitly reports the committed write and document ID; do not blindly retry the create. Prepared batch/transaction operations return IDs without a resolution read. Ordinary updates remain write-only.

## Atomic writes

`db.batch()` queues typed `.operation()` values from create/update builders and repository `deleteOperation(id)`. Adds validate before network I/O; commit is one SDK atomic batch. Auto-ID creates are supported. Explicit-ID create operations require existence checks and therefore must use a transaction. Prepared operations are owned by one atomic unit, preventing accidental reuse; each unit permits at most 100 distinct documents from one injected SDK instance.

`db.transaction((tx) async { ... })` supplies typed `tx.get(repository,id)` and `tx.add(operation)`. All writes are queued until the callback completes, then explicit-ID create existence checks run before any writes. SDK transactions own retries and require connectivity. Callback code can run repeatedly: do not perform external side effects there. Validation/decoding errors abort the transaction. Batches preserve SDK offline queuing. Server confirmation remains the commit future, not a local event. No foreign-key or uniqueness guarantee is implied.

## Formatting

`fql format [--project PATH] [--check]` validates the entire schema graph first, then formats all recursively discovered `.fql` and `.fQL` files. It preserves comments, literal spellings, declaration order, and modifier order. Invalid projects are never rewritten. Replacements are staged with rollback on ordinary write failures; check mode writes nothing and fails if formatting is needed. Format does not generate artifacts or deploy anything. Run format before generate so source fingerprints match the canonical files.

## Diff and audit

`fql diff` compares the current effective schema against the last successful generation baseline. `--against PROJECT` compares against another validated project instead. `--json` emits structured changes; `--check` exits 3 when changes exist. Renames are remove/add, never guessed. Field changes are conservatively breaking; policy, enum, object, and index changes require review. Adding optional fields can affect older strict readers. Diff does not migrate data. Review before regenerating, since successful generation advances the baseline.

`fql audit --firebase-project ID [--database ID] [--model NAME] [--max-documents N] [--json]` performs bounded GET-only Firestore REST scans. The default budget is 1000 documents across selected models. Production requires an OAuth token in `FQL_ACCESS_TOKEN`; use an account with read-only IAM permissions. The token is never a command-line argument or report value. Normal IAM read costs apply. Audit validates stored data without defaults or fixes and reports document/field paths without document values.

`--emulator 127.0.0.1:8080` deliberately uses privileged emulator reads; only loopback endpoints are accepted. Auditing bypasses client Rules when using privileged credentials and does not test authorization. Pagination, HTTP errors, redirects, response sizes, cancellation, and scan caps are handled explicitly. A capped/failed scan is not reported as clean. Exit codes: 0 complete and clean, 1 operation failure, 2 invalid CLI usage, 3 invalid documents, 4 incomplete budget-limited scan. No production scan runs automatically.

## Usage examples

```dart
final info = ConversationInfo.create().setTitle('Room').build();
final create = db.conversations.create()
    .setOwnerId(uid).setInfo(info).setState(ConversationState.open)
    .setTags(['team']).operation();
final batch = db.batch();
batch.add(create);
await batch.commit(); // create.id is available before commit

await db.transaction((tx) async {
  final current = await tx.get(db.conversations, create.id);
  tx.add(db.conversations.update(create.id)
      .setCount(current.data.count + 1).operation());
});
```

```sh
fql format
fql format --check
fql diff --json
fql generate
fql audit --firebase-project demo-fql --emulator 127.0.0.1:8080 --max-documents 100 --json
```

Emulator and production audits are explicit alternatives. Production scanning needs a separately provided OAuth token; it never falls back to emulator credentials.

## Declarative authorization and schema extensions

See [authorization](authorization.md) for typed expressions, trusted claims, related-document lookups, reusable policies, and atomic post-state checks. These extensions require compiler compatibility `0.1.0-dev.4`; language/configuration `0.1` and lock format `2` remain unchanged. New projects use `.fql`; existing `.fQL` schemas remain supported.

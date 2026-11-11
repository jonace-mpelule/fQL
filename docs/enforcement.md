# Enforcement and runtime behavior

| Feature | Generated Dart / TypeScript | Generated Security Rules |
| --- | --- | --- |
| Required / optional / null | Write validation and read decoding | Resulting document on create/update |
| Types and supported scalar ranges | Write validation and read decoding | Resulting document on create/update |
| Length, bounds, allowed values | Write validation and read decoding | Resulting document on create/update |
| Calendar dates / timestamps | Exact date codec, timestamp range/precision | Calendar/leap-year checks, timestamp range/precision |
| Defaults | Missing create inputs only | Validate supplied document; never insert values |
| Immutable fields | Update builders omit setters; runtime rejects attempted updates | Reject affected immutable keys |
| Unknown fields | Reject input and stored unknown keys | Reject unknown stored keys on create/update |
| Authorization | Surface SDK denial; no security guarantee from client checks | Typed expressions per operation, separate get/list, default denial |
| Typed auth claims / named policies | No local authorization or token issuance | Explicit claim types, bounded acyclic policy expansion; missing/invalid accessed security inputs deny |
| Related-document lookups / post-state checks | No hidden client lookups | Typed get/exists/getAfter; operation and aggregate rules access budgets apply |
| Existing malformed documents | Error on read/query/listener decoding | No retroactive cleanup or structural read filtering |
| Enums / objects / bounded lists | Typed immutable values and recursive codecs | Exact maps and unrolled bounded-element checks |
| Managed timestamps | Explicit pending/confirmed wrappers; managed writes | Request-time equality and created-field immutability |
| Batches / transactions | Prepared typed operations; SDK atomic commit | Every resulting document must satisfy its policy/schema |
| Uniqueness / relationships | Not supported | Not supported |

Rules apply to mobile/web client requests. Privileged Admin/server SDKs bypass them and need their own validation and authorization. FQL does not currently ship a trusted backend runtime. Generated types do not prevent a modified client from submitting raw SDK requests.

Generated rules are complete files for FQL-managed collections and deny other access by omission. Review them before explicit Firebase deployment. Do not paste broad allow rules alongside them: overlapping grants can broaden access. Generation fails for unsupported schema constraints/policies rather than emitting a weaker approximation.

## Reads and listeners

`repository.get(id)` returns `FqlSnapshot<Model>`; query `all()` returns `FqlSnapshot<List<Model>>`. The wrappers expose `fromCache`, `hasPendingWrites`, and query pagination cursors. `Source.server`, `Source.cache`, and `Source.serverAndCache` are supported through the official SDK.

Every returned document is decoded before exposure. One invalid query document fails that result event; no partial list is returned. Document/query listener decoding errors are stream error events. Subsequent valid events can recover without recreating the subscription; an SDK permission error can close the underlying listener. Cancelling the returned subscription cancels the SDK listener. Deletion on a document listener produces a `not-found` stream error.

Errors use `FqlError` codes: `validation`, `decoding`, `not-found`, `already-exists`, `permission`, `query`, `read-after-write`, and `sdk`. Errors carry model/field/document locations where available, without rejected values. SDK messages are not copied into FQL errors. SDK permission rules can mask document absence; not every missing document can produce `not-found`.

## Writes, costs, and offline behavior

- Auto-ID creates perform one SDK write without a preliminary read. Their completion waits for server acknowledgement; SDK-generated IDs provide probabilistic collision avoidance.
- Explicit-ID creates use an SDK transaction with an existence read, preventing overwrite. They incur a read, require permission to read that document, and require network access. This is an intentional create-only operation; no transaction callback performs application side effects.
- Partial updates perform one SDK update without a hidden read. Client validation covers supplied changes; rules validate the resulting document and immutable values. Defaults are not reapplied. Update builders offer `setField(null)` for nullable fields and `removeField()` for optional fields; omission means unchanged.
- Delete delegates to the SDK. Auth and operation errors surface through the same error boundary.
- Offline cache and optimistic writes remain SDK behavior. Passing local validation is not server confirmation. Pending writes can later be rejected after reconnecting. Snapshots include metadata-only events so confirmation is observable.
- The SDK owns transaction retries and synchronization. No custom retry loop, caching layer, or synchronization engine is added.

## Queries

v0.1 supports conjunctions of equality and ordered range predicates, up to 10 predicates and 8 order fields, with at most one range field. The first explicit order must match that range field. Boolean/null ranges and cross-model predicates are rejected. Limits are 1–1000. SDK errors still surface when a valid query needs a deployed index or violates authorization conditions.

Pagination uses SDK document-snapshot cursors tied to the query's collection, predicates, ordering, and limit. Reusing a cursor with a different query is rejected. Cursors are in-memory objects, not portable serialized bookmarks. Queries may return fewer documents when ordering excludes documents without an ordered field, according to official SDK behavior.

## Verification limits

Emulator tests prove local rule behavior; they do not deploy production rules or indexes. Firestore's emulator does not provide proof that production composite indexes are built and ready. Flutter web integration establishes web SDK behavior only; Android/iOS acceptance requires separate device runs. No claim of deployed security, device compatibility, or migration safety follows from generating files.

References: [field validation](https://firebase.google.com/docs/firestore/security/rules-fields), [query authorization](https://firebase.google.com/docs/firestore/security/rules-query), [listener metadata](https://firebase.google.com/docs/firestore/query-data/listen), [transactions](https://firebase.google.com/docs/firestore/manage-data/transactions), [Rules strings](https://firebase.google.com/docs/reference/rules/rules.String), and [Rules timestamps](https://firebase.google.com/docs/reference/rules/rules.Timestamp).

The [extension contracts](extensions.md) describe explicit-ID batch restrictions, transaction retries, managed-create read costs, list/rule budgets, read-only auditing, and lockfile baseline upgrades.

## TypeScript target

The same generated Rules and schema constraints apply to TypeScript. The runtime
uses Firebase JavaScript SDK promises/callback listeners rather than Dart futures
and streams. Optional values use `undefined`; explicit null remains `null`.
See [TypeScript contracts](typescript.md) for mappings, unsubscribe handling,
React Native persistence limits, and SDK compatibility.

## Declarative policy boundaries

[Authorization contracts](authorization.md) specify claim issuance, lookup costs, get/list compatibility, strict input guards under negation/disjunction, query requirements, and atomic state checks. Server revocation does not erase cached data or guarantee time-driven listener eviction. Policies do not attest physical-device identity, authorize privileged backend writes, or replace resulting-document validation.

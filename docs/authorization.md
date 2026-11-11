# Declarative authorization

This document defines the implemented declarative-policy extension. Local test evidence and remaining deployment/device boundaries are recorded in [verification notes](verification.md). The earlier behavioral pseudocode is not FQL syntax.

## Language and types

All `.fql` and `.fQL` files in the configured schema tree compile together. Declare at most one `claims` block across the project, and reusable policies at top level:

```fql
schema "0.1"
claims {
  deviceId string
  generation int
}
policy activeDeviceFor(uid string, device string, generation int) {
  authenticated
  and auth.uid == uid
  and auth.claims.deviceId == device
  and auth.claims.generation == generation
  and get(Account, uid).deviceId == device
  and get(Account, uid).generation == generation
  and get(Account, uid).active == true
}
```

Claims and named-policy parameters support `string`, `bool`, `int`, and `float`; parameters also support `timestamp` and `date`. Integers and finite numbers stay within the shared safe numeric range. Claims are declarations of token fields, not client-supplied data or a token issuance mechanism. No claims are stored in model documents or generated models.

Expressions support parentheses, `not`, comparisons (`==`, `!=`, `<`, `<=`, `>`, `>=`), `and`, then `or`, in descending precedence. Comparisons require matching scalar types (int/float may compare); ordering is limited to numbers, dates, strings, and timestamps. Literals are strings, safe finite decimal numbers, and booleans; null comparisons, arithmetic, arbitrary functions, dynamic property names, and unbounded expressions are rejected.

`authenticated` and `deny` are booleans; `owner(field)` retains its existing required immutable string semantics and `owner(id)` compares the document ID. Other values are `auth.uid`, declared `auth.claims.name`, `now` (server `request.time`), `id`, `existing.field`, and `incoming.field`. `existing` is available for get/list/read/update/delete; `incoming` for create/update and represents the complete resulting document, including partial updates. Policy field access supports required nonnullable scalar root fields; use explicit schema constraints rather than relying on absent or nullable authorization values. Named policies use explicit scalar parameters and cannot capture `id`, `owner`, `existing`, or `incoming` implicitly.

`get(Model, documentId).field` reads the existing related document; `exists(Model, documentId)` tests its existence. `getAfter(Model, documentId).field` reads its post-atomic-write state and is restricted to create/update/delete policies. Model references resolve across files. IDs must be nonempty string path segments, excluding `/`, `.` and `..`, and at most 1500 UTF-8 bytes. There is no raw path interpolation or application-side lookup.

Named policies return booleans, accept at most eight parameters, and may call other policies without recursion. Missing/invalid auth claims and missing/invalid fields accessed by a policy deny the entire policy, including when an access is beneath `not` or in an unused `or` branch. This deliberately strict behavior prevents negation from turning invalid security inputs into a grant. `not exists(...)` is the supported way to authorize absence of a related record. A lookup validates the scalar types (including safe numeric ranges and calendar date validity) the policy actually uses, not the complete related document or its other field constraints. Timestamp guards use ordered comparisons against server time: this rejects invalid types while remaining provable for expiration range queries, unlike an explicit timestamp type test in the tested emulator. Rules lookups do not require granting clients direct read access to the related collection.

## Access operations and compatibility

Existing `read`, `create`, `update`, `delete` policies remain valid. `get` and `list` may replace `read`; mixing `read` with either is rejected, since overlapping Firestore grants are additive. Unspecified operations deny access. Authorization always remains conjoined with structural validation, managed timestamps, and immutability on writes. Named policies are expanded into validated typed expressions; unsupported constructs fail before output replacement. Generated clients do not perform authorization locally or make additional network calls.

Language/configuration version remains `0.1`; new declarations require a compiler supporting this extension. Older compilers reject them instead of approximating them. The lockfile retains format 2 and includes claims and named policy definitions in the canonical diff baseline. Changes to either require authorization review. Generated runtime serialization is unchanged.

## Flamingo scenario

An envelope can use:

```fql
access {
  get = activeDeviceFor(existing.recipientId, existing.deviceId, existing.generation)
        and existing.expiresAt > now
  list = activeDeviceFor(existing.recipientId, existing.deviceId, existing.generation)
         and existing.expiresAt > now
  create = authenticated and incoming.senderId == auth.uid
}
```

Declare recipient/device/generation/routing fields immutable where updates must preserve them. A mailbox query needs equality predicates for recipient UID, device ID, and generation, and an expiration lower bound strictly later than server request time. A client-selected future cutoff can fail if the request arrives after that cutoff; refresh it and handle permission failures. Declare the corresponding composite index explicitly (equality fields followed by expiration). FQL does not automatically infer predicates/indexes from arbitrary policy expressions, filter unauthorized documents locally, or guarantee every expression is queryable; emulator-test the actual query shape.

`getAfter` alone does not require a change. To require a related record to change together, compare its before/after values or require a new value tied to the current write. Rules validate each write and the complete atomic result. A batch with a missing companion change must fail; ordinary existing matching state must not accidentally satisfy the intended invariant.

## Bounds and enforcement

Policies are limited to 32 expression nesting levels, 256 expanded nodes and a conservative combined expression budget of 900 (including document validation, weighted expansion nodes, and calendar checks). Document lookups are conservatively counted without assuming cache reuse; at most ten emitted access calls per operation. Firestore also imposes an aggregate limit of twenty across atomic multi-operation requests, so a larger batch can be denied even when each operation is valid. Lookup costs can apply even to denied requests. See [rule conditions and access limits](https://firebase.google.com/docs/firestore/security/rules-conditions).

Trusted backend code must issue credentials bound to the intended device/session; account-wide custom claims alone do not establish physical-device identity. Tokens can be copied; this feature does not provide attestation. Account authorization checks enable server-side revocation independently of old token claims, but cannot erase already delivered/cached data or promise that listeners expire at a wall-clock instant without a new server evaluation. Offline reads retain official SDK cache behavior; local optimistic writes are not authorization confirmation. Privileged server SDKs bypass rules and need separate validation and authorization.

Rules artifacts must be reviewed and explicitly deployed. Nested collections, encryption, key claiming, notification dispatch, and reliable quotas remain separate concerns. See [query authorization](https://firebase.google.com/docs/firestore/security/rules-query) for why queries must satisfy rules over their possible result set.

The complete [policy/model fixtures](../tests/emulator/fixtures/policies/) are compiled during [emulator tests](../tests/emulator/policies.test.mjs), including mixed filename extensions, a mailbox composite index, and coordinated batch/transaction checks.

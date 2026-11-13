# Write schemas

[Handbook](README.md) · [Quickstart](../README.md#quickstart) · [Next: data operations](data-operations.md)

FQL schemas describe documents, validation, authorization, and explicit composite indexes. You author `.fql` files; Dart and TypeScript are generated outputs. This guide builds on the Message and User models from `fql init`.

## Define a model

Use one file per model where practical, for example `fQL/schemas/messages.fql`. Start each file with `schema "0.1"`, then declare a model. The [quickstart Message schema](../README.md#2-define-a-schema) is a complete example.

| Part | Purpose |
| --- | --- |
| `model Message` | Names the generated model `Message`, repository `db.messages`, and predicates `MessageFields` |
| `collection = "messages"` | Selects the top-level Firestore collection |
| `fields { ... }` | Defines stored fields and their constraints |
| `access { ... }` | Defines which operations are authorized; omissions deny access |
| `indexes { ... }` | Declares supported composite indexes to generate |

Model names use UpperCamelCase; field names use lowerCamelCase. Model names and collection names must be unique across the project. Do not declare an `id` field: document IDs are metadata available as `model.id`.

Every `.fql` or `.fQL` file under the configured schemas directory is discovered recursively. Named types resolve across files, regardless of file ordering. You can keep shared types in `fQL/schemas/types/`; do not add import statements. Comments use `//` or `/* ... */`; declarations do not use semicolons.

## Choose field types

Every field chooses either `required()` or `optional()`. Add `nullable()` separately when explicit null is a valid stored value.

| FQL type | Dart | TypeScript | Stored representation |
| --- | --- | --- | --- |
| `string` | `String` | `string` | String |
| `bool` | `bool` | `boolean` | Boolean |
| `int` | `int` | `number` | Integer within ±9,007,199,254,740,991 |
| `float` | `double` | `number` | Finite number in the same safe range |
| `date` | `FqlDate` | `FqlDate` | Calendar-date string, `YYYY-MM-DD` |
| `timestamp` | SDK `Timestamp` | SDK `Timestamp` | Firestore timestamp, microsecond precision |

Dates support real Gregorian dates in years 0001–9999. Use `FqlDate(2000, 2, 29)` in Dart or `new FqlDate(2000, 2, 29)` in TypeScript; a `date` is not a timestamp or a timezone-dependent JavaScript `Date`. Timestamps support years 0001–9999 at microsecond precision. See the [language reference](language.md#field-semantics) for exact bounds and codecs.

### Distinguish missing values from null

These are field declarations inside a `fields` block:

```text
name string.required()
nickname string.optional()
bio string.optional().nullable()
```

| Declaration | Key may be absent? | Value may be null? |
| --- | --- | --- |
| `required()` | No | No |
| `required().nullable()` | No | Yes |
| `optional()` | Yes | No |
| `optional().nullable()` | Yes | Yes |

An optional Dart getter is nullable; check `user.hasBio` to distinguish a missing key from a present null. TypeScript optional getters include `undefined`, while nullable getters include `null`; `hasBio` or `hasField('bio')` also checks presence. Do not pass `undefined` as a setter value. [Partial update examples](data-operations.md#leave-unchanged-set-null-or-remove) show how to set, clear, and remove fields deliberately.

### Add constraints and defaults

Examples inside `fields`:

```text
body string.required().minLength(1).maxLength(4000)
priority int.required().min(0).max(10).default(0)
status string.required().oneOf("active", "paused").default("active")
ownerId string.required().immutable()
```

String bounds count UTF-8 bytes, not visible characters. Numeric bounds are inclusive. `oneOf(...)` accepts 1–32 scalar values of the field's type. Defaults apply only when a create input omits the field; explicit null must satisfy nullability. Defaults never fill missing values during reads or updates, and Rules do not insert them for raw SDK writes.

An immutable field has no generated update setter. Generated Rules also prevent changes to its value or presence. Unknown stored keys, missing required fields, and malformed values fail decoding rather than being ignored or repaired.

## Add enums, objects, and lists

Save this as `fQL/schemas/parcels.fql` alongside the scaffolded models, or split the named types into other schema files:

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
    ownerId string.required().immutable()
    delivery Delivery.required().default("draft")
    address Address.required()
    tags list<string>.required().maxItems(8)
    createdAt timestamp.required().serverCreated()
    updatedAt timestamp.required().serverUpdated()
  }
  access {
    read = owner(ownerId)
    create = owner(ownerId)
    update = owner(ownerId)
    delete = owner(ownerId)
  }
}
```

Run `fql format`, `fql validate`, and `fql generate` before using the generated types.

**Dart**

```dart
final address = Address.create()
    .setStreet('1 Example Road').setCity('Example City').build();
await db.parcels.create().setOwnerId(uid)
    .setAddress(address).setDelivery(Delivery.draft).setTags(['demo']).save();
```

**TypeScript**

```ts
const address = Address.create()
  .setStreet('1 Example Road').setCity('Example City').build();
await db.parcels.create().setOwnerId(uid)
  .setAddress(address).setDelivery(Delivery.draft).setTags(['demo']).save();
```

Import `Address` and `Delivery` from the generated client, in addition to `FqlClient`. These snippets run inside an async function with `db` and the authenticated `uid` in scope.

Enums store member names as strings. Objects are embedded maps, not separate linked documents. Lists require a `maxItems` bound from 0 to 16 and may also have `minItems`; elements cannot be null. Nested objects/lists are replaced through the root field setter, rather than edited through dotted paths or array transforms. Cycles, excessive nesting, and schemas exceeding validation budgets are rejected; see [extension limits](extensions.md#named-enums-objects-and-bounded-lists).

## Use server-generated timestamps

The Parcel example uses `serverCreated()` and `serverUpdated()`. Both require nonnullable, required root timestamp fields. They cannot have defaults or `oneOf` constraints, and they have no generated setters.

`serverCreated()` writes the server timestamp on create and preserves it afterward. `serverUpdated()` writes it on every create/update. Generated getters return `FqlServerTimestamp`, exposing `isPending` and `value`: `value` is null while an optimistic local value is unresolved.

A direct `.save()` create with managed timestamps performs a server read after acknowledgement so it can return resolved fields. This adds a read and requires read permission. A `read-after-write` error means the write committed but the resolution read failed; do not blindly repeat the create. Prepared batch/transaction operations return IDs without that resolution read. See [managed timestamp contracts](extensions.md#managed-timestamps).

## Declare who can access documents

The Message example uses `owner(ownerId)` for all operations. Its owner field must be a required, immutable, nonnullable string. On create, it must equal the signed-in user's UID; afterward, access checks the stored owner. `owner(id)` instead uses the document ID, as in the scaffolded User model.

Other simple policies are `deny` and `authenticated`. Grant only the operations your application needs. Omitted operations deny access. You can use separate `get` and `list` grants instead of `read`, but cannot combine `read` with either in one model.

For more complex access, use the [authorization reference](authorization.md): boolean expressions, typed auth claims, reusable policies, related-document lookups, and atomic post-state checks. Claims must be issued by a trusted backend; declaring them does not issue credentials.

Policies become Security Rules. Regeneration alone does not activate them, and generated client validation does not authorize requests. Queries must meet the policy over their possible result set. For example, an owner-scoped Message query needs `MessageFields.ownerId.eq(uid)`; Rules are not result filters. See Firebase's [query authorization guidance](https://firebase.google.com/docs/firestore/security/rules-query).

## Declare indexes for your queries

The quickstart model declares:

```text
indexes {
  index(ownerId.asc, createdAt.desc)
}
```

This generates an entry for the query that filters on owner ID and sorts by creation time. Composite indexes contain 2–8 distinct fields. Firestore handles normal single-field indexes; FQL does not infer every index from arbitrary application queries. Review and [deploy generated indexes](workflow.md#deploy-rules-and-indexes), then wait for them to become ready before depending on the hosted query.

## Validate your changes

```sh
fql format
fql validate
fql diff
fql generate
```

Validation diagnoses unsupported definitions and unresolved/duplicate names with source locations. Review `diff` before generating, because successful generation advances its baseline. If your app already has data, follow the [schema evolution workflow](workflow.md#change-a-schema-with-existing-data); generation does not migrate documents.

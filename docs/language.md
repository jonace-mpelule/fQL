# FQL language 0.1

[User handbook](README.md) · [Quickstart](../README.md#quickstart)

Every `.fql` or `.fQL` file starts with `schema "0.1"`. Named `enum` and `object` definitions, bounded `list<Type>` fields, and managed timestamp modifiers are specified in [extensions](extensions.md). Definitions across recursively discovered files form one schema. Declaration order does not affect name resolution. Comments use `//` or `/* ... */`. Strings use double quotes and Go-style escapes. Numeric literals use decimal notation; integer defaults use integral decimal literals. Semicolons and executable expressions are not supported.

```fql
schema "0.1"
model Message {
  collection = "messages"
  fields {
    ownerId string.required().immutable()
    body string.required().minLength(1).maxLength(4000)
    createdAt timestamp.required().immutable()
    priority int.required().min(0).max(10).default(0)
  }
  access {
    read = owner(ownerId)
    create = owner(ownerId)
    update = owner(ownerId)
    delete = owner(ownerId)
  }
  indexes {
    index(ownerId.asc, createdAt.desc)
  }
}
```

Models use nonreserved UpperCamelCase names, fields use nonreserved lowerCamelCase names, and collections use top-level identifiers beginning with a lowercase letter. Reserved Dart/runtime names (plus target-specific TypeScript runtime conflicts) and generated-name collisions are rejected. The `has` field prefix is reserved for presence getters. Identifiers are limited to 64 ASCII characters. Model and collection names must be unique. Document IDs are metadata rather than stored fields.

## Field semantics

| Type | Stored value | Dart value | Limits |
| --- | --- | --- | --- |
| `string` | Firestore string | `String` | At most 1,048,487 UTF-8 bytes |
| `bool` | Firestore boolean | `bool` | `true` or `false` |
| `int` | Firestore integer | `int` | ±9,007,199,254,740,991 for portable precision |
| `float` | Firestore number | `double` | Finite, same safe numeric range; stored integers are accepted as numbers |
| `date` | `YYYY-MM-DD` string | `FqlDate` | Real Gregorian calendar date, years 0001–9999 |
| `timestamp` | Firestore timestamp | SDK `Timestamp` | Years 0001–9999; microsecond precision |

Choose exactly one of `required()` and `optional()`. Required means the key must exist. Optional permits a missing key. `nullable()` independently permits explicit null. Optional generated getters return nullable values; `hasBio` or `hasField('bio')` distinguishes absence from explicit null.

- `minLength(n)` / `maxLength(n)`: nonnegative UTF-8 byte bounds on strings, not user-perceived characters.
- `min(n)` / `max(n)`: finite numeric bounds; integer bounds use decimal integer literals.
- `oneOf(literals...)`: 1–32 type-checked allowed scalar values. Nullability remains an independent permission.
- `default(literal)`: applies to missing create inputs only. It does not apply during reads or updates, and Security Rules do not insert defaults. Date defaults use date strings; timestamp defaults use RFC3339 strings with microsecond precision.
- `immutable()`: generated updates cannot set/remove the field; rules reject changes to its stored value or presence.

No implicit string-to-date conversion occurs in application inputs: use `FqlDate`. Decoding stored calendar-date strings is an explicit codec operation. Numeric integer-to-double conversion for `float` is part of the declared storage contract, not an unchecked coercion. Unknown stored fields, missing required fields, and malformed values produce decoding errors. Existing documents are not migrated or repaired.

## Authorization and indexes

Operations are `read`, `create`, `update`, and `delete`. Omitted operations deny access. Policies are `deny`, `authenticated`, and `owner(id)` or `owner(fieldName)`. Owner fields must be required, nonnullable, immutable strings. The authenticated UID must match the document ID or protected field. Create checks incoming ownership; read/update/delete check existing ownership. Rules separately validate the entire resulting document on create/update.

Owner-field collection queries must include an equality predicate for the authenticated UID. Document-ID ownership supports direct access; v0.1 does not generate document-ID query predicates. Rules authorize queries based on their possible results; they do not filter unauthorized documents out of a collection. Policy checking remains server-side, including when a local optimistic write appeared successful.

Composite indexes declare 2–8 distinct fields and `asc`/`desc` ordering. Single-field indexes remain Firestore's normal configuration. FQL does not discover indexes for arbitrary runtime queries or deploy them automatically.

## Project configuration

`fQL/fql.config` contains these required keys, each once:

```text
version = "0.1"
target = "dart"
schemas = "schemas"
output = "../lib/data/fql"
firebase = "../firebase/generated"
```

Set `target = "typescript"` for TypeScript generation (`fql.ts`); initialization defaults to `../src/data/fql` for this target. Only one target is selected per project configuration. See [TypeScript mappings and runtime behavior](typescript.md).

Paths are relative to this configuration, use forward slashes, and must stay inside the project. Source, configuration, and target output paths must not overlap. Managed paths cannot contain symlinks. The source tree permits up to 128 schema files, 1 MiB per file, 16 MiB combined; the schema permits 100 models and 32 fields per model. These are initial compiler safety budgets, not claims about Firestore limits.

`fQL/fql.lock` is generated JSON compatibility metadata containing format version 2, generator compatibility version, an effective-schema comparison baseline, a schema/configuration fingerprint, and artifact hashes. Fingerprints and text artifact hashes normalize CRLF to LF so checkouts remain portable across operating systems. It is not a schema-authoring format. Commit it with generated outputs. Modified owned files are protected; restore the generated files before regenerating. If a process is killed, inspect the artifacts and lockfile before removing an abandoned `fQL/.generate.lock` directory. Ordinary generation errors roll back replacements; a multi-file update is not crash-atomic.

Declarative authorization expressions, claim declarations, named policies, related-document lookups, and `get`/`list` separation are specified in [authorization](authorization.md). Existing simple policies remain compatible; `read` cannot be combined with `get` or `list`.

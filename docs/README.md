# FQL user handbook

[Repository home](../README.md) · [Quickstart](../README.md#quickstart) · [Contributing](../CONTRIBUTING.md)

Start with the [quickstart](../README.md#quickstart) to install FQL, define a Message schema, generate a client, and create your first document. This handbook is the next stop when you want to build out your application. All documentation lives in the repository and can be browsed on GitHub or alongside your checkout.

## Learn by task

| Task | Guide |
| --- | --- |
| Define a collection and its fields | [Write schemas](schemas.md#define-a-model) |
| Choose types, optional/null values, defaults, and constraints | [Field design](schemas.md#choose-field-types) |
| Add enums, embedded objects, or lists | [Structured data](schemas.md#add-enums-objects-and-lists) |
| Use server-created or server-updated timestamps | [Managed timestamps](schemas.md#use-server-generated-timestamps) |
| Restrict access to the document owner | [Permissions](schemas.md#declare-who-can-access-documents) |
| Use auth claims, related documents, and reusable policies | [Authorization reference](authorization.md) |
| Create, read, update, or delete | [Data operations](data-operations.md#create-read-update-and-delete) |
| Clear a nullable field or remove an optional field | [Partial updates](data-operations.md#leave-unchanged-set-null-or-remove) |
| Query, sort, or paginate | [Queries and cursors](data-operations.md#query-and-paginate) |
| Listen for changes and release resources | [Subscriptions](data-operations.md#listen-for-changes) |
| Commit a batch or run a transaction | [Atomic writes](data-operations.md#use-batches-and-transactions) |
| Handle validation, permissions, and decoding failures | [Errors](data-operations.md#handle-errors) |
| Configure paths, switch targets, or use the CLI | [Project workflow](workflow.md) |
| Deploy rules and indexes | [Deployment](workflow.md#deploy-rules-and-indexes) |
| Evolve a schema or inspect existing documents | [Schema changes and audits](workflow.md#change-a-schema-with-existing-data) |
| Fix common setup and runtime problems | [Troubleshooting](workflow.md#troubleshooting) |
| Run the full example locally | [Flutter emulator example](../examples/flutter/README.md) |

## Reference material

The guides explain how to use FQL. These documents specify the precise contracts and limits:

| Reference | Covers |
| --- | --- |
| [Language 0.1](language.md) | Grammar, scalar types, field semantics, configuration and lockfile formats, compiler budgets |
| [Extensions](extensions.md) | Enums, objects, lists, managed timestamps, atomic writes, format, diff, and audit |
| [Authorization](authorization.md) | Policy expressions, get/list grants, claims, lookups, access budgets, and enforcement limits |
| [TypeScript target](typescript.md) | JavaScript SDK integration, type mappings, React Native/Expo compatibility, runtime behavior |
| [Enforcement and runtime](enforcement.md) | Client versus Rules guarantees, errors, reads, offline behavior, query and operation limits |
| [Verification notes](verification.md) | What has been tested locally and what still needs device or deployed verification |

For compiler architecture, repository development, and release packaging, see [architecture](architecture.md), the [feature checklist](v0.1-features.md), and [contributing](../CONTRIBUTING.md).

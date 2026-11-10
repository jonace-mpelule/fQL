import 'package:cloud_firestore/cloud_firestore.dart';
import 'package:fake_cloud_firestore/fake_cloud_firestore.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:fql_example/data/fql/fql.dart';

void main() {
  Map<String, dynamic> stored() => {
    'ownerId': 'alice',
    'info': {'title': 'Room'},
    'state': 'open',
    'tags': ['one'],
    'history': [
      {'title': 'Earlier'},
    ],
    'count': 0,
    'createdAt': Timestamp(0, 0),
    'updatedAt': Timestamp(0, 0),
  };
  test(
    'enums, objects and typed lists decode strictly and retain immutable values',
    () {
      final value = Conversation.decode('c', stored());
      expect(value.state, ConversationState.open);
      expect(value.info.title, 'Room');
      expect(value.history!.single.title, 'Earlier');
      expect(() => value.tags!.add('mutate'), throwsUnsupportedError);
      expect(value.toMap()['state'], 'open');
      final object = ConversationInfo.create()
          .setTitle('Typed')
          .setDescription(null)
          .build();
      expect(object.hasDescription, true);
      expect(
        () => ConversationInfo.create().setTitle('').build(),
        throwsA(isA<FqlError>()),
      );
      for (final extra in [
        <String, dynamic>{'state': 'unknown'},
        {
          'info': {'title': 'Room', 'extra': true},
        },
        {
          'tags': [1],
        },
        {'tags': List.filled(9, 'too many')},
        {
          'history': [
            {'title': 1},
          ],
        },
      ]) {
        expect(
          () => Conversation.decode('c', {...stored(), ...extra}),
          throwsA(isA<FqlError>()),
        );
      }
    },
  );
  test(
    'pending server timestamps are explicit and cannot masquerade as confirmed data',
    () {
      final data = {...stored(), 'createdAt': null, 'updatedAt': null};
      expect(() => Conversation.decode('c', data), throwsA(isA<FqlError>()));
      final pending = Conversation.decode('c', data, hasPendingWrites: true);
      expect(pending.createdAt.isPending, true);
      expect(pending.createdAt.value, null);
      expect(() => pending.toMap(), throwsA(isA<FqlError>()));
      expect(Conversation.decode('c', stored()).createdAt.isPending, false);
    },
  );
  test('prepared batches validate, capture IDs and cannot be reused', () async {
    final db = FqlClient(FakeFirebaseFirestore());
    final first = db.users.create().setName('First').operation();
    final second = db.users.create().setName('Second').operation();
    final batch = db.batch();
    batch.add(first);
    batch.add(second);
    await batch.commit();
    expect((await db.users.get(first.id)).data.name, 'First');
    await expectLater(batch.commit(), throwsA(isA<FqlError>()));
    expect(() => db.batch().add(first), throwsA(isA<FqlError>()));
    expect(
      () => db.batch().add(
        db.users.create(id: 'fixed').setName('Fixed').operation(),
      ),
      throwsA(isA<FqlError>()),
    );
    expect(() => db.users.create().operation(), throwsA(isA<FqlError>()));
  });
  test(
    'typed transaction reads, updates and create-only checks share one atomic unit',
    () async {
      final db = FqlClient(FakeFirebaseFirestore());
      await db.users.create(id: 'alice').setName('Before').save();
      final result = await db.transaction((tx) async {
        final before = await tx.get(db.users, 'alice');
        tx.add(db.users.update('alice').setName('After').operation());
        tx.add(db.users.create(id: 'bob').setName('Bob').operation());
        return before.data.name;
      });
      expect(result, 'Before');
      expect((await db.users.get('alice')).data.name, 'After');
      await expectLater(
        db.transaction((tx) async {
          tx.add(db.users.create(id: 'bob').setName('Duplicate').operation());
        }),
        throwsA(
          isA<FqlError>().having((e) => e.code, 'code', 'already-exists'),
        ),
      );
    },
  );
  test(
    'post-commit timestamp resolution errors identify an already committed write',
    () async {
      final sdk = FakeFirebaseFirestore(
        securityRules:
            "rules_version = '2'; service cloud.firestore { match /databases/{database}/documents { match /{document=**} { allow write: if true; allow read: if false; } } }",
      );
      final db = FqlClient(sdk);
      await expectLater(
        db.conversations
            .create()
            .setOwnerId('alice')
            .setInfo(ConversationInfo.create().setTitle('Committed').build())
            .save(),
        throwsA(
          isA<FqlError>()
              .having((e) => e.code, 'code', 'read-after-write')
              .having((e) => e.documentId, 'documentId', isNotNull),
        ),
      );
    },
  );
}

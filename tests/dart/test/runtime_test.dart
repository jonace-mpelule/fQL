import 'dart:async';
import 'package:cloud_firestore/cloud_firestore.dart';
import 'package:fake_cloud_firestore/fake_cloud_firestore.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:fql_example/data/fql/fql.dart';

Matcher code(String expected) =>
    isA<FqlError>().having((e) => e.code, 'code', expected);
void main() {
  test('calendar dates are exact and leap-aware', () {
    expect(FqlDate(2000, 2, 29).value, '2000-02-29');
    for (final value in [
      '1900-02-29',
      '2025-02-29',
      '0000-01-01',
      '2026-04-31',
      '2026-1-01',
    ]) {
      expect(() => FqlDate.parse(value), throwsA(code('validation')));
    }
  });
  test('decoding preserves absent, null, and zero values without defaults', () {
    final user = User.decode('alice', {
      'name': 'A',
      'active': false,
      'status': 'active',
    });
    expect(user.active, false);
    expect(user.hasBio, false);
    expect(user.bio, null);
    final withNull = User.decode('alice', {...user.toMap(), 'bio': null});
    expect(withNull.hasBio, true);
    expect(withNull.bio, null);
    expect(
      () => User.decode('alice', {'name': 'A'}),
      throwsA(code('decoding')),
    );
    expect(
      () => User.decode('alice', {...user.toMap(), 'extra': 1}),
      throwsA(code('decoding')),
    );
    expect(
      () =>
          User.decode('alice', {...user.toMap(), 'dateOfBirth': '2025-02-29'}),
      throwsA(code('decoding')),
    );
    expect(
      () => User.decode('alice', {...user.toMap(), 'status': 'other'}),
      throwsA(code('decoding')),
    );
    expect(
      () => User.decode('alice', {...user.toMap(), 'rating': double.nan}),
      throwsA(code('decoding')),
    );
  });
  test('timestamp precision and safe integers are validated', () {
    final data = {
      'ownerId': 'a',
      'body': 'hello',
      'createdAt': Timestamp(0, 1000),
      'priority': 0,
    };
    expect(Message.decode('m', data).priority, 0);
    expect(
      () => Message.decode('m', {...data, 'createdAt': Timestamp(0, 1)}),
      throwsA(code('decoding')),
    );
    expect(
      () => Message.decode('m', {...data, 'priority': 9007199254740992}),
      throwsA(code('decoding')),
    );
  });
  test(
    'CRUD uses defaults, partial changes, and duplicate protection',
    () async {
      final sdk = FakeFirebaseFirestore();
      final db = FqlClient(sdk);
      final user = await db.users
          .create(id: 'alice')
          .setName('Alice')
          .setBio(null)
          .save();
      expect(user.active, true);
      expect(user.status, 'active');
      expect(user.hasBio, true);
      await expectLater(
        db.users.create(id: 'alice').setName('Overwrite').save(),
        throwsA(code('already-exists')),
      );
      await db.users.update('alice').setName('Updated').removeBio().save();
      final updated = (await db.users.get('alice')).data;
      expect(updated.name, 'Updated');
      expect(updated.hasBio, false);
      await expectLater(db.users.create().save(), throwsA(code('validation')));
      await expectLater(
        db.users.update('alice').save(),
        throwsA(code('validation')),
      );
      await expectLater(
        db.users.create().setName('😃' * 26).save(),
        throwsA(code('validation')),
      );
      await expectLater(
        db.users.create().setName('A').setRating(6).save(),
        throwsA(code('validation')),
      );
      await db.users.delete('alice');
      await expectLater(db.users.get('alice'), throwsA(code('not-found')));
    },
  );
  test('typed queries paginate and reject invalid combinations', () async {
    final sdk = FakeFirebaseFirestore();
    final db = FqlClient(sdk);
    for (final name in ['A', 'B', 'C']) {
      await db.users.create().setName(name).save();
    }
    final query = db.users.query().orderBy(UserFields.name).limit(2);
    final first = await query.all();
    expect(first.data.map((u) => u.name), ['A', 'B']);
    final second = await query.startAfter(first.cursor!).all();
    expect(second.data.single.name, 'C');
    await expectLater(
      db.users.query().where(MessageFields.body.eq('x')).all(),
      throwsA(code('query')),
    );
    await expectLater(
      db.users.query().where(UserFields.active.gt(true)).all(),
      throwsA(code('query')),
    );
    await expectLater(
      db.users
          .query()
          .where(UserFields.name.gt('A'))
          .orderBy(UserFields.active)
          .all(),
      throwsA(code('query')),
    );
    await expectLater(
      db.users.query().limit(1).startAfter(first.cursor!).all(),
      throwsA(code('query')),
    );
    expect(() => query.limit(0), throwsA(code('query')));
    await expectLater(
      db.users.create().setName('\uD800').save(),
      throwsA(code('validation')),
    );
  });
  test(
    'malformed query results fail instead of silently disappearing',
    () async {
      final sdk = FakeFirebaseFirestore();
      final db = FqlClient(sdk);
      await sdk.collection('users').doc('broken').set({
        'name': 42,
        'active': true,
        'status': 'active',
      });
      await expectLater(db.users.query().all(), throwsA(code('decoding')));
      await expectLater(db.users.get('broken'), throwsA(code('decoding')));
    },
  );
  test('listeners expose decoding failures and recover after repair', () async {
    final sdk = FakeFirebaseFirestore();
    final db = FqlClient(sdk);
    await sdk.collection('users').doc('alice').set({'name': 42});
    final errorSeen = Completer<void>();
    final repaired = Completer<User>();
    final subscription = db.users
        .watch('alice')
        .listen(
          (s) {
            if (!repaired.isCompleted) repaired.complete(s.data);
          },
          onError: (Object error) {
            expect(error, code('decoding'));
            if (!errorSeen.isCompleted) errorSeen.complete();
          },
        );
    await errorSeen.future.timeout(const Duration(seconds: 5));
    await sdk.collection('users').doc('alice').set({
      'name': 'Repaired',
      'active': true,
      'status': 'active',
    });
    expect(
      (await repaired.future.timeout(const Duration(seconds: 5))).name,
      'Repaired',
    );
    await subscription.cancel();
  });
}

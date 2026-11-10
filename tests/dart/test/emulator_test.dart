import 'dart:async';
import 'dart:convert';
import 'package:cloud_firestore/cloud_firestore.dart';
import 'package:firebase_auth/firebase_auth.dart' as auth;
import 'package:firebase_core/firebase_core.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:fql_example/data/fql/fql.dart';
import 'package:http/http.dart' as http;
import 'package:firebase_core_web/firebase_core_web.dart';
import 'package:firebase_auth_web/firebase_auth_web.dart';
import 'package:cloud_firestore_web/cloud_firestore_web.dart';
import 'package:flutter_web_plugins/flutter_web_plugins.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  late FirebaseFirestore sdk;
  late FirebaseApp app;
  var sequence = 0;
  late FqlClient db;
  late String uid;
  setUp(() async {
    // flutter test does not run the application's generated web plugin registrant.
    FirebaseCoreWeb.registerWith(webPluginRegistrar);
    FirebaseAuthWeb.registerWith(webPluginRegistrar);
    FirebaseFirestoreWeb.registerWith(webPluginRegistrar);
    app = await Firebase.initializeApp(
      name: 'fql-test-${sequence++}',
      options: const FirebaseOptions(
        apiKey: 'demo-fql-key',
        appId: '1:123456789:web:fql-tests',
        messagingSenderId: '123456789',
        projectId: 'demo-fql',
      ),
    ).timeout(const Duration(seconds: 30));
    sdk = FirebaseFirestore.instanceFor(app: app);
    sdk.settings = const Settings(webExperimentalForceLongPolling: true);
    sdk.useFirestoreEmulator('127.0.0.1', 8080);
    await auth.FirebaseAuth.instanceFor(
      app: app,
    ).useAuthEmulator('127.0.0.1', 9099);
    uid = (await auth.FirebaseAuth.instanceFor(
      app: app,
    ).signInAnonymously().timeout(const Duration(seconds: 15))).user!.uid;
    db = FqlClient(sdk);
  });
  tearDown(() async {
    await sdk.enableNetwork();
    await sdk.terminate();
    await app.delete();
  });
  test(
    'real SDK CRUD, pagination, timestamps, and metadata',
    () async {
      final user = await db.users
          .create(id: uid)
          .setName('Emulator user')
          .setDateOfBirth(FqlDate(2000, 2, 29))
          .save();
      expect(user.active, true);
      expect(
        (await db.users.get(uid, source: Source.server)).data.dateOfBirth,
        FqlDate(2000, 2, 29),
      );
      await expectLater(
        db.users.create(id: uid).setName('Duplicate').save(),
        throwsA(
          isA<FqlError>().having((e) => e.code, 'code', 'already-exists'),
        ),
      );
      for (var i = 0; i < 3; i++) {
        await db.messages
            .create()
            .setOwnerId(uid)
            .setBody('Message $i')
            .setCreatedAt(
              Timestamp.fromMillisecondsSinceEpoch(1700000000000 + i),
            )
            .save();
      }
      final query = db.messages
          .query()
          .where(MessageFields.ownerId.eq(uid))
          .orderBy(MessageFields.createdAt, descending: true)
          .limit(2);
      final first = await query.all(source: Source.server);
      expect(first.data.length, 2);
      expect(first.hasPendingWrites, false);
      final second = await query
          .startAfter(first.cursor!)
          .all(source: Source.server);
      expect(second.data.length, 1);
      final message = first.data.first;
      final event = await db.messages.watch(message.id).first;
      expect(event.data.id, message.id);
      await db.messages.update(message.id).setBody('Updated').save();
      expect(
        (await db.messages.get(message.id, source: Source.cache)).data.body,
        'Updated',
      );
      await db.messages.delete(message.id);
    },
    timeout: const Timeout(Duration(seconds: 30)),
  );

  test(
    'cached reads validate legacy documents seeded through emulator admin REST',
    () async {
      final response = await http.patch(
        Uri.parse(
          'http://127.0.0.1:8080/v1/projects/demo-fql/databases/(default)/documents/users/$uid',
        ),
        headers: {
          'Authorization': 'Bearer owner',
          'Content-Type': 'application/json',
        },
        body: jsonEncode({
          'fields': {
            'name': {'integerValue': '42'},
          },
        }),
      );
      expect(response.statusCode, 200);
      await expectLater(
        db.users.get(uid, source: Source.server),
        throwsA(isA<FqlError>().having((e) => e.code, 'code', 'decoding')),
      );
      await expectLater(
        db.users.get(uid, source: Source.cache),
        throwsA(isA<FqlError>().having((e) => e.code, 'code', 'decoding')),
      );
      // A valid replacement is allowed through the SDK rules for this owner.
      await sdk.collection('users').doc(uid).set({
        'name': 'Restored',
        'active': true,
        'status': 'active',
      });
    },
  );

  test(
    'offline writes expose pending state and resolve only after reconnection',
    () async {
      final message = await db.messages
          .create()
          .setOwnerId(uid)
          .setBody('Before offline')
          .setCreatedAt(Timestamp.now())
          .save();
      final pending = Completer<void>();
      final confirmed = Completer<void>();
      final subscription = db.messages.watch(message.id).listen((event) {
        if (event.data.body == 'Offline change') {
          if (event.hasPendingWrites && !pending.isCompleted)
            pending.complete();
          if (!event.hasPendingWrites &&
              !event.fromCache &&
              !confirmed.isCompleted)
            confirmed.complete();
        }
      });
      try {
        await sdk.disableNetwork();
        var completed = false;
        final write = db.messages
            .update(message.id)
            .setBody('Offline change')
            .save()
            .then((_) => completed = true);
        await pending.future.timeout(const Duration(seconds: 10));
        expect(completed, false);
        await sdk.enableNetwork();
        await write.timeout(const Duration(seconds: 10));
        await confirmed.future.timeout(const Duration(seconds: 10));
      } finally {
        await sdk.enableNetwork();
        await subscription.cancel();
      }
    },
    timeout: const Timeout(Duration(seconds: 40)),
  );

  test(
    'managed timestamps stay explicit while offline and resolve from the server',
    () async {
      final info = ConversationInfo.create().setTitle('Room').build();
      final value = await db.conversations
          .create()
          .setOwnerId(uid)
          .setInfo(info)
          .setTags(['one'])
          .save();
      expect(value.state, ConversationState.open);
      expect(value.createdAt.isPending, false);
      final pending = Completer<Conversation>();
      final confirmed = Completer<Conversation>();
      final sub = db.conversations.watch(value.id).listen((event) {
        if (event.data.count == 1) {
          if (event.hasPendingWrites && !pending.isCompleted)
            pending.complete(event.data);
          if (!event.hasPendingWrites &&
              !event.fromCache &&
              !confirmed.isCompleted)
            confirmed.complete(event.data);
        }
      });
      try {
        await sdk.disableNetwork();
        final write = db.conversations.update(value.id).setCount(1).save();
        final local = await pending.future.timeout(const Duration(seconds: 10));
        expect(local.updatedAt.isPending, true);
        expect(local.createdAt.isPending, false);
        await sdk.enableNetwork();
        await write;
        final server = await confirmed.future.timeout(
          const Duration(seconds: 10),
        );
        expect(server.updatedAt.isPending, false);
        expect(server.createdAt.value, value.createdAt.value);
        final afterReconnect = db.batch();
        afterReconnect.add(
          db.conversations.update(value.id).setCount(2).operation(),
        );
        await afterReconnect.commit().timeout(const Duration(seconds: 10));
      } finally {
        await sdk.enableNetwork();
        await sub.cancel();
      }
    },
    timeout: const Timeout(Duration(seconds: 40)),
  );

  test(
    'typed batches and transactions commit atomically and preserve validation errors',
    () async {
      await sdk.enableNetwork();
      try {
        await db.users.create(id: uid).setName('Atomic user').save();
      } on FqlError catch (e) {
        if (e.code != 'already-exists') rethrow;
      }
      final conversation = await db.conversations
          .create()
          .setOwnerId(uid)
          .setInfo(ConversationInfo.create().setTitle('Atomic').build())
          .save();
      final message = db.messages
          .create()
          .setOwnerId(uid)
          .setBody('Atomic message')
          .setCreatedAt(Timestamp.now())
          .operation();
      final batch = db.batch();
      batch.add(message);
      batch.add(
        db.conversations.update(conversation.id).setCount(1).operation(),
      );
      await batch.commit().timeout(const Duration(seconds: 10));
      expect((await db.messages.get(message.id)).data.body, 'Atomic message');
      final previous = await db.transaction((tx) async {
        final before = await tx.get(db.conversations, conversation.id);
        tx.add(
          db.conversations
              .update(conversation.id)
              .setCount(before.data.count + 1)
              .operation(),
        );
        return before.data.count;
      });
      expect(previous, 1);
      await expectLater(
        db.transaction((tx) async {
          tx.add(
            db.conversations.update(conversation.id).setCount(3).operation(),
          );
          tx.add(db.users.create(id: uid).setName('Duplicate').operation());
        }),
        throwsA(
          isA<FqlError>().having((e) => e.code, 'code', 'already-exists'),
        ),
      );
      expect((await db.conversations.get(conversation.id)).data.count, 2);
      final good = db.messages
          .create()
          .setOwnerId(uid)
          .setBody('Must roll back')
          .setCreatedAt(Timestamp.now())
          .operation();
      final denied = db.messages
          .create()
          .setOwnerId('other')
          .setBody('Denied')
          .setCreatedAt(Timestamp.now())
          .operation();
      final failing = db.batch();
      failing.add(good);
      failing.add(denied);
      await expectLater(
        failing.commit(),
        throwsA(isA<FqlError>().having((e) => e.code, 'code', 'permission')),
      );
      final result = await http.get(
        Uri.parse(
          'http://127.0.0.1:8080/v1/projects/demo-fql/databases/(default)/documents/messages/${good.id}',
        ),
        headers: {'Authorization': 'Bearer owner'},
      );
      expect(result.statusCode, 404);
    },
    timeout: const Timeout(Duration(seconds: 40)),
  );

  test(
    'an optimistic offline write can later be rejected by authorization',
    () async {
      final message = await db.messages
          .create()
          .setOwnerId(uid)
          .setBody('Owner only')
          .setCreatedAt(Timestamp.now())
          .save();
      await db.messages.get(message.id, source: Source.server);
      await auth.FirebaseAuth.instanceFor(app: app).signOut();
      await auth.FirebaseAuth.instanceFor(app: app).signInAnonymously();
      await sdk.disableNetwork();
      try {
        final rejection = expectLater(
          db.messages.update(message.id).setBody('Forbidden').save(),
          throwsA(isA<FqlError>().having((e) => e.code, 'code', 'permission')),
        );
        await Future<void>.delayed(const Duration(milliseconds: 100));
        await sdk.enableNetwork();
        await rejection.timeout(const Duration(seconds: 15));
      } finally {
        await sdk.enableNetwork();
      }
    },
    timeout: const Timeout(Duration(seconds: 30)),
  );
}

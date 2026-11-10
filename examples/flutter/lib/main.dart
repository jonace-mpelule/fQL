import 'package:cloud_firestore/cloud_firestore.dart';
import 'package:firebase_auth/firebase_auth.dart' as auth;
import 'package:firebase_core/firebase_core.dart';
import 'package:flutter/material.dart';
import 'data/fql/fql.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await Firebase.initializeApp(
    options: const FirebaseOptions(
      apiKey: 'demo-fql-key',
      appId: '1:123456789:web:fql',
      messagingSenderId: '123456789',
      projectId: 'demo-fql',
    ),
  );
  final sdk = FirebaseFirestore.instance;
  sdk.useFirestoreEmulator('127.0.0.1', 8080);
  await auth.FirebaseAuth.instance.useAuthEmulator('127.0.0.1', 9099);
  final session = await auth.FirebaseAuth.instance.signInAnonymously();
  runApp(ExampleApp(FqlClient(sdk), session.user!.uid));
}

class ExampleApp extends StatelessWidget {
  final FqlClient db;
  final String uid;
  const ExampleApp(this.db, this.uid, {super.key});
  @override
  Widget build(BuildContext context) => MaterialApp(
    title: 'FQL example',
    theme: ThemeData(colorSchemeSeed: Colors.deepPurple),
    home: MessagesPage(db, uid),
  );
}

class MessagesPage extends StatefulWidget {
  final FqlClient db;
  final String uid;
  const MessagesPage(this.db, this.uid, {super.key});
  @override
  State<MessagesPage> createState() => _MessagesPageState();
}

class _MessagesPageState extends State<MessagesPage> {
  final _input = TextEditingController();
  late final Stream<FqlSnapshot<List<Message>>> _messages;
  String? _error;
  bool _busy = false;
  @override
  void initState() {
    super.initState();
    _messages = widget.db.messages
        .query()
        .where(MessageFields.ownerId.eq(widget.uid))
        .orderBy(MessageFields.createdAt, descending: true)
        .limit(50)
        .watch();
  }

  Future<void> _perform(Future<void> Function() action) async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await action();
    } catch (e) {
      if (mounted) setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _send() => _perform(() async {
    await widget.db.messages
        .create()
        .setOwnerId(widget.uid)
        .setBody(_input.text)
        .setCreatedAt(Timestamp.now())
        .save();
    _input.clear();
  });
  Future<void> _profile() => _perform(() async {
    try {
      await widget.db.users
          .create(id: widget.uid)
          .setName('Example user')
          .save();
    } on FqlError catch (e) {
      if (e.code != 'already-exists') rethrow;
      await widget.db.users.update(widget.uid).setName('Updated user').save();
    }
    await widget.db.users.get(widget.uid);
  });
  @override
  void dispose() {
    _input.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('FQL messages'),
      actions: [
        TextButton(
          onPressed: _busy ? null : _profile,
          child: const Text('Save profile'),
        ),
      ],
    ),
    body: Column(
      children: [
        if (_error != null)
          Padding(padding: const EdgeInsets.all(16), child: Text(_error!)),
        Expanded(
          child: StreamBuilder<FqlSnapshot<List<Message>>>(
            stream: _messages,
            builder: (context, snapshot) {
              if (snapshot.hasError)
                return Center(child: Text('${snapshot.error}'));
              if (!snapshot.hasData)
                return const Center(child: CircularProgressIndicator());
              final event = snapshot.data!;
              return Column(
                children: [
                  Text(
                    event.hasPendingWrites
                        ? 'Saving changes…'
                        : event.fromCache
                        ? 'Showing cached messages'
                        : 'Up to date',
                  ),
                  Expanded(
                    child: ListView(
                      children: [
                        for (final message in event.data)
                          ListTile(
                            title: Text(message.body),
                            subtitle: Text('Priority ${message.priority}'),
                            onTap: _busy
                                ? null
                                : () => _perform(
                                    () => widget.db.messages
                                        .update(message.id)
                                        .setBody('${message.body} ✓')
                                        .save(),
                                  ),
                            trailing: IconButton(
                              icon: const Icon(Icons.delete_outline),
                              onPressed: _busy
                                  ? null
                                  : () => _perform(
                                      () =>
                                          widget.db.messages.delete(message.id),
                                    ),
                            ),
                          ),
                      ],
                    ),
                  ),
                ],
              );
            },
          ),
        ),
        Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _input,
                  decoration: const InputDecoration(labelText: 'Message'),
                ),
              ),
              IconButton(
                onPressed: _busy ? null : _send,
                icon: const Icon(Icons.send),
              ),
            ],
          ),
        ),
      ],
    ),
  );
}

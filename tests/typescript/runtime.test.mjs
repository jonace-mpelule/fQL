import assert from 'node:assert/strict';
import { test } from 'node:test';
import { initializeApp, deleteApp } from 'firebase/app';
import { getFirestore, terminate } from 'firebase/firestore';
import {
  FqlClient, FqlDate, FqlError, Timestamp, User, Defaults, MessageFields,
  Conversation, ConversationInfo, ConversationState,
} from './build/generated/fql.js';

const fails = (code, field) => e => e instanceof FqlError && e.code === code && (field === undefined || e.field === field);

test('strict codecs preserve absent/null/zero, dates, timestamps and nested values', () => {
  const user = User.decode('u', { name: 'Example', active: false, status: 'active', bio: null, rating: 0 });
  assert.equal(user.hasBio, true); assert.equal(user.bio, null);
  assert.equal(user.hasDateOfBirth, false); assert.equal(user.dateOfBirth, undefined);
  assert.equal(user.rating, 0); assert.equal(user.active, false);
  assert.ok(Object.isFrozen(user));
  assert.throws(() => User.decode('u', { name: 'Example' }), fails('decoding', 'active'));
  assert.throws(() => User.decode('u', { ...user.toMap(), secret: 'sensitive' }), fails('decoding', 'secret'));
  assert.throws(() => User.decode('u', { ...user.toMap(), name: '\ud800' }), fails('decoding', 'name'));
  assert.throws(() => new FqlDate(2025, 2, 29), fails('validation'));
  assert.equal(FqlDate.parse('0001-01-01').value, '0001-01-01');
  const info = ConversationInfo.create().setTitle('Room').setDescription(null).build();
  assert.equal(info.hasDescription, true);
  const wire = { ownerId: 'u', info: info.toMap(), state: 'open', count: 0, tags: ['a'], createdAt: null, updatedAt: null };
  const pending = Conversation.decode('c', wire, true);
  assert.ok(pending.createdAt.isPending); assert.equal(pending.state, ConversationState.open);
  assert.throws(() => pending.toMap(), fails('validation'));
  assert.throws(() => Conversation.decode('c', wire), fails('decoding', 'createdAt'));
  assert.throws(() => Conversation.decode('c', { ...wire, info: { title: 7 } }, true), fails('decoding', 'info.title'));
  assert.throws(() => Conversation.decode('c', { ...wire, count: Number.MAX_SAFE_INTEGER + 1 }, true), fails('decoding', 'count'));
  assert.throws(() => Conversation.decode('c', { ...wire, createdAt: new Timestamp(0, 1) }, true), fails('decoding', 'createdAt'));
});

test('builders apply defaults only on create, capture writes and enforce atomic ownership', async () => {
  const app = initializeApp({ projectId: 'demo-fql' }, 'fql-typescript-local');
  const sdk = getFirestore(app);
  const db = new FqlClient(sdk);
  try {
    const defaults = Defaults.decode('d', db.defaults.create().operation().data);
    assert.equal(defaults.moment.nanoseconds, 123456000);
    assert.equal(defaults.birthday.value, '2000-02-29'); assert.equal(defaults.flag, false);
    assert.equal(db.users.update('u').setBio(null).operation().data.bio, null);
    const remove = db.users.update('u').removeBio().operation();
    assert.equal(typeof remove.data.bio, 'object');
    assert.throws(() => db.users.create().setName(undefined).operation(), fails('validation', 'name'));
    assert.throws(() => db.messages.create().setBody('x').operation(), fails('validation', 'ownerId'));
    const tags = ['one'];
    const op = db.conversations.create().setOwnerId('u').setInfo(ConversationInfo.create().setTitle('Room').build()).setTags(tags).operation();
    tags.push('two'); assert.deepEqual(op.data.tags, ['one']); assert.ok(Object.isFrozen(op.data.tags));
    const batch = db.batch(); batch.add(op);
    assert.throws(() => db.batch().add(op), fails('validation'));
    assert.throws(() => batch.add(op), fails('validation'));
    assert.throws(() => db.batch().add(db.users.create('u').setName('Example').operation()), fails('validation'));
    const errors = [];
    await assert.rejects(db.messages.query().where(MessageFields.body.eq(5)).all(), fails('query', 'body'));
    assert.throws(() => db.messages.query().where(MessageFields.priority.gt(1)).orderBy(MessageFields.body).watch(() => {}, e => errors.push(e)), fails('query'));
    assert.throws(() => db.messages.query().limit(0), fails('query'));
    await assert.rejects(db.users.get('../invalid'), fails('validation'));
    assert.deepEqual(errors, []);
  } finally { await terminate(sdk); await deleteApp(app); }
});

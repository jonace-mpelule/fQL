import assert from 'node:assert/strict';
import { test } from 'node:test';
import { initializeTestEnvironment, assertFails } from '@firebase/rules-unit-testing';
import { readFile } from 'node:fs/promises';
import { doc, setDoc, Timestamp } from 'firebase/firestore';
import { FqlClient, FqlError, MessageFields, ConversationInfo, User } from './build/generated/fql.js';

test('generated TypeScript CRUD, listener, batch and transaction work with enforcing Rules', async () => {
  const address = process.env.FIRESTORE_EMULATOR_HOST;
  assert.ok(address, 'Run through npm run test:typescript:emulator; this test requires the local emulator');
  const [host, port] = address.split(':');
  const env = await initializeTestEnvironment({ projectId: 'demo-fql', firestore: {
    host, port: Number(port), rules: await readFile(new URL('./firebase/generated/firestore.rules', import.meta.url), 'utf8'),
  } });
  try {
    await env.clearFirestore();
    const sdk = env.authenticatedContext('ts-owner').firestore()._delegate;
    const db = new FqlClient(sdk);
    const profile = await db.users.create('ts-owner').setName('Example').save();
    assert.equal(profile.id, 'ts-owner'); assert.equal(profile.active, true);
    await assert.rejects(db.users.create('ts-owner').setName('Duplicate').save(), e => e instanceof FqlError && e.code === 'already-exists');
    await db.users.update(profile.id).setBio(null).save();
    assert.equal((await db.users.get(profile.id)).data.bio, null);
    await db.users.update(profile.id).removeBio().save();
    assert.equal((await db.users.get(profile.id)).data.hasBio, false);
    const message = await db.messages.create().setOwnerId('ts-owner').setBody('Hello').setCreatedAt(new Timestamp(1700000000, 123456000)).save();
    const query = db.messages.query().where(MessageFields.ownerId.eq('ts-owner')).orderBy(MessageFields.createdAt).limit(1);
    const first = await query.all('server'); assert.equal(first.data[0].body, 'Hello');
    assert.equal((await query.startAfter(first.cursor).all()).data.length, 0);
    await new Promise((resolve, reject) => {
      const timer = setTimeout(() => { stop(); reject(new Error('Listener timed out')); }, 10000);
      const stop = db.messages.watch(message.id, snapshot => {
        if (!snapshot.hasPendingWrites) { clearTimeout(timer); stop(); resolve(); }
      }, error => { clearTimeout(timer); stop(); reject(error); });
    });
    const op = db.conversations.create().setOwnerId('ts-owner').setInfo(ConversationInfo.create().setTitle('Room').build()).operation();
    const batch = db.batch(); batch.add(op); await batch.commit();
    assert.equal((await db.conversations.get(op.id, 'server')).data.createdAt.isPending, false);
    await db.transaction(async tx => {
      const current = await tx.get(db.conversations, op.id);
      tx.add(db.conversations.update(op.id).setCount(current.data.count + 1).operation());
    });
    assert.equal((await db.conversations.get(op.id)).data.count, 1);
    const managed = await db.conversations.create().setOwnerId('ts-owner').setInfo(ConversationInfo.create().setTitle('Managed').build()).save();
    assert.equal(managed.createdAt.isPending, false);
    await assertFails(setDoc(doc(sdk, 'users', 'ts-owner'), { name: 123, active: true, status: 'active' }));
    await assertFails(setDoc(doc(sdk, 'messages', message.id), { ...message.toMap(), ownerId: 'other' }));
    const other = new FqlClient(env.authenticatedContext('other').firestore()._delegate);
    await assert.rejects(other.messages.get(message.id), e => e instanceof FqlError && e.code === 'permission');
    // Privileged legacy data is rejected by the generated decoder, including listeners.
    await env.withSecurityRulesDisabled(async context => {
      await setDoc(doc(context.firestore(), 'users', 'ts-owner'), { name: 123, active: true, status: 'active' });
    });
    await assert.rejects(db.users.get('ts-owner', 'server'), e => e instanceof FqlError && e.code === 'decoding' && e.field === 'name');
    await new Promise((resolve, reject) => {
      let failed = false;
      const timer = setTimeout(() => { stop(); reject(new Error('Listener recovery timed out')); }, 10000);
      const stop = db.users.watch('ts-owner', snapshot => {
        if (failed && snapshot.data.name === 'Repaired') { clearTimeout(timer); stop(); resolve(); }
      }, error => {
        if (error.code !== 'decoding') { clearTimeout(timer); stop(); reject(error); return; }
        if (failed) return;
        failed = true;
        env.withSecurityRulesDisabled(async context => {
          await setDoc(doc(context.firestore(), 'users', 'ts-owner'), { name: 'Repaired', active: true, status: 'active' });
        }).catch(reject);
      });
    });
    await db.messages.delete(message.id);
    assert.equal(User.decode('u', profile.toMap()).name, 'Example');
  } finally { await env.cleanup(); }
});

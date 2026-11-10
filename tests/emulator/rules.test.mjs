import { readFile } from 'node:fs/promises';
import { after, before, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';
import { initializeTestEnvironment } from '@firebase/rules-unit-testing';
import { doc, setDoc, getDoc, updateDoc, deleteDoc, getDocs, collection, query, where, orderBy, limit, Timestamp, deleteField, setLogLevel, serverTimestamp, writeBatch } from 'firebase/firestore';

setLogLevel('silent');
let env;
const user = (extra = {}) => ({ name: 'Alice', active: true, status: 'active', ...extra });
const message = (extra = {}) => ({ ownerId: 'alice', body: 'Hello', createdAt: Timestamp.fromMillis(1_700_000_000_000), priority: 0, ...extra });
const denied = (operation) => assert.rejects(operation, error => error.code === 'permission-denied');
before(async () => {
  env = await initializeTestEnvironment({ projectId: 'demo-fql', firestore: {
    rules: await readFile(new URL('../../examples/flutter/firebase/generated/firestore.rules', import.meta.url), 'utf8'),
  }});
});
after(async () => { await env?.cleanup(); });
beforeEach(async () => { await env.clearFirestore(); });

test('owner by document ID controls CRUD; anonymous and other users are denied', async () => {
  const alice = env.authenticatedContext('alice').firestore();
  const bob = env.authenticatedContext('bob').firestore();
  const anon = env.unauthenticatedContext().firestore();
  await setDoc(doc(alice, 'users/alice'), user());
  assert.equal((await getDoc(doc(alice, 'users/alice'))).data().name, 'Alice');
  await denied(getDoc(doc(bob, 'users/alice')));
  await denied(getDoc(doc(anon, 'users/alice')));
  await denied(setDoc(doc(alice, 'users/bob'), user()));
  await denied(updateDoc(doc(bob, 'users/alice'), { name: 'Bad' }));
  await denied(deleteDoc(doc(bob, 'users/alice')));
  await deleteDoc(doc(alice, 'users/alice'));
});

test('document structure, optionality, nullability, bytes, and scalar bounds', async () => {
  const db = env.authenticatedContext('alice').firestore();
  const ref = doc(db, 'users/alice');
  for (const data of [user({ name: 1 }), user({ name: '' }), user({ name: '😃'.repeat(26) }),
    user({ active: null }), user({ extra: 'bad' }), user({ rating: 6 }), user({ rating: NaN }),
    user({ rating: Infinity }), user({ status: 'invalid' }), { name: 'Missing required field' }, user({ dateOfBirth: null })]) {
    await denied(setDoc(ref, data));
  }
  await setDoc(ref, user({ bio: null, rating: 4.5, dateOfBirth: '2000-02-29' }));
  await updateDoc(ref, { bio: deleteField() });
  assert.equal(Object.hasOwn((await getDoc(ref)).data(), 'bio'), false);
  await denied(updateDoc(ref, { active: deleteField() }));
});

test('calendar dates reject impossible dates and invalid years', async () => {
  const db = env.authenticatedContext('alice').firestore();
  const ref = doc(db, 'users/alice');
  for (const date of ['2025-02-29', '1900-02-29', '2026-04-31', '0000-01-01', '2026-00-01', '2026-13-01', '2026-01-00', '2026-1-01', 'wrong']) {
    await denied(setDoc(ref, user({ dateOfBirth: date })));
  }
  for (const date of ['2000-02-29', '2024-02-29', '0001-01-01', '9999-12-31']) {
    await setDoc(ref, user({ dateOfBirth: date }));
  }
});

test('owner field cannot be changed and timestamps/integers retain constraints', async () => {
  const db = env.authenticatedContext('alice').firestore();
  const ref = doc(db, 'messages/m1');
  await setDoc(ref, message());
  await updateDoc(ref, { body: 'Updated', priority: 10 });
  await denied(updateDoc(ref, { ownerId: 'bob' }));
  await denied(updateDoc(ref, { ownerId: deleteField() }));
  await denied(updateDoc(ref, { createdAt: Timestamp.fromMillis(100) }));
  await denied(updateDoc(ref, { priority: 1.5 }));
  await denied(updateDoc(ref, { priority: 11 }));
  await denied(setDoc(doc(db, 'messages/m2'), message({ ownerId: 'bob' })));
  await denied(setDoc(doc(db, 'messages/m2'), message({ createdAt: 'not a timestamp' })));
});

test('queries must carry ownership predicates; explicit indexes are valid artifacts', async () => {
  const db = env.authenticatedContext('alice').firestore();
  await setDoc(doc(db, 'messages/m1'), message());
  await denied(getDocs(collection(db, 'messages')));
  const result = await getDocs(query(collection(db, 'messages'), where('ownerId', '==', 'alice'), orderBy('createdAt', 'desc'), limit(10)));
  assert.equal(result.size, 1);
  const indexes = JSON.parse(await readFile(new URL('../../examples/flutter/firebase/generated/firestore.indexes.json', import.meta.url)));
  assert.deepEqual(indexes.indexes[0], { collectionGroup: 'messages', queryScope: 'COLLECTION', fields: [
    { fieldPath: 'ownerId', order: 'ASCENDING' }, { fieldPath: 'createdAt', order: 'DESCENDING' },
  ]});
});

test('undeclared collections are denied', async () => {
  const db = env.authenticatedContext('alice').firestore();
  await denied(setDoc(doc(db, 'unknown/x'), { anything: true }));
  await denied(getDoc(doc(db, 'unknown/x')));
});

test('privileged seeding bypasses rules but clients cannot write malformed replacements', async () => {
  await env.withSecurityRulesDisabled(async context => {
    await setDoc(doc(context.firestore(), 'users/alice'), { name: 42 });
  });
  const db = env.authenticatedContext('alice').firestore();
  assert.equal((await getDoc(doc(db, 'users/alice'))).data().name, 42);
  await denied(updateDoc(doc(db, 'users/alice'), { active: true }));
  await setDoc(doc(db, 'users/alice'), user());
});

test('authenticated/default-deny policies and all scalar literals compile into enforcing rules', async () => {
  const original = await readFile(new URL('../../examples/flutter/firebase/generated/firestore.rules', import.meta.url), 'utf8');
  let fixture;
  try {
    fixture = await initializeTestEnvironment({ projectId: 'demo-fql', firestore: {
      rules: await readFile(new URL('../dart/firebase/generated/firestore.rules', import.meta.url), 'utf8'),
    }});
    const db = fixture.authenticatedContext('fixture').firestore();
    const anon = fixture.unauthenticatedContext().firestore();
    await setDoc(doc(db, 'tiny/t1'), { label: 'Allowed' });
    await denied(setDoc(doc(anon, 'tiny/t2'), { label: 'Anonymous' }));
    await denied(setDoc(doc(db, 'denied/d1'), { flag: true }));
    const valid = {
      title: 'a\u0007$dollar"', birthday: '2000-02-29', moment: new Timestamp(1767225600, 123456000),
      number: 0.00001, nullable: null, flag: false,
    };
    await setDoc(doc(db, 'defaults/d1'), valid);
    await denied(updateDoc(doc(db, 'defaults/d1'), { number: 2 }));
    await denied(updateDoc(doc(db, 'defaults/d1'), { birthday: '2000-03-01' }));
    await denied(updateDoc(doc(db, 'defaults/d1'), { moment: Timestamp.fromMillis(0) }));
  } finally {
    await fixture?.cleanup();
    const restored = await initializeTestEnvironment({ projectId: 'demo-fql', firestore: { rules: original }});
    await restored.cleanup();
  }
});


test('enums, embedded maps, bounded lists and managed timestamps are enforced for raw SDK callers', async () => {
  const db=env.authenticatedContext('alice').firestore();
  const ref=doc(db,'conversations/c1');
  const valid={ownerId:'alice',info:{title:'Room'},state:'open',count:0,tags:['one'],history:[{title:'Old'}],createdAt:serverTimestamp(),updatedAt:serverTimestamp()};
  for(const extra of [{state:'unknown'},{info:{title:'Room',extra:true}},{tags:[1]},{tags:Array(9).fill('too many')},{history:[{title:1}]},{createdAt:Timestamp.fromMillis(0)}]) {
    await denied(setDoc(ref,{...valid,...extra}));
  }
  await setDoc(ref,valid);
  await denied(updateDoc(ref,{count:1}));
  await denied(updateDoc(ref,{createdAt:serverTimestamp(),updatedAt:serverTimestamp()}));
  await updateDoc(ref,{state:'archived',count:1,updatedAt:serverTimestamp()});
  const batch=writeBatch(db);
  batch.set(doc(db,'messages/good'),message());
  batch.set(doc(db,'messages/bad'),message({ownerId:'bob'}));
  await denied(batch.commit());
  await env.withSecurityRulesDisabled(async context=>assert.equal((await getDoc(doc(context.firestore(),'messages/good'))).exists(),false));
});

import { mkdtemp, mkdir, readFile, cp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { before, beforeEach, after, test } from 'node:test';
import assert from 'node:assert/strict';
import { initializeTestEnvironment } from '@firebase/rules-unit-testing';
import { doc, getDoc, getDocs, collection, query, where, orderBy, limit, setDoc, updateDoc, deleteDoc, writeBatch, runTransaction, Timestamp, serverTimestamp, setLogLevel } from 'firebase/firestore';

setLogLevel('silent');
let env, root;
const claims = { deviceId: 'device-a', generation: 2 };
const denied = operation => assert.rejects(operation, e => e.code === 'permission-denied');
const account = extra => ({ deviceId: 'device-a', generation: 2, active: true, ...extra });
const envelope = extra => ({ recipientId: 'alice', senderId: 'sender', deviceId: 'device-a', generation: 2, expiresAt: Timestamp.fromMillis(Date.now() + 3_600_000), body: 'Encrypted fixture', ...extra });
const dbFor = (uid = 'alice', token = claims) => env.authenticatedContext(uid, token).firestore();
const seed = async (path, data) => env.withSecurityRulesDisabled(c => setDoc(doc(c.firestore(), path), data));
before(async () => {
  root = await mkdtemp(join(tmpdir(), 'fql-policy-'));
  await mkdir(join(root, 'fQL'), { recursive: true });
  await cp(new URL('./fixtures/policies', import.meta.url), join(root, 'fQL/schemas'), { recursive: true });
  await writeFile(join(root, 'fQL/fql.config'), 'version="0.1"\ntarget="dart"\nschemas="schemas"\noutput="../client"\nfirebase="../firebase"\n');
  execFileSync('go', ['run', './cmd/fql', 'generate', '--project', root], { stdio: 'pipe' });
  env = await initializeTestEnvironment({ projectId: 'demo-fql-policies', firestore: {
    rules: await readFile(join(root, 'firebase/firestore.rules'), 'utf8'),
  }});
});
after(async () => { await env?.cleanup(); if (root) await rm(root, { recursive: true, force: true }); });
beforeEach(async () => {
  await env.clearFirestore();
  await seed('accounts/alice', account());
  await seed('envelopes/e1', envelope());
});

test('device policies allow valid credentials and reject impersonation, stale and malformed claims', async () => {
  assert.equal((await getDoc(doc(dbFor(), 'envelopes/e1'))).exists(), true);
  for (const db of [env.unauthenticatedContext().firestore(), dbFor('bob'), dbFor('alice', {}),
    dbFor('alice', { ...claims, deviceId: 'other' }), dbFor('alice', { ...claims, generation: 1 }),
    dbFor('alice', { ...claims, generation: '2' }), dbFor('alice', { ...claims, generation: null }),
    dbFor('alice', { ...claims, generation: 2.5 })]) await denied(getDoc(doc(db, 'envelopes/e1')));
});

test('account revocation, suspension, missing/malformed related data, blocks and expiration deny', async () => {
  const db = dbFor();
  for (const data of [account({ deviceId: 'new-device' }), account({ generation: 3 }), account({ active: false }),
    account({ active: 'true' }), { active: true }]) {
    await seed('accounts/alice', data);
    await denied(getDoc(doc(db, 'envelopes/e1')));
  }
  await env.withSecurityRulesDisabled(c => deleteDoc(doc(c.firestore(), 'accounts/alice')));
  await denied(getDoc(doc(db, 'envelopes/e1')));
  await seed('accounts/alice', account());
  await seed('blocks/alice', { reason: 'blocked' });
  await denied(getDoc(doc(db, 'envelopes/e1')));
  await env.withSecurityRulesDisabled(c => deleteDoc(doc(c.firestore(), 'blocks/alice')));
  for (const expiresAt of [Timestamp.fromMillis(0), Timestamp.fromMillis(Date.now() - 60_000), 'tomorrow']) {
    await seed('envelopes/e1', envelope({ expiresAt }));
    await denied(getDoc(doc(db, 'envelopes/e1')));
  }
});

test('mailbox query must constrain recipient, device, generation and a future expiration cutoff', async () => {
  const db = dbFor();
  const mailbox = collection(db, 'envelopes');
  const filters = [where('recipientId', '==', 'alice'), where('deviceId', '==', 'device-a'),
    where('generation', '==', 2), where('expiresAt', '>', Timestamp.fromMillis(Date.now() + 60_000))];
  const result = await getDocs(query(mailbox, ...filters, orderBy('expiresAt'), limit(10)));
  assert.equal(result.size, 1);
  await denied(getDocs(mailbox));
  for (let i = 0; i < filters.length; i++) await denied(getDocs(query(mailbox, ...filters.filter((_, j) => i !== j))));
  await seed('accounts/alice', account({ generation: 3 }));
  await denied(getDocs(query(mailbox, ...filters)));
});

test('get and list grants remain independent', async () => {
  await seed('single/x', { flag: true });
  await seed('listing/x', { ownerId: 'alice' });
  const db = dbFor();
  await getDoc(doc(db, 'single/x'));
  await denied(getDocs(collection(db, 'single')));
  await denied(getDoc(doc(db, 'listing/x')));
  assert.equal((await getDocs(query(collection(db, 'listing'), where('ownerId', '==', 'alice')))).size, 1);
});

test('negation cannot turn missing or malformed security inputs into a grant', async () => {
  await seed('negated/x', { flag: true });
  assert.equal((await getDoc(doc(dbFor(), 'negated/x'))).exists(), true);
  for (const token of [{}, { deviceId: null }, { deviceId: 1 }]) await denied(getDoc(doc(dbFor('alice', token), 'negated/x')));
  await seed('accounts/alice', { deviceId: 'device-a' });
  await denied(getDoc(doc(dbFor(), 'negated/x')));
});

test('create ownership, partial updates, immutable routing, and structure remain enforced', async () => {
  const db = dbFor('sender', {});
  const ref = doc(db, 'envelopes/new');
  await denied(setDoc(ref, envelope({ senderId: 'alice' })));
  await denied(setDoc(ref, envelope({ extra: true })));
  await denied(setDoc(ref, envelope({ body: '' })));
  await denied(setDoc(ref, envelope({ generation: '2' })));
  await setDoc(ref, envelope());
  await updateDoc(ref, { body: 'Updated' });
  for (const patch of [{ recipientId: 'bob' }, { deviceId: 'other' }, { generation: 3 }, { senderId: 'other' }, { body: 4 }, { unknown: true }]) await denied(updateDoc(ref, patch));
  await denied(updateDoc(doc(dbFor(), 'envelopes/new'), { body: 'not sender' }));
});

test('getAfter requires a companion change in batches and transactions; failed atomic writes leave no record', async () => {
  await seed('companions/alice', { generation: 1 });
  const db = dbFor();
  const record = doc(db, 'atomicRecords/r1');
  const companion = doc(db, 'companions/alice');
  await denied(setDoc(record, { ownerId: 'alice', generation: 2 }));
  const batch = writeBatch(db);
  batch.set(record, { ownerId: 'alice', generation: 2 });
  batch.update(companion, { generation: 2 });
  await batch.commit();
  await denied(updateDoc(record, { generation: 3 }));
  await runTransaction(db, async tx => {
    const current = await tx.get(companion);
    tx.update(companion, { generation: current.data().generation + 1 });
    tx.update(record, { generation: 3 });
  });
  const bad = writeBatch(db);
  bad.set(doc(db, 'atomicRecords/bad'), { ownerId: 'alice', generation: 4 });
  bad.update(companion, { generation: 3 });
  await denied(bad.commit());
  await env.withSecurityRulesDisabled(async c => assert.equal((await getDoc(doc(c.firestore(), 'atomicRecords/bad'))).exists(), false));
});


test('expiration rejects equality with server request time and malformed timestamps under negation', async () => {
  const db = dbFor();
  await denied(setDoc(doc(db, 'expiry/x'), { ownerId: 'alice', expiresAt: serverTimestamp() }));
  await denied(setDoc(doc(db, 'expiry/x'), { ownerId: 'alice', expiresAt: Timestamp.fromMillis(0) }));
  await setDoc(doc(db, 'expiry/x'), { ownerId: 'alice', expiresAt: Timestamp.fromMillis(Date.now() + 60_000) });
  await seed('expired/x', { expiresAt: 'tomorrow' });
  await denied(getDoc(doc(db, 'expired/x')));
  await seed('expired/x', { expiresAt: Timestamp.fromMillis(0) });
  assert.equal((await getDoc(doc(db, 'expired/x'))).exists(), true);
});

test('or expressions, typed bool/float claims, and calendar guards remain strict', async () => {
  const data = { birthday: '2000-02-29', flag: true, score: 1.5 };
  for (const [index, token] of [{ staff: true, weight: 0 }, { staff: false, weight: 0.75 }].entries()) {
    const db = dbFor('alice', token);
    await setDoc(doc(db, `policyTypes/valid${index}`), data);
    assert.equal((await getDoc(doc(db, `policyTypes/valid${index}`))).exists(), true);
  }
  for (const token of [{ staff: false, weight: 0.25 }, { staff: true }, { staff: 'true', weight: 1 },
    { staff: true, weight: 1e16 }]) await denied(getDoc(doc(dbFor('alice', token), 'policyTypes/valid0')));
  const db = dbFor('alice', { staff: true, weight: 0 });
  await denied(setDoc(doc(db, 'policyTypes/invalid'), { ...data, birthday: '2025-02-29' }));
  await seed('policyTypes/invalid', { ...data, birthday: '2025-02-29' });
  await denied(getDoc(doc(db, 'policyTypes/invalid')));
});

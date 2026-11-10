// Compile-time checks for the public API; this function performs no I/O here.
import type { Firestore } from 'firebase/firestore';
import { FqlClient, FqlDate, Timestamp, ConversationInfo, ConversationState, MessageFields } from './generated/fql.js';
export function usage(sdk: Firestore, uid: string) {
  const db = new FqlClient(sdk);
  const message = db.messages.create().setOwnerId(uid).setBody('Hello').setCreatedAt(Timestamp.now());
  db.users.create(uid).setName('Example').setDateOfBirth(new FqlDate(2000, 2, 29)).operation();
  const info = ConversationInfo.create().setTitle('Room').setDescription(null).build();
  const op = db.conversations.create().setOwnerId(uid).setInfo(info).setState(ConversationState.open).setTags(['team']).operation();
  const batch = db.batch(); batch.add(op);
  db.conversations.update(op.id).removeTags().setInfo(info).operation();
  const query = db.messages.query().where(MessageFields.ownerId.eq(uid)).orderBy(MessageFields.createdAt, 'desc').limit(20);
  const stop = query.watch(snapshot => { const body: string | undefined = snapshot.data[0]?.body; void body; }, error => { void error.code; });
  stop();
  // @ts-expect-error schema string fields reject numbers
  message.setBody(123);
  // @ts-expect-error immutable ownership cannot be updated
  db.messages.update('id').setOwnerId(uid);
  // @ts-expect-error managed fields have no setters
  db.conversations.create().setCreatedAt(Timestamp.now());
  // @ts-expect-error typed predicates reject wrong values
  MessageFields.ownerId.eq(12);
  const decoded = ConversationInfo.decode({ title: 'Room' });
  const presence: string | null | undefined = decoded.description;
  // @ts-expect-error optional nullable fields cannot be read as always present strings
  const required: string = decoded.description;
  void presence; void required;
}

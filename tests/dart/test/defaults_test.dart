import 'package:cloud_firestore/cloud_firestore.dart';
import 'package:fake_cloud_firestore/fake_cloud_firestore.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:fql_runtime_tests/generated/fql.dart';

void main() {
  test('all literal defaults round-trip through generated codecs', () async {
    final db = FqlClient(FakeFirebaseFirestore());
    final value = await db.defaults.create().save();
    expect(value.title, 'a\u0007\$dollar"');
    expect(value.birthday, FqlDate(2000, 2, 29));
    expect(value.moment, Timestamp(1767225600, 123456000));
    expect(value.number, 0.00001);
    expect(value.flag, false);
    expect(value.nullable, null);
    expect(value.hasOptional, false);
    final decoded = Defaults.decode(value.id, value.toMap());
    expect(decoded.moment, value.moment);
    expect(decoded.birthday, value.birthday);
    expect(decoded.title, value.title);
    await expectLater(
      db.defaults.update(value.id).setNumber(2).save(),
      throwsA(isA<FqlError>()),
    );
    expect(() => Defaults.decode('old', {}), throwsA(isA<FqlError>()));
  });
  test('minimal schemas compile and use the same runtime', () async {
    final db = FqlClient(FakeFirebaseFirestore());
    final value = await db.tiny.create().setLabel('Small').save();
    expect((await db.tiny.get(value.id)).data.label, 'Small');
  });
}

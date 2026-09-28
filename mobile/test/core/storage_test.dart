import 'package:agrismart/core/storage/local_db.dart';
import 'package:agrismart/core/storage/token_store.dart';
import 'package:drift/native.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('kv documents round-trip and delete', () async {
    final db = LocalDb(NativeDatabase.memory());
    expect(await db.read('x'), isNull);
    await db.write('x', {'a': 1});
    await db.write('x', {'a': 2});
    expect(await db.read('x'), {'a': 2});
    await db.remove('x');
    expect(await db.read('x'), isNull);
    await db.close();
  });

  test('sync ops keep sequence order, dequeue and reject', () async {
    final db = LocalDb(NativeDatabase.memory());
    final t = DateTime.utc(2026);
    expect(await db.enqueue('k1', 'create_scan', {'a': 1}, t), 1);
    expect(await db.enqueue('k2', 'annotate_scan', {}, t), 2);
    await db.reject('k2', 'scan.invalid');
    expect((await db.ops(pendingOnly: true)).map((o) => o.idempotencyKey), ['k1']);
    final all = await db.ops();
    expect(all.last.status, 'rejected');
    expect(all.last.errorCode, 'scan.invalid');
    expect(all.first.payload, {'a': 1});
    expect(all.first.createdAt, t);
    expect(all.first.seq, 1);
    expect(all.first.kind, 'create_scan');
    await db.dequeue('k1');
    expect((await db.ops()).length, 1);
    await db.close();
  });

  test('secure token store round-trip', () async {
    FlutterSecureStorage.setMockInitialValues({});
    final s = SecureTokenStore();
    expect(await s.read(), isNull);
    const t = Tokens(access: 'a', refresh: 'r', role: 'guest', userId: 'u');
    await s.write(t);
    final back = await s.read();
    expect(back!.access, 'a');
    expect(back.isGuest, isTrue);
    expect(back.userId, 'u');
    await s.clear();
    expect(await s.read(), isNull);
  });
}

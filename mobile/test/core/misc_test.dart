import 'package:agrismart/app/services.dart';
import 'package:agrismart/core/clock.dart';
import 'package:agrismart/core/storage/local_db.dart';
import 'package:drift/native.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/fakes.dart';

void main() {
  test('clocks and ids', () {
    final c = FixedClock(epoch)..advance(const Duration(hours: 1));
    expect(c.now(), epoch.add(const Duration(hours: 1)));
    final s = SequenceIdGenerator();
    expect(s.next(), '00000000-0000-7000-8000-000000000001');
    expect(UuidV7Generator().next(), isNot(UuidV7Generator().next()));
  });

  test('local db declares no generated tables', () async {
    final db = LocalDb(NativeDatabase.memory());
    expect(db.allTables, isEmpty);
    await db.close();
  });

  testWidgets('AppScope exposes services and notifies on change', (tester) async {
    final a = (await tester.runAsync(() => TestKit.create()))!.services;
    final b = (await tester.runAsync(() => TestKit.create()))!.services;
    late AppServices found;
    await tester.pumpWidget(AppScope(services: a, child: Builder(builder: (c) {
      found = AppScope.of(c);
      return const SizedBox();
    })));
    expect(found, same(a));
    expect(AppScope(services: b, child: const SizedBox()).updateShouldNotify(AppScope(services: a, child: const SizedBox())), isTrue);
  });
}

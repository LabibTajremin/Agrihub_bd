import 'package:agrismart/app/shell.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/fakes.dart';

/// Lets real async work (the local database) complete, then settles frames.
Future<void> settle(WidgetTester tester) async {
  for (var i = 0; i < 5; i++) {
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 20)));
    await tester.pumpAndSettle();
  }
}

Future<void> tapText(WidgetTester tester, String text) async {
  await tester.ensureVisible(find.text(text).last);
  await tester.tap(find.text(text).last);
  await settle(tester);
}

/// The complete first-run journey: splash → language (Bangla) → slides →
/// phone → OTP → profile → permissions → offline model → home. Shared by the
/// widget suite and the on-device integration test.
Future<void> runFirstRunFlow(WidgetTester tester, TestKit kit) async {
  kit.stubAuth();
  await kit.pump(tester, location: '/');
  await settle(tester);

  // language picker — switch to Bangla live
  expect(find.text('Choose your language'), findsOneWidget);
  await tester.tap(find.byKey(const Key('lang.bn')));
  await settle(tester);
  expect(find.text('আপনার ভাষা বেছে নিন'), findsOneWidget);
  await tester.tap(find.byType(FilledButton));
  await settle(tester);

  // three value slides
  for (var i = 0; i < 3; i++) {
    await tester.tap(find.byType(FilledButton));
    await settle(tester);
  }

  // phone → OTP
  await tester.enterText(find.byKey(const Key('phone.input')), '01712345678');
  await tester.tap(find.byType(FilledButton));
  await settle(tester);
  expect(find.textContaining('+8801712345678'), findsOneWidget);
  await tester.enterText(find.byKey(const Key('otp.input')), '123456');
  await tester.pump();
  await tester.tap(find.byType(FilledButton));
  await settle(tester);
  expect((await kit.tokens.read())!.role, 'farmer');

  // profile
  await tester.enterText(find.byKey(const Key('profile.name')), 'Rahim');
  await tester.enterText(find.byKey(const Key('profile.district')), 'Bogura');
  await tester.tap(find.byType(FilledButton));
  await settle(tester);
  final patch = kit.server.seen.lastWhere((r) => r.method == 'PATCH');
  expect(patch.body, {'name': 'Rahim', 'district': 'Bogura', 'language': 'bn'});

  // permissions primer → model download → home
  await tester.tap(find.byType(FilledButton));
  await settle(tester);
  await tester.tap(find.byType(FilledButton));
  await settle(tester);
  await tester.tap(find.byType(FilledButton));
  await settle(tester);
  expect(find.byType(Shell), findsOneWidget);
  expect(await tester.runAsync(() => kit.services.onboarding.completed), isTrue);
}

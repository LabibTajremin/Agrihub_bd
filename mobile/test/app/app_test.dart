import 'package:agrismart/core/theme/theme.dart';
import 'package:agrismart/core/theme/tokens.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/fakes.dart';

void main() {
  testWidgets('language switches at runtime without a restart, direction follows is_rtl', (tester) async {
    final kit = await tester.runAsync(() => TestKit.create());
    await kit!.pump(tester);
    expect(find.text('Home'), findsOneWidget);
    final state = tester.state(find.byType(Scaffold));
    expect(Directionality.of(state.context), TextDirection.ltr);
    await tester.runAsync(() => kit.services.l10n.setLanguage('ar'));
    await tester.pumpAndSettle();
    expect(find.text('الرئيسية'), findsOneWidget);
    // the same Scaffold element survives: rebuilt in place, not restarted
    expect(tester.state(find.byType(Scaffold)), same(state));
    expect(Directionality.of(state.context), TextDirection.rtl);
  });

  testWidgets('tabs navigate', (tester) async {
    final kit = await tester.runAsync(() => TestKit.create());
    await kit!.pump(tester);
    for (final label in ['Doctor', 'Advisor', 'Voice', 'Settings', 'Home']) {
      await tester.tap(find.text(label).last);
      await tester.pumpAndSettle();
    }
    expect(find.text('Scan a leaf'), findsOneWidget);
  });

  test('theme is built from the 37 tokens', () {
    final t = AppTheme.light();
    expect(Tokens.count, 37);
    expect(t.colorScheme.primary, Tokens.primary);
    expect(t.scaffoldBackgroundColor, Tokens.background);
    expect(t.textTheme.bodyLarge!.fontSize, Tokens.body.fontSize);
  });

  testWidgets('golden: light theme shell (English)', (tester) async {
    tester.view.physicalSize = const Size(1080, 1920);
    tester.view.devicePixelRatio = 3;
    addTearDown(tester.view.reset);
    final kit = await tester.runAsync(() => TestKit.create());
    await kit!.pump(tester);
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('goldens/shell_en.png'));
  });

  testWidgets('golden: Arabic mirrors the layout (RTL)', (tester) async {
    tester.view.physicalSize = const Size(1080, 1920);
    tester.view.devicePixelRatio = 3;
    addTearDown(tester.view.reset);
    final kit = await tester.runAsync(() => TestKit.create(lang: 'ar'));
    await kit!.pump(tester);
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('goldens/shell_ar_rtl.png'));
  });
}

import 'package:agrismart/app/app.dart';
import 'package:agrismart/app/router.dart';
import 'package:agrismart/core/models/offline_model.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import '../../flows/first_run.dart';
import '../../support/fakes.dart';

Future<TestKit> openSettings(WidgetTester tester, {bool signedIn = true, String path = '/settings', ModelSource? source}) async {
  final kit = (await tester.runAsync(() => TestKit.create(modelSource: source)))!;
  await tester.runAsync(kit.onboarded);
  kit.stubAuth();
  if (signedIn) {
    await tester.runAsync(() async {
      await kit.signIn();
      await kit.services.db.write('profile', {...TestKit.user('farmer'), 'name': 'Rahim', 'district': 'Bogura'});
    });
  }
  await tester.pumpWidget(AgriSmartApp(services: kit.services, router: buildRouter(initialLocation: path)));
  await settle(tester);
  return kit;
}

void main() {
  testWidgets('settings lists every section; profile edits save', (tester) async {
    final kit = await openSettings(tester);
    for (final t in ['Profile', 'Language', 'Offline models', 'Expert help', 'Privacy', 'Sign out', 'Version 0.1.0']) {
      expect(find.text(t), findsOneWidget, reason: t);
    }
    expect(find.text('Rahim'), findsOneWidget);
    expect(find.text('English'), findsOneWidget);
    await tapText(tester, 'Profile');
    expect(find.widgetWithText(TextField, 'Bogura'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('settings-name')), 'Rahim Uddin');
    await tapText(tester, 'Save');
    expect(find.text('Rahim Uddin'), findsOneWidget);
    expect(kit.server.seen.last.body, {'name': 'Rahim Uddin', 'district': 'Bogura'});
  });

  testWidgets('profile save errors stay on the screen', (tester) async {
    final kit = await openSettings(tester, path: '/settings/profile');
    kit.server.json('PATCH', '/v1/me', {'error': {'code': 'user.invalid_profile', 'message': 'errors.user.invalid_profile'}}, 400);
    await tapText(tester, 'Save');
    expect(find.text('Please check your profile details.'), findsOneWidget);
  });

  testWidgets('language selector switches live and updates the profile', (tester) async {
    final kit = await openSettings(tester);
    await tapText(tester, 'Language');
    await tester.tap(find.byKey(const Key('settings-lang-bn')));
    await settle(tester);
    expect(kit.services.l10n.current.language.code, 'bn');
    expect(kit.server.seen.last.body, {'language': 'bn'});
    kit.server.offline = true;
    await tester.tap(find.byKey(const Key('settings-lang-ar')));
    await settle(tester);
    expect(Directionality.of(tester.element(find.byType(ListView))), TextDirection.rtl);
  });

  testWidgets('guest sees sign in; language change skips the profile', (tester) async {
    final kit = await openSettings(tester, signedIn: false);
    expect(find.text('Sign in'), findsOneWidget);
    expect(find.text('Sign out'), findsNothing);
    await tapText(tester, 'Language');
    await tester.tap(find.byKey(const Key('settings-lang-hi')));
    await settle(tester);
    expect(kit.server.seen.where((r) => r.path == '/v1/me'), isEmpty);
    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(AgriSmartApp(services: kit.services, router: buildRouter(initialLocation: '/settings')));
    await settle(tester);
    await tester.tap(find.byIcon(Icons.login));
    await settle(tester);
    expect(find.byKey(const Key('phone.input')), findsOneWidget);
  });

  testWidgets('sign out revokes the session even offline', (tester) async {
    final kit = await openSettings(tester);
    kit.server.offline = true;
    await tester.tap(find.byKey(const Key('sign-out')));
    await settle(tester);
    expect(find.byKey(const Key('phone.input')), findsOneWidget);
    expect(await tester.runAsync(kit.tokens.read), isNull);
    expect(await tester.runAsync(kit.services.auth.cachedProfile), isNull);
  });

  testWidgets('offline model manager: download, progress, remove', (tester) async {
    final source = ControlledModelSource();
    final kit = await openSettings(tester, path: '/settings/models', source: source);
    expect(find.text('24 MB'), findsOneWidget);
    await tapText(tester, 'Download');
    source.controller.add(0.4);
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 20)));
    await tester.pump();
    expect(find.text('Downloading… 40%'), findsOneWidget);
    await source.controller.close();
    await settle(tester);
    expect(find.text('Installed · 0.0.0'), findsOneWidget);
    await tapText(tester, 'Remove');
    expect(kit.services.models.status, ModelStatus.absent);
    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(AgriSmartApp(services: kit.services, router: buildRouter(initialLocation: '/settings')));
    await settle(tester);
    await tapText(tester, 'Offline models');
    expect(find.text('Models used to diagnose crops without internet.'), findsOneWidget);
  });

  testWidgets('expert help copies the helpline; privacy explains data use', (tester) async {
    String? copied;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, (call) async {
      if (call.method == 'Clipboard.setData') {
        copied = (call.arguments as Map)['text'] as String;
      }
      return null;
    });
    await openSettings(tester);
    await tapText(tester, 'Expert help');
    await tapText(tester, 'Call 16123');
    expect(copied, '16123');
    expect(find.text('16123'), findsOneWidget);
    await tester.pageBack();
    await settle(tester);
    await tapText(tester, 'Privacy');
    expect(find.textContaining('Your photos stay on your phone'), findsOneWidget);
  });
}

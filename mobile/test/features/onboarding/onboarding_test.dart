import 'package:agrismart/app/app.dart';
import 'package:agrismart/app/router.dart';
import 'package:agrismart/app/shell.dart';
import 'package:agrismart/core/models/offline_model.dart';
import 'package:agrismart/features/onboarding/auth_repository.dart';
import 'package:agrismart/features/onboarding/slides_screen.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../../flows/first_run.dart';
import '../../support/fakes.dart';

Future<TestKit> open(WidgetTester tester, String location, {TestKit? kit}) async {
  kit ??= (await tester.runAsync(() => TestKit.create()))!;
  kit.stubAuth();
  await kit.pump(tester, location: location);
  await settle(tester);
  return kit;
}

String errorText(WidgetTester tester) => tester.widget<Text>(find.byKey(const Key('frame.error'))).data!;

void main() {
  group('normalizePhone', () {
    test('mirrors the server rules', () {
      expect(normalizePhone('017-1234 5678'), '+8801712345678');
      expect(normalizePhone('8801712345678'), '+8801712345678');
      expect(normalizePhone('+14155550100'), '+14155550100');
      expect(normalizePhone('12345'), isNull);
      expect(normalizePhone('01212345678'), isNull);
    });
  });

  group('AuthRepository', () {
    test('otp challenge, cached profile and session state', () async {
      final kit = await TestKit.create();
      kit.stubAuth();
      final auth = kit.services.auth;
      expect(await auth.cachedProfile(), isNull);
      expect(await auth.signedIn, isFalse);
      kit.server.json('POST', '/v1/auth/otp/request', {'expires_at': '2026-03-01T06:05:00Z', 'dev_code': '123456'}, 202);
      final c = await auth.requestOtp('+8801712345678');
      expect((c.phone, c.devCode, c.expiresAt), ('+8801712345678', '123456', DateTime.utc(2026, 3, 1, 6, 5)));
      final p = await auth.verifyOtp('+8801712345678', '123456');
      expect(p.role, 'farmer');
      expect(await auth.signedIn, isTrue);
      expect((await auth.cachedProfile())!.id, p.id);
      final updated = await auth.updateProfile(district: 'Rajshahi');
      expect(kit.server.seen.last.body, {'district': 'Rajshahi'});
      expect(updated.district, 'Rajshahi');
      final sparse = Profile.fromJson({'id': 'x', 'role': 'guest'});
      expect((sparse.name, sparse.district, sparse.language), ('', '', 'bn'));
    });
  });

  group('OfflineModelManager', () {
    test('load, download, version check and remove', () async {
      final kit = await TestKit.create();
      final models = kit.services.models;
      expect(models.manifest.sizeLabel, '24 MB');
      await models.load();
      expect(models.status, ModelStatus.absent);
      await models.download();
      expect((models.status, models.progress), (ModelStatus.ready, 1.0));
      final again = OfflineModelManager(db: kit.services.db);
      await again.load();
      expect(again.status, ModelStatus.ready);
      await kit.services.db.write('model.offline', {'version': 'old'});
      final stale = OfflineModelManager(db: kit.services.db);
      await stale.load();
      expect(stale.status, ModelStatus.absent);
      await models.remove();
      expect((models.status, models.progress), (ModelStatus.absent, 0.0));
    });
  });

  testWidgets('splash sends returning users straight home', (tester) async {
    final kit = (await tester.runAsync(() => TestKit.create()))!;
    await tester.runAsync(kit.onboarded);
    await tester.pumpWidget(AgriSmartApp(services: kit.services, router: buildRouter()));
    expect(find.text('Preparing your farm assistant…'), findsOneWidget);
    await settle(tester);
    expect(find.byType(Shell), findsOneWidget);
  });

  testWidgets('language picker lists all seven languages in their own script', (tester) async {
    await open(tester, '/onboarding/language');
    for (final name in ['বাংলা', 'English', 'हिन्दी', 'Español', 'Français', 'العربية', 'Português']) {
      expect(find.text(name), findsOneWidget);
    }
    expect(find.byIcon(Icons.check_circle), findsOneWidget);
    await tester.tap(find.byKey(const Key('lang.ar')));
    await settle(tester);
    expect(find.text('اختر لغتك'), findsOneWidget);
    expect(Directionality.of(tester.element(find.byType(ListView))), TextDirection.rtl);
  });

  testWidgets('slides page through and can be skipped', (tester) async {
    await open(tester, '/onboarding/slides');
    expect(find.text('Scan a sick leaf'), findsOneWidget);
    await tapText(tester, 'Next');
    expect(find.text('Get a treatment plan'), findsOneWidget);
    await tapText(tester, 'Next');
    expect(find.text('Continue'), findsOneWidget);
    expect(SlidesScreen.slides, hasLength(3));
    await tapText(tester, 'Skip');
    expect(find.text('Enter your mobile number'), findsOneWidget);
  });

  testWidgets('phone: invalid number, server error, guest path', (tester) async {
    final kit = await open(tester, '/onboarding/phone');
    await tester.enterText(find.byKey(const Key('phone.input')), '123');
    await tapText(tester, 'Send code');
    expect(errorText(tester), 'Enter a valid mobile number.');

    kit.server.json('POST', '/v1/auth/otp/request',
        {'error': {'code': 'auth.otp_rate_limited', 'message': 'errors.auth.otp_rate_limited'}}, 429);
    await tester.enterText(find.byKey(const Key('phone.input')), '01712345678');
    await tapText(tester, 'Send code');
    expect(errorText(tester), 'Please wait before requesting another code.');

    kit.server.json('POST', '/v1/auth/guest', {'error': {'code': 'x', 'message': 'errors.unknown.key'}}, 403);
    await tapText(tester, 'Continue as guest');
    expect(errorText(tester), 'Something went wrong. Please try again.');

    kit.server.json('POST', '/v1/auth/guest', TestKit.session('guest'), 201);
    await tapText(tester, 'Continue as guest');
    expect(find.text('Allow access'), findsOneWidget);
    expect((await kit.tokens.read())!.isGuest, isTrue);
  });

  testWidgets('otp: wrong code, resend, verify enabled only at six digits', (tester) async {
    final kit = await open(tester, '/onboarding/otp?phone=%2B8801712345678');
    expect(find.text('Sent to +8801712345678'), findsOneWidget);
    FilledButton verify() => tester.widget<FilledButton>(find.byType(FilledButton));
    await tester.enterText(find.byKey(const Key('otp.input')), '123');
    await tester.pump();
    expect(verify().onPressed, isNull);
    await tester.enterText(find.byKey(const Key('otp.input')), '999999');
    await tester.pump();
    await tapText(tester, 'Verify');
    expect(errorText(tester), 'That code is not correct.');
    await tapText(tester, 'Resend code');
    expect(kit.server.seen.last.path, '/v1/auth/otp/request');
    expect(find.byKey(const Key('frame.error')), findsNothing);
    expect(verify().onPressed, isNull);
  });

  testWidgets('otp route without a phone still renders', (tester) async {
    await open(tester, '/onboarding/otp');
    expect(find.text('Sent to '), findsOneWidget);
  });

  testWidgets('profile: prefilled from cache, error, skip', (tester) async {
    final kit = (await tester.runAsync(() => TestKit.create()))!;
    await tester.runAsync(() => kit.services.db.write('profile', {...TestKit.user('farmer'), 'name': 'Karim'}));
    await open(tester, '/onboarding/profile', kit: kit);
    expect(find.text('Karim'), findsOneWidget);
    kit.server.json('PATCH', '/v1/me', {'error': {'code': 'user.invalid_profile', 'message': 'errors.user.invalid_profile'}}, 400);
    await tapText(tester, 'Save');
    expect(errorText(tester), 'Please check your profile details.');
    await tapText(tester, 'Skip');
    expect(find.text('Allow access'), findsOneWidget);
  });

  testWidgets('permissions primer explains all three and records it', (tester) async {
    final kit = await open(tester, '/onboarding/permissions');
    expect(find.textContaining('Camera'), findsOneWidget);
    expect(find.textContaining('Location'), findsOneWidget);
    expect(find.textContaining('Microphone'), findsOneWidget);
    await tapText(tester, 'Allow');
    expect(find.text('Download the offline doctor'), findsOneWidget);
    expect(await tester.runAsync(() => kit.services.onboarding.permissionsPrimed), isTrue);
  });

  testWidgets('model download shows progress, then continues home', (tester) async {
    final source = ControlledModelSource();
    final kit = (await tester.runAsync(() => TestKit.create(modelSource: source)))!;
    await open(tester, '/onboarding/model', kit: kit);
    expect(find.text('Diagnose crops without internet. About 24 MB.'), findsOneWidget);
    await tapText(tester, 'Download');
    source.controller.add(0.5);
    await settle(tester);
    expect(find.text('Downloading… 50%'), findsOneWidget);
    expect(tester.widget<FilledButton>(find.byType(FilledButton)).onPressed, isNull);
    await source.controller.close();
    await settle(tester);
    expect(find.text('Ready to use offline'), findsOneWidget);
    expect(find.text('Skip'), findsNothing);
    await tapText(tester, 'Continue');
    expect(find.byType(Shell), findsOneWidget);
  });

  testWidgets('model download can be skipped', (tester) async {
    final kit = await open(tester, '/onboarding/model');
    await tapText(tester, 'Skip');
    expect(find.byType(Shell), findsOneWidget);
    expect(await tester.runAsync(() => kit.services.onboarding.completed), isTrue);
  });
}

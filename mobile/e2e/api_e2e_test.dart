// End-to-end: the Flutter app's own client code against the real Go API.
// Run with scripts/e2e.sh (starts Postgres + API); skipped without E2E_API.
import 'dart:io';

import 'package:agrismart/app/services.dart';
import 'package:agrismart/core/clock.dart';
import 'package:agrismart/core/l10n/localization.dart';
import 'package:agrismart/core/models/offline_model.dart';
import 'package:agrismart/core/network/api_client.dart';
import 'package:agrismart/core/network/connectivity.dart';
import 'package:agrismart/core/storage/local_db.dart';
import 'package:agrismart/core/sync/sync_queue.dart';
import 'package:agrismart/features/doctor/doctor_repository.dart';
import 'package:agrismart/features/onboarding/auth_repository.dart';
import 'package:agrismart/features/voice/voice_controller.dart';
import 'package:connectivity_plus_platform_interface/connectivity_plus_platform_interface.dart';
import 'package:drift/native.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import '../test/support/fakes.dart';

const api = String.fromEnvironment('E2E_API');

Future<AppServices> services({String lang = 'en'}) async {
  ConnectivityPlatform.instance = FakeConnectivity();
  final tokens = MemoryTokenStore();
  final db = LocalDb(NativeDatabase.memory());
  final client = ApiClient(baseUrl: api, tokens: tokens);
  final l10n = LocalizationRepository(bundle: rootBundle, db: db, api: client, strict: true);
  await db.write('i18n.lang', lang);
  await l10n.init();
  final ids = UuidV7Generator();
  const clock = SystemClock();
  return AppServices(
    clock: clock,
    ids: ids,
    db: db,
    tokens: tokens,
    api: client,
    connectivity: ConnectivityService(),
    l10n: l10n,
    sync: SyncQueue(db: db, api: client, ids: ids, clock: clock),
    auth: AuthRepository(api: client, tokens: tokens, db: db),
    models: OfflineModelManager(db: db),
    engine: ScriptedEngine([diagnosisOf('brown_spot', 0.82)]),
  );
}

var _phone = 1712000000 + (DateTime.now().millisecondsSinceEpoch % 900000);

/// Signs a new farmer in through the real OTP flow (dev code exposed in test env).
Future<AppServices> farmer() async {
  final s = await services();
  final phone = '0${_phone++}';
  final challenge = await s.auth.requestOtp(normalizePhone(phone)!);
  expect(challenge.devCode, isNotNull, reason: 'the E2E server must run with features.expose_otp');
  await s.auth.verifyOtp(challenge.phone, challenge.devCode!);
  return s;
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  HttpOverrides.global = null; // real sockets: this suite talks to a live server
  final skip = api.isEmpty ? 'set E2E_API (scripts/e2e.sh)' : null;

  group('against the live API', skip: skip, () {
    test('dictionary delta sync in every language', () async {
      for (final lang in languages) {
        final s = await services(lang: lang.code);
        expect(await s.l10n.sync(), isTrue, reason: lang.code);
        expect(s.l10n.current.version, greaterThan(0));
        expect(s.l10n.t('home.nav.home'), isNotEmpty);
      }
    });

    test('phone + OTP sign-in, profile, token refresh, sign-out', () async {
      final s = await farmer();
      final p = await s.auth.updateProfile(name: 'Rahim', district: 'Bogura', language: 'bn');
      expect((p.name, p.district, p.language, p.role), ('Rahim', 'Bogura', 'bn', 'farmer'));
      final before = await s.tokens.read();
      final after = await s.api.refresh();
      expect(after!.refresh, isNot(before!.refresh));
      expect((await s.api.get('/v1/me'))['name'], 'Rahim');
      await s.auth.signOut();
      expect(await s.auth.signedIn, isFalse);
    });

    test('wrong OTP is rejected with a translatable key', () async {
      final s = await services();
      final c = await s.auth.requestOtp('+8801799999999');
      final wrong = c.devCode == '000000' ? '111111' : '000000';
      await expectLater(s.auth.verifyOtp(c.phone, wrong),
          throwsA(isA<ApiException>().having((e) => s.l10n.current.has(e.messageKey), 'known key', isTrue)));
    });

    test('guest session reads the home tab', () async {
      final s = await services();
      expect((await s.auth.continueAsGuest()).role, 'guest');
      final f = await s.home.forecast();
      expect(f.value.days, hasLength(7));
      expect(f.fromCache, isFalse);
    });

    test('offline scan → upload → sync → visible in history and saved log', () async {
      final s = await farmer();
      final scan = await s.doctor.analyse(leafPhoto(5), 'rice_boro');
      await s.doctor.setSaved(scan.id, true);
      expect(await s.doctor.syncNow(), isTrue);
      final synced = (await s.doctor.scan(scan.id))!;
      expect(synced.state, ScanState.synced);
      await s.sync.flush(); // the save made before upload follows as an annotation
      final history = await s.home.scans();
      expect(history.value.map((e) => e.id), contains(synced.serverId));
      final saved = await s.home.scans(saved: true);
      expect(saved.value.single.id, synced.serverId);
      // replaying the same queue is idempotent
      expect(await s.doctor.syncNow(), isTrue);
      expect((await s.home.scans()).value.length, history.value.length);
    });

    test('field → recommendations → crop detail → ROI → rotation → seasonal outlook', () async {
      final s = await farmer();
      final field = await s.advisor.createField(
          name: 'North plot', unit: 'bigha', valueMilli: 1500, location: (lat: 24.85, lng: 89.37),
          irrigation: 'partial', texture: 'loam', ph: 6.5);
      expect((await s.advisor.fields()).value.map((f) => f.id), contains(field.id));
      final recs = await s.advisor.recommendations(field.id);
      expect(recs.value.items, isNotEmpty);
      final top = recs.value.items.first;
      expect(s.l10n.current.has('crop.${top.cropCode}.name'), isTrue);
      final detail = await s.advisor.crop(field.id, top.cropCode);
      expect(detail.value.roi.gross.likely, greaterThan(0));
      final cheaper = await s.advisor.roi(field.id, top.cropCode, {...detail.value.roi.costs, 'labour': 0});
      expect(cheaper.net.likely, greaterThan(detail.value.roi.net.likely));
      final rotation = await s.advisor.rotation(field.id);
      expect(rotation.value.steps, isNotEmpty);
      final outlook = await s.advisor.outlook();
      expect(outlook.value.rainMm.keys, containsAll(['aus', 'aman', 'boro']));
    });

    test('alerts inbox, read receipts', () async {
      final s = await farmer();
      final inbox = await s.home.alerts();
      expect(inbox.value.unread, greaterThanOrEqualTo(0));
      await s.home.markAllRead();
      expect((await s.home.alerts()).value.unread, 0);
    });

    test('voice assistant stub answers in the user language', () async {
      final s = await farmer();
      await s.voice.ask(const Utterance(transcript: 'will it rain this week?'));
      expect(s.voice.state, VoiceState.response);
      expect(s.voice.answer!.answerKey, 'voice.answer.weather');
      expect(s.l10n.current.has(s.voice.answer!.answerKey), isTrue);
      await s.voice.ask(const Utterance(transcript: 'hello'));
      expect(s.voice.state, VoiceState.notUnderstood);
    });

    test('voice manifest is served (clips optional)', () async {
      final s = await services(lang: 'bn');
      expect(await s.clips.manifest('bn'), isA<Map<String, Object?>>());
    });
  });
}

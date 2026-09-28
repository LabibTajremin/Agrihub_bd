import 'dart:async';
import 'dart:typed_data';

import 'package:agrismart/app/app.dart';
import 'package:agrismart/app/router.dart';
import 'package:agrismart/core/network/api_client.dart';
import 'package:agrismart/core/storage/local_db.dart';
import 'package:agrismart/core/widgets/async_view.dart';
import 'package:agrismart/features/doctor/dhash.dart';
import 'package:agrismart/features/doctor/diagnosis_engine.dart';
import 'package:agrismart/features/doctor/doctor_repository.dart';
import 'package:drift/native.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../../flows/first_run.dart';
import '../../support/fakes.dart';

Future<TestKit> kitWith(WidgetTester tester, List<LeafDiagnosis> results) async {
  final kit = (await tester.runAsync(() => TestKit.create(engine: ScriptedEngine(results))))!;
  await tester.runAsync(kit.onboarded);
  return kit;
}

Future<void> open(WidgetTester tester, TestKit kit, String location) async {
  tester.view.physicalSize = const Size(1200, 3000);
  tester.view.devicePixelRatio = 1.5;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(const SizedBox());
  await tester.pumpWidget(AgriSmartApp(services: kit.services, router: buildRouter(initialLocation: location)));
  await settle(tester);
}

Future<void> shoot(WidgetTester tester) async {
  await tapText(tester, 'Take photo');
  await tapText(tester, 'Use photo');
}

void main() {
  testWidgets('offline: full scan flow with the network disabled, then sync when back', (tester) async {
    final kit = await kitWith(tester, [diagnosisOf('rice_blast', 0.91, severity: 'high')]);
    kit.server.offline = true;
    await open(tester, kit, '/doctor');
    expect(find.byKey(const Key('fake-preview')), findsOneWidget);
    expect(find.byKey(const Key('alignment-guide')), findsOneWidget);
    expect(find.text('Fit one leaf inside the frame'), findsOneWidget);

    await tester.tap(find.byKey(const Key('doctor-crop')));
    await settle(tester);
    await tester.tap(find.text('Potato').last);
    await settle(tester);

    await tapText(tester, 'Take photo');
    expect(find.text('Is the leaf clear?'), findsOneWidget);
    await tapText(tester, 'Retake');
    expect(find.byKey(const Key('alignment-guide')), findsOneWidget);
    await shoot(tester);

    expect(find.text('Rice blast'), findsOneWidget);
    expect(find.text('91%'), findsOneWidget);
    expect(tester.widget<LinearProgressIndicator>(find.byKey(const Key('confidence-meter'))).value, 0.91);
    expect(find.text('High'), findsOneWidget);
    expect(find.text('Spray tricyclazole 75 WP at 0.6 g per litre of water.'), findsOneWidget);
    expect(find.byKey(const Key('safety-note')), findsOneWidget);
    await tapText(tester, 'Organic');
    expect(find.text('Drain the field for 3–4 days and stop urea top-dressing.'), findsOneWidget);
    expect(find.byKey(const Key('pending-sync')), findsOneWidget);
    await tapText(tester, 'Save to log');
    expect(find.text('Saved'), findsOneWidget);
    expect(kit.server.seen, isEmpty); // nothing left the phone

    await open(tester, kit, '/doctor');
    expect(find.widgetWithText(Badge, '1'), findsOneWidget);
    await tester.tap(find.byKey(const Key('doctor-sync')));
    await settle(tester);
    expect(find.text('1 scans waiting'), findsOneWidget);
    expect(find.text('Potato'), findsOneWidget);
    await tapText(tester, 'Sync now');
    expect(find.byKey(const Key('sync-offline')), findsOneWidget);

    kit.server.offline = false;
    kit.stubUpload();
    await tapText(tester, 'Sync now');
    expect(find.text('Everything is synced'), findsOneWidget);
    final put = kit.server.seen.firstWhere((r) => r.method == 'PUT');
    expect(put.headers.containsKey('Authorization'), isFalse);
    final sync = kit.server.seen.firstWhere((r) => r.path == '/v1/scans/sync');
    final op = ((sync.body! as Map)['operations'] as List).single as Map;
    expect((op['kind'], op['create']['crop_code'], op['create']['lang']), ('create_scan', 'potato', 'en'));
    // the save made offline follows as an annotation on the next flush
    final queued = await tester.runAsync(() => kit.services.db.ops(pendingOnly: true));
    expect(queued!.single.kind, 'annotate_scan');
    expect(queued.single.payload['scan_id'], 'srv-1');
  });

  testWidgets('healthy result shows advice', (tester) async {
    final kit = await kitWith(tester, [diagnosisOf('healthy', 0.95)]);
    await open(tester, kit, '/doctor');
    await shoot(tester);
    expect(find.text('Your crop looks healthy'), findsOneWidget);
    expect(find.text('Keep monitoring weekly and maintain balanced fertiliser.'), findsOneWidget);
    expect(find.byKey(const Key('confidence-meter')), findsNothing);
  });

  testWidgets('low confidence escalates: retake or ask an expert', (tester) async {
    final kit = await kitWith(tester, [diagnosisOf('tungro', 0.52)]);
    await open(tester, kit, '/doctor');
    await shoot(tester);
    expect(find.text('We are not sure'), findsOneWidget);
    expect(find.text('Tungro'), findsNothing);
    await tapText(tester, 'Ask an expert');
    expect(find.text('Expert help'), findsOneWidget);
    await open(tester, kit, '/doctor');
    await shoot(tester);
    await tapText(tester, 'Retake');
    expect(find.byKey(const Key('alignment-guide')), findsOneWidget);
  });

  testWidgets('camera: none available, then retry; permission denied', (tester) async {
    final kit = await kitWith(tester, [diagnosisOf('healthy', 0.9)]);
    kit.camera.cameras = const [];
    await open(tester, kit, '/doctor');
    expect(find.byType(ErrorPanel), findsOneWidget);
    expect(tester.widget<FilledButton>(find.byType(FilledButton)).onPressed, isNull);
    kit.camera.cameras = const [
      CameraDescriptionFake.front,
    ];
    await tapText(tester, 'Try again');
    expect(find.byKey(const Key('fake-preview')), findsOneWidget);

    kit.camera.failInit = true;
    await open(tester, kit, '/doctor');
    expect(find.byType(ErrorPanel), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    await settle(tester);
    expect(kit.camera.disposed, greaterThan(0));
  });

  testWidgets('preview and analysing need a photo', (tester) async {
    final kit = await kitWith(tester, [diagnosisOf('healthy', 0.9)]);
    await open(tester, kit, '/doctor/preview');
    expect(find.byKey(const Key('alignment-guide')), findsOneWidget);
    await open(tester, kit, '/doctor/analysing');
    expect(find.byKey(const Key('alignment-guide')), findsOneWidget);
  });

  testWidgets('analysing screen shows progress text', (tester) async {
    final engine = ScriptedEngine([diagnosisOf('healthy', 0.9)])..gate = Completer<void>();
    final kit = (await tester.runAsync(() => TestKit.create(engine: engine)))!;
    await open(tester, kit, '/doctor');
    await tapText(tester, 'Take photo');
    await tester.tap(find.text('Use photo'));
    for (var i = 0; i < 5; i++) {
      await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 20)));
      await tester.pump(const Duration(milliseconds: 400));
    }
    expect(find.text('Analysing…'), findsOneWidget);
    expect(find.text('Checking the leaf for signs of disease'), findsOneWidget);
    engine.gate!.complete();
    await settle(tester);
    expect(find.text('Your crop looks healthy'), findsOneWidget);
  });

  testWidgets('sync: rejected scans are flagged; upload errors are shown', (tester) async {
    final kit = await kitWith(tester, [diagnosisOf('brown_spot', 0.8)]);
    await tester.runAsync(() => kit.services.doctor.analyse(leafPhoto(2), 'rice_boro'));
    kit.stubUpload();
    kit.server.json('POST', '/v1/media/tickets', {'error': {'code': 'media.too_large', 'message': 'errors.media.too_large'}}, 413);
    await open(tester, kit, '/doctor/sync');
    await tapText(tester, 'Sync now');
    expect(find.text('The file is too large.'), findsOneWidget);

    kit.stubUpload(outcome: 'rejected');
    await tapText(tester, 'Sync now');
    expect(find.text('Failed'), findsOneWidget);
    expect(find.byIcon(Icons.error_outline), findsOneWidget);
  });

  testWidgets('result screen for a synced scan hides the pending notice; unsave queues an annotation', (tester) async {
    final kit = await kitWith(tester, [diagnosisOf('brown_spot', 0.8)]);
    final s = (await tester.runAsync(() async {
      final s = await kit.services.doctor.analyse(leafPhoto(3), 'rice_boro');
      kit.stubUpload();
      await kit.services.doctor.syncNow();
      await kit.services.doctor.setSaved(s.id, true);
      return s;
    }))!;
    await open(tester, kit, '/doctor/result/${s.id}');
    expect(find.byKey(const Key('pending-sync')), findsNothing);
    await tapText(tester, 'Saved');
    expect(find.text('Save to log'), findsOneWidget);
    final ops = await tester.runAsync(() => kit.services.db.ops(pendingOnly: true));
    expect([for (final o in ops!) o.payload['annotation']], [
      {'saved': true},
      {'saved': false},
    ]);
  });

  group('repository', () {
    test('dHash cache: a near-identical photo of the same crop skips the model', () async {
      final engine = ScriptedEngine([diagnosisOf('brown_spot', 0.8)]);
      final kit = await TestKit.create(engine: engine);
      final doctor = kit.services.doctor;
      await doctor.analyse(leafPhoto(4), 'rice_boro');
      await doctor.analyse(leafPhoto(4), 'rice_boro');
      expect(engine.calls, 1);
      await doctor.analyse(leafPhoto(4), 'wheat');
      await doctor.analyse(leafPhoto(9), 'rice_boro');
      expect(engine.calls, 3);
      await doctor.analyse(Uint8List.fromList([1, 2, 3]), 'rice_boro'); // not an image: no hash
      expect(engine.calls, 4);
      expect((await doctor.scans()).length, 5);
      expect(doctor.waiting, 5);
      expect(await doctor.photoOf((await doctor.scans()).first.id), [1, 2, 3]);
    });

    test('cache keeps the newest entries only', () async {
      final kit = await TestKit.create(engine: ScriptedEngine([diagnosisOf('brown_spot', 0.8)]));
      await kit.services.db.write('leafcache', [
        for (var i = 0; i < DoctorRepository.cacheSize; i++) {'phash': 'ffffffffffffff${i.toRadixString(16).padLeft(2, '0')}', 'crop': 'x'},
      ]);
      await kit.services.doctor.analyse(leafPhoto(5), 'rice_boro');
      final cache = await kit.services.db.read('leafcache') as List;
      expect(cache, hasLength(DoctorRepository.cacheSize));
    });

    test('offline sync reports false; unknown upload failures propagate', () async {
      final kit = await TestKit.create(engine: ScriptedEngine([diagnosisOf('brown_spot', 0.8)]));
      await kit.services.doctor.analyse(leafPhoto(6), 'rice_boro');
      kit.server.offline = true;
      expect(await kit.services.doctor.syncNow(), isFalse);
      kit.server.offline = false;
      kit.stubUpload();
      kit.server.json('PUT', 'http://blob.test/put/1', null, 500);
      await expectLater(kit.services.doctor.syncNow(), throwsA(isA<ApiException>()));
      kit.server.offline = true;
      await expectLater(kit.services.api.upload('PUT', 'http://blob.test/x', [1], const {}),
          throwsA(isA<ApiException>().having((e) => e.isOffline, 'offline', isTrue)));
    });

    test('flush left pending while the server is unreachable keeps scans queued', () async {
      final kit = await TestKit.create(engine: ScriptedEngine([diagnosisOf('brown_spot', 0.8)]));
      await kit.services.doctor.analyse(leafPhoto(7), 'rice_boro');
      kit.stubUpload();
      kit.server.on('POST', '/v1/scans/sync', (_) => const Reply(0));
      expect(await kit.services.doctor.syncNow(), isFalse);
      expect((await kit.services.doctor.unsynced()).single.state, ScanState.queued);
    });
  });

  group('engine and hashing', () {
    test('stub engine is deterministic and routes by confidence', () async {
      const e = StubDiagnosisEngine();
      final a = await e.diagnose(leafPhoto(1), 'rice_boro');
      final b = await e.diagnose(leafPhoto(1), 'rice_boro');
      expect(a.toJson(), b.toJson());
      expect(a.modelVersion, 'stub-0');
      expect(a.confidence, inInclusiveRange(0.5, 0.99));
      final seen = <String>{};
      for (var i = 0; i < 40; i++) {
        seen.add((await e.diagnose(Uint8List.fromList([i, i * 3, i * 7]), 'x')).diseaseCode);
      }
      expect(seen, containsAll(StubDiagnosisEngine.diseases));
      final healthy = diagnosisOf('healthy', 0.9, severity: 'high');
      expect((healthy.healthy, healthy.severity, healthy.plan('chemical')), (true, 'low', null));
      final round = LeafDiagnosis.fromJson(diagnosisOf('tungro', 0.6).toJson());
      expect((round.confident, round.plan('chemical')!.safetyKey), (false, 'treatment.safety'));
      expect(confidenceThreshold, 0.7);
    });

    test('dHash: stable, small distance for re-encodes, null for non-images', () {
      final h = dHash(leafPhoto(1))!;
      expect(h, hasLength(16));
      expect(dHash(leafPhoto(1)), h);
      expect(hamming(h, h), 0);
      expect(hamming('0000000000000000', 'ffffffffffffffff'), 64);
      expect(hamming(h, dHash(leafPhoto(8))!), greaterThan(DoctorRepository.cacheDistance));
      expect(dHash(Uint8List.fromList([0, 1])), isNull);
    });

    test('local store upgrades from schema 1 by adding the blob table', () async {
      final db = LocalDb(NativeDatabase.memory(setup: (raw) {
        raw.execute('CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT NOT NULL)');
        raw.userVersion = 1;
      }));
      await db.putBlob('b', Uint8List.fromList([9]));
      expect(await db.blob('b'), [9]);
      await db.removeBlob('b');
      expect(await db.blob('b'), isNull);
      await db.close();
    });
  });
}

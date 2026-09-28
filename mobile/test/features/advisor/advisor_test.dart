import 'package:agrismart/app/app.dart';
import 'package:agrismart/app/router.dart';
import 'package:agrismart/core/audio/voice.dart';
import 'package:agrismart/core/widgets/async_view.dart';
import 'package:agrismart/core/widgets/chart.dart';
import 'package:agrismart/features/advisor/models.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator_platform_interface/geolocator_platform_interface.dart';

import '../../flows/first_run.dart';
import '../../support/fakes.dart';

Future<TestKit> kitFor(WidgetTester tester, {String lang = 'en'}) async {
  final kit = (await tester.runAsync(() => TestKit.create(lang: lang)))!;
  await tester.runAsync(kit.onboarded);
  kit.stubAdvisor();
  kit.stubHome();
  return kit;
}

Future<void> open(WidgetTester tester, TestKit kit, String location) async {
  tester.view.physicalSize = const Size(1200, 5000);
  tester.view.devicePixelRatio = 1.5;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(const SizedBox());
  await tester.pumpWidget(AgriSmartApp(services: kit.services, router: buildRouter(initialLocation: location)));
  await settle(tester);
}

/// Every chart on screen carries exactly one narration control.
void expectNarrated(WidgetTester tester, int charts) {
  expect(find.byType(BarChart), findsNWidgets(charts));
  for (final e in find.byType(BarChart).evaluate()) {
    expect(find.descendant(of: find.byWidget(e.widget), matching: find.byType(NarrationControl)), findsOneWidget);
  }
}

const chartScreens = {
  '/advisor/season': 'forecast',
  '/advisor/field/f1': 'scores',
  '/advisor/field/f1/crop/wheat': 'yield',
  '/advisor/field/f1/crop/wheat/roi': 'roi',
  '/advisor/field/f1/rotation': 'rotation',
  '/home/weather': 'weather',
};

void main() {
  testWidgets('every chart widget has a narration control', (tester) async {
    final kit = await kitFor(tester);
    for (final MapEntry(key: path, value: kind) in chartScreens.entries) {
      await open(tester, kit, path);
      expectNarrated(tester, 1);
      expect(find.byKey(Key('narrate-$kind')), findsOneWidget, reason: path);
    }
  });

  testWidgets('fields list, empty state, navigation to advice', (tester) async {
    final kit = await kitFor(tester);
    await open(tester, kit, '/advisor');
    expect(find.text('North plot'), findsOneWidget);
    expect(find.text('1.5 bigha · Partly irrigated'), findsOneWidget);
    await tapText(tester, 'North plot');
    expect(find.text('Best crops for this field'), findsWidgets);
    expect(find.text('Boro (dry winter) · 180 mm'), findsOneWidget);
    expect(find.text('Suitability 86/100'), findsOneWidget);
    expect(find.text('okra'), findsWidgets); // unknown crop code falls back

    kit.server.json('GET', '/v1/fields', {'fields': <Object>[]});
    await open(tester, kit, '/advisor');
    expect(find.text('Add your first field to get crop advice.'), findsOneWidget);
    await tapText(tester, 'Season outlook');
    expect(find.text('Aman (monsoon)'), findsOneWidget);
    expect(find.text('1400 mm'), findsOneWidget);
  });

  testWidgets('field setup with GPS pin creates a field', (tester) async {
    final kit = await kitFor(tester);
    await open(tester, kit, '/advisor');
    await tapText(tester, 'Add field');
    expect(find.text('Field details'), findsOneWidget);

    await tapText(tester, 'Save');
    expect(find.text('Please check the field details.'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('setup-name')), 'South plot');
    await tapText(tester, 'Save');
    expect(find.text('Enter a valid area.'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('setup-area')), '2.5');
    await tapText(tester, 'Save');
    expect(find.text('That location is not valid.'), findsOneWidget);

    for (final (key, option) in [('setup-unit', 'acre'), ('setup-irrigation', 'Rain-fed'), ('setup-soil', 'Clay loam')]) {
      await tester.tap(find.byKey(Key(key)));
      await settle(tester);
      await tester.tap(find.text(option).last);
      await settle(tester);
    }

    await tester.tap(find.byKey(const Key('setup-location')));
    await settle(tester);
    expect(find.text('23.81000'), findsOneWidget); // prefilled with the home location
    kit.geo.enabled = false;
    await tapText(tester, 'Use my location');
    expect(find.text('That location is not valid.'), findsOneWidget);
    kit.geo.enabled = true;
    kit.geo.onRequest = LocationPermission.deniedForever;
    await tapText(tester, 'Use my location');
    expect(find.text('That location is not valid.'), findsOneWidget);
    kit.geo.permission = LocationPermission.whileInUse;
    await tapText(tester, 'Use my location');
    expect(find.text('24.84810'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('gps-lat')), '95');
    await tapText(tester, 'Done');
    expect(find.text('That location is not valid.'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('gps-lat')), '24.9');
    await tapText(tester, 'Done');
    expect(find.text('24.9000, 89.3730'), findsOneWidget);

    kit.server.json('GET', '/v1/advisory/fields/f2/recommendations',
        {'season': 'aus', 'outlook': {'rain_mm': 600}, 'items': <Object>[]});
    await tapText(tester, 'Save');
    final post = kit.server.seen.lastWhere((r) => r.method == 'POST' && r.path == '/v1/fields').body! as Map;
    expect(post['area'], {'value_milli': 2500, 'unit': 'acre'});
    expect((post['irrigation'], (post['soil'] as Map)['texture']), ('none', 'clay_loam'));
    expect(post['location'], {'lat': 24.9, 'lng': 89.373});
    expect(find.text('Aus (pre-monsoon) · 600 mm'), findsOneWidget);
    expect(await tester.runAsync(kit.services.home.location), (lat: 24.9, lng: 89.373));
  });

  testWidgets('field setup surfaces server errors', (tester) async {
    final kit = await kitFor(tester);
    kit.server.json('POST', '/v1/fields', {'error': {'code': 'farm.invalid_area', 'message': 'errors.farm.invalid_area'}}, 400);
    await open(tester, kit, '/advisor/new');
    await tester.enterText(find.byKey(const Key('setup-name')), 'X');
    await tester.enterText(find.byKey(const Key('setup-area')), '1');
    await tester.tap(find.byKey(const Key('setup-location')));
    await settle(tester);
    await tapText(tester, 'Done');
    await tapText(tester, 'Save');
    expect(find.text('Enter a valid area.'), findsOneWidget);
  });

  testWidgets('crop detail, ROI recalculation and rotation', (tester) async {
    final kit = await kitFor(tester);
    await open(tester, kit, '/advisor/field/f1');
    await tapText(tester, 'Wheat');
    expect(find.text('Crop details'), findsOneWidget);
    expect(find.text('4200 kg'), findsOneWidget);
    expect(find.text('৳51,000'), findsOneWidget);
    expect(find.text('Return 94%'), findsOneWidget);
    await tapText(tester, 'Profit estimate');
    expect(find.widgetWithText(TextField, '8000'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('cost-seed')), '10000');
    await tapText(tester, 'Save');
    final body = kit.server.seen.lastWhere((r) => r.path.endsWith('/roi')).body! as Map;
    expect((body['costs_poisha'] as Map)['seed'], 1000000);
    expect(find.text('৳49,000'), findsOneWidget);

    kit.server.json('POST', '/v1/advisory/fields/f1/crops/wheat/roi',
        {'error': {'code': 'advisory.invalid_input', 'message': 'errors.advisory.invalid_input'}}, 400);
    await tapText(tester, 'Save');
    expect(find.text('Please check the values you entered.'), findsOneWidget);
    kit.server.json('POST', '/v1/advisory/fields/f1/crops/wheat/roi',
        {'error': {'code': 'x', 'message': 'errors.nope'}}, 400);
    await tapText(tester, 'Save');
    expect(find.text('Something went wrong. Please try again.'), findsOneWidget);

    await open(tester, kit, '/advisor/field/f1');
    await tapText(tester, 'Rotation plan');
    expect(find.byKey(const Key('rotation-deficit')), findsOneWidget);
    expect(find.text('Mung bean'), findsOneWidget);
    expect(find.text('Aus (pre-monsoon) · N 20 → 55 kg/ha'), findsOneWidget);
  });

  testWidgets('rotation without deficit; advice offline from cache', (tester) async {
    final kit = await kitFor(tester);
    kit.server.json('GET', '/v1/advisory/fields/f1/rotation', {'steps': <Object>[], 'nitrogen_deficit': false});
    await open(tester, kit, '/advisor/field/f1/rotation');
    expect(find.byKey(const Key('rotation-deficit')), findsNothing);
    await open(tester, kit, '/advisor/field/f1');
    kit.server.offline = true;
    await open(tester, kit, '/advisor/field/f1');
    expect(find.byKey(const Key('offline.banner')), findsOneWidget);
    await open(tester, kit, '/advisor');
    // opening a nested route built (and cached) the field list beneath it
    expect(find.byKey(const Key('offline.banner')), findsOneWidget);
    expect(find.byType(ErrorPanel), findsNothing);
  });

  testWidgets('narration plays the pre-recorded clip, stops, and reports a missing clip', (tester) async {
    final kit = await kitFor(tester);
    kit.stubVoice('en', ['narration.chart.scores']);
    await open(tester, kit, '/advisor/field/f1');
    await tester.tap(find.byKey(const Key('narrate-scores')));
    await settle(tester);
    expect(kit.audio.played, hasLength(1));
    expect(find.text('Stop'), findsOneWidget);
    kit.audio.finish();
    await settle(tester);
    expect(find.text('Listen'), findsOneWidget);

    await tester.tap(find.byKey(const Key('narrate-scores')));
    await settle(tester);
    await tester.tap(find.byKey(const Key('narrate-scores')));
    await settle(tester);
    expect(find.text('Listen'), findsOneWidget);
    expect(kit.audio.stops, greaterThan(0));
    expect(kit.audio.played, hasLength(2));

    await open(tester, kit, '/advisor/season');
    await tester.tap(find.byKey(const Key('narrate-forecast')));
    await settle(tester);
    expect(find.text('Audio not available yet'), findsOneWidget);
  });

  test('voice clips: cached by checksum, verified, offline-safe', () async {
    final kit = await TestKit.create();
    final voice = kit.services.voice;
    kit.stubVoice('bn', ['a.b']);
    final first = await voice.clip('bn', 'a.b');
    expect(first, isNotNull);
    kit.server.routes.remove('GET http://cdn.test/bn/a.b.mp3');
    expect(await voice.clip('bn', 'a.b'), first); // from the blob cache
    expect(await voice.clip('bn', 'missing.key'), isNull);
    kit.stubVoice('pt', ['c.d']);
    kit.server.routes.remove('GET http://cdn.test/pt/c.d.mp3');
    expect(await voice.clip('pt', 'c.d'), isNull); // clip unreachable, not cached
    kit.stubVoice('fr', ['a.b'], corrupt: true);
    expect(await voice.clip('fr', 'a.b'), isNull);
    kit.server.offline = true;
    expect(await voice.clip('hi', 'a.b'), isNull);
    kit.server.json('GET', '/v1/voice/es', {'lang': 'es', 'clips': null});
    kit.server.offline = false;
    expect(await voice.manifest('es'), isEmpty);
  });

  test('narration controller disposes its completion listener', () async {
    final kit = await TestKit.create();
    final c = NarrationController(voice: kit.services.voice, out: PluginAudioOut(), language: () => 'en');
    await c.stop();
    c.dispose();
  });

  test('money and area formatting', () {
    expect(takaDigits(123456789), '1,234,568');
    expect(takaDigits(-150000), '-1,500');
    expect(takaDigits(0), '0');
    final f = FieldInfo.fromJson({...TestKit.fieldJson('x', 'n'), 'soil': null});
    expect((f.texture, f.ph), (null, null));
  });
}

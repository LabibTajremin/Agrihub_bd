import 'package:agrismart/app/shell.dart';
import 'package:agrismart/core/network/api_client.dart';
import 'package:agrismart/core/widgets/async_view.dart';
import 'package:agrismart/core/widgets/data_age.dart';
import 'package:agrismart/features/home/alerts_screen.dart';
import 'package:agrismart/features/home/home_repository.dart';
import 'package:agrismart/features/home/models.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../../flows/first_run.dart';
import '../../support/fakes.dart';

Future<TestKit> kitFor(WidgetTester tester, {String lang = 'en'}) async {
  final kit = (await tester.runAsync(() => TestKit.create(lang: lang)))!;
  await tester.runAsync(kit.onboarded);
  return kit;
}

Future<void> open(WidgetTester tester, TestKit kit, String location) async {
  tester.view.physicalSize = const Size(1200, 4000);
  tester.view.devicePixelRatio = 1.5;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(const SizedBox()); // a fresh app (and router) per location
  await kit.pump(tester, location: location);
  await settle(tester);
}

void main() {
  testWidgets('dashboard: greeting, weather, unread badge, recent scans, navigation', (tester) async {
    final kit = await kitFor(tester);
    kit.stubHome();
    await tester.runAsync(() => kit.services.db.write('profile', {...TestKit.user('farmer'), 'name': 'Rahim'}));
    await open(tester, kit, '/home');
    expect(find.text('Hello, Rahim'), findsOneWidget);
    expect(find.text('18–30°C · Rainfall 12 mm'), findsOneWidget);
    expect(find.widgetWithText(Badge, '1'), findsOneWidget);
    expect(find.text('Brown spot'), findsOneWidget);
    expect(find.text('Potato'), findsOneWidget);
    expect(find.text('Jute'), findsNothing); // only three recent scans
    expect(find.byKey(const Key('offline.banner')), findsNothing);

    for (final (label, expected) in [
      ('Today\'s weather', 'Next days'),
      ('Saved log', 'Brown spot'),
      ('See all', 'Jute'),
    ]) {
      await tapText(tester, label);
      expect(find.text(expected), findsWidgets);
      await tester.pageBack();
      await settle(tester);
    }
    for (final label in ['Scan a leaf', 'Crop advisor', 'Ask by voice']) {
      await tapText(tester, label);
      expect(find.byType(Shell), findsOneWidget);
      await tapText(tester, 'Home');
    }
    await tester.fling(find.byType(ListView).first, const Offset(0, 400), 1000);
    await settle(tester);
    await tapText(tester, 'Alerts');
    expect(find.text('Heavy rain expected'), findsOneWidget);
  });

  testWidgets('dashboard: guest greeting and sections that fail stay quiet', (tester) async {
    final kit = await kitFor(tester);
    await open(tester, kit, '/home');
    expect(find.text('Hello, farmer'), findsOneWidget);
    expect(find.text('Weather data may be out of date'), findsOneWidget);
    expect(find.text('No scans yet. Scan your first leaf.'), findsOneWidget);
  });

  testWidgets('offline variant: network disabled, cached data shown with its age', (tester) async {
    final kit = await kitFor(tester);
    kit.stubHome();
    await open(tester, kit, '/home');
    expect(find.byKey(const Key('offline.banner')), findsNothing);

    kit.server.offline = true;
    kit.clock.advance(const Duration(hours: 2));
    await open(tester, kit, '/home');
    expect(find.byKey(const Key('offline.banner')), findsOneWidget);
    expect(find.text('You are offline. Showing saved data. Updated 2 h ago'), findsOneWidget);
    expect(find.text('Brown spot'), findsOneWidget);
    expect(kit.services.connectivity.online, isFalse);
  });

  testWidgets('weather detail: seven days, stale notice, offline error and retry', (tester) async {
    final kit = await kitFor(tester);
    kit.stubHome(stale: true);
    await open(tester, kit, '/home/weather');
    expect(find.textContaining('Humidity 70%'), findsNWidgets(7));
    expect(find.byKey(const Key('weather.stale')), findsOneWidget);

    final fresh = await kitFor(tester);
    fresh.server.offline = true;
    await open(tester, fresh, '/home/weather');
    expect(find.byType(ErrorPanel), findsOneWidget);
    expect(find.text('You are offline. Showing saved data.'), findsOneWidget);
    fresh.server.offline = false;
    fresh.stubHome();
    await tapText(tester, 'Try again');
    expect(find.byType(ErrorPanel), findsNothing);
    expect(find.byKey(const Key('weather.stale')), findsNothing);
  });

  testWidgets('alerts: inbox, detail marks read with translated params, mark all', (tester) async {
    final kit = await kitFor(tester);
    kit.stubHome();
    await open(tester, kit, '/home/alerts');
    expect(find.text('Warning'), findsOneWidget);
    expect(find.text('Critical'), findsOneWidget);
    await tapText(tester, 'Check your treated field');
    expect(find.text('It is time to re-scan for Brown spot and repeat treatment if needed.'), findsOneWidget);
    expect(kit.server.seen.any((r) => r.method == 'POST' && r.path == '/v1/alerts/a2/read'), isTrue);
    await tester.pageBack();
    await settle(tester);
    await tapText(tester, 'Mark all as read');
    expect(kit.server.seen.any((r) => r.path == '/v1/alerts/read-all'), isTrue);
  });

  testWidgets('alerts: empty inbox and offline detail from cache', (tester) async {
    final kit = await kitFor(tester);
    kit.server.json('GET', '/v1/alerts', {'alerts': <Object>[], 'unread_count': 0});
    await open(tester, kit, '/home/alerts');
    expect(find.text('No alerts right now'), findsOneWidget);
    expect(find.text('Mark all as read'), findsNothing);

    kit.stubHome();
    await open(tester, kit, '/home/alerts/a1');
    expect(find.text('About 60 mm of rain is expected. Clear drainage channels and delay spraying.'), findsOneWidget);
    await open(tester, kit, '/home/alerts');
    kit.server.offline = true;
    await open(tester, kit, '/home/alerts/a1');
    expect(find.byKey(const Key('offline.banner')), findsOneWidget);
    await open(tester, kit, '/home/alerts');
    expect(find.byKey(const Key('offline.banner')), findsOneWidget);
    expect(find.text('Mark all as read'), findsNothing);
  });

  testWidgets('alert detail: a failed read receipt surfaces as an error', (tester) async {
    final kit = await kitFor(tester);
    kit.stubHome();
    kit.server.json('POST', '/v1/alerts/a1/read', {'error': {'code': 'alert.not_found', 'message': 'errors.alert.not_found'}}, 404);
    await open(tester, kit, '/home/alerts/a1');
    expect(find.text('Alert not found.'), findsOneWidget);
  });

  testWidgets('saved log empty state; history offline banner', (tester) async {
    final kit = await kitFor(tester);
    kit.stubHome(scans: [TestKit.scanJson('s9', 'okra')]);
    await open(tester, kit, '/home/saved');
    expect(find.text('Nothing here yet'), findsOneWidget);
    await open(tester, kit, '/home/history');
    expect(find.text('okra'), findsOneWidget); // unknown crop code falls back to the code
    kit.server.offline = true;
    await open(tester, kit, '/home/history');
    expect(find.byKey(const Key('offline.banner')), findsOneWidget);
  });

  testWidgets('Bangla parity: every home screen renders fully translated', (tester) async {
    final kit = await kitFor(tester, lang: 'bn');
    kit.stubHome(stale: true);
    for (final path in ['/home', '/home/weather', '/home/alerts', '/home/alerts/a2', '/home/history', '/home/saved']) {
      await open(tester, kit, path);
      expect(tester.takeException(), isNull, reason: path);
    }
    expect(find.text('Saved log'), findsNothing);
  });

  testWidgets('async view shows a spinner while loading', (tester) async {
    final kit = await kitFor(tester);
    await tester.pumpWidget(MaterialApp(
        home: AsyncView<int>(load: () => Future.delayed(const Duration(seconds: 1), () => 1), builder: (_, v, _) => Text('$v'))));
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    await tester.pump(const Duration(seconds: 1));
    expect(find.text('1'), findsOneWidget);
  });

  testWidgets('age formatting', (tester) async {
    final kit = await kitFor(tester);
    await kit.pump(tester);
    final ctx = tester.element(find.byType(Shell));
    expect(formatAge(ctx, const Duration(seconds: 5)), 'just now');
    expect(formatAge(ctx, const Duration(minutes: 5)), '5 min');
    expect(formatAge(ctx, const Duration(hours: 3)), '3 h');
    expect(formatAge(ctx, const Duration(days: 2)), '2 d');
  });

  test('severity colours and repository helpers', () async {
    expect({for (final s in ['critical', 'warning', 'info']) severityColor(s)}, hasLength(3));
    final kit = await TestKit.create();
    final home = kit.services.home;
    expect(await home.location(), defaultLocation);
    await home.setLocation((lat: 24.85, lng: 89.37));
    expect(await home.location(), (lat: 24.85, lng: 89.37));
    kit.server.json('GET', '/v1/alerts', {'error': {'code': 'auth.forbidden', 'message': 'errors.auth.forbidden'}}, 403);
    await expectLater(home.alerts(), throwsA(isA<ApiException>().having((e) => e.status, 'status', 403)));
    final a = AlertItem.fromJson({...TestKit.alertJson('x', 'heat', 'info', {}), 'params': null});
    expect(a.params, isEmpty);
  });
}

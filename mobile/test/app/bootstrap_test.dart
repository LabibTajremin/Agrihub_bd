import 'dart:async';
import 'dart:io';

import 'package:agrismart/app/app.dart';

import 'package:agrismart/app/bootstrap.dart';
import 'package:agrismart/main.dart' as app;
import 'package:flutter/widgets.dart';
import 'package:connectivity_plus_platform_interface/connectivity_plus_platform_interface.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:path_provider_platform_interface/path_provider_platform_interface.dart';
import 'package:plugin_platform_interface/plugin_platform_interface.dart';

import '../support/fakes.dart';

class FakePaths extends PathProviderPlatform with MockPlatformInterfaceMixin {
  FakePaths(this.dir);
  final String dir;
  @override
  Future<String?> getApplicationSupportPath() async => dir;
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() {
    FlutterSecureStorage.setMockInitialValues({});
    ConnectivityPlatform.instance = FakeConnectivity();
  });

  test('bootstrap wires production services', () async {
    final dir = await Directory.systemTemp.createTemp('agri');
    PathProviderPlatform.instance = FakePaths(dir.path);
    final s = await bootstrap(baseUrl: 'http://api.test');
    expect(s.l10n.current.language.code, 'bn');
    expect(s.api.dio.options.baseUrl, 'http://api.test');
    expect(s.connectivity.online, isTrue);
    expect(s.ids.next(), hasLength(36));
    expect(s.clock.now().isUtc, isTrue);
    await s.db.close();
  });

  testWidgets('main boots the app', (tester) async {
    final dir = await tester.runAsync(() => Directory.systemTemp.createTemp('agri'));
    PathProviderPlatform.instance = FakePaths(dir!.path);
    unawaited(app.main());
    // bootstrap talks to the database isolate: let real time pass, then pump.
    for (var i = 0; i < 200 && find.byType(AgriSmartApp).evaluate().isEmpty; i++) {
      await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 10)));
      await tester.pump();
    }
    final root = tester.widget<AgriSmartApp>(find.byType(AgriSmartApp));
    expect(root.services.l10n.current.language.code, 'bn');
    await tester.pumpWidget(const SizedBox());
    await tester.runAsync(root.services.db.close);
  });
}

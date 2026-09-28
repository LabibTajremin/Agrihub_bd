import 'package:agrismart/core/network/connectivity.dart';
import 'package:agrismart/core/sync/auto_sync.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/fakes.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('runs once each time the network comes back', () async {
    final c = ConnectivityService();
    var runs = 0;
    final a = AutoSync(connectivity: c, run: () async => runs++)..start();
    c.report(true); // no change
    c.report(false);
    expect(runs, 0);
    c.report(true);
    expect(runs, 1);
    c.report(false);
    c.report(true);
    expect(runs, 2);
    a.stop();
    c.report(false);
    c.report(true);
    expect(runs, 2);
  });

  test('syncAll uploads waiting scans and swallows failures', () async {
    final kit = await TestKit.create(engine: ScriptedEngine([diagnosisOf('brown_spot', 0.8)]));
    await kit.services.doctor.analyse(leafPhoto(3), 'rice_boro');
    kit.stubUpload();
    kit.server.json('POST', '/v1/media/tickets', {'error': {'code': 'media.too_large', 'message': 'm'}}, 413);
    await kit.services.syncAll(); // failure stays queued, no throw
    expect(kit.services.doctor.waiting, 1);
    kit.stubUpload();
    await kit.services.syncAll();
    expect(kit.services.doctor.waiting, 0);
  });
}

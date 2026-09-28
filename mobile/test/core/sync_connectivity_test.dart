import 'package:agrismart/core/network/connectivity.dart';
import 'package:connectivity_plus_platform_interface/connectivity_plus_platform_interface.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/fakes.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('offline queue flushes in order, drops applied, keeps rejected', () async {
    final kit = await TestKit.create();
    final q = kit.services.sync;
    expect((await q.flush()).remaining, 0);
    final k1 = await q.enqueue('create_scan', {'create': {'media_id': 'm'}});
    final k2 = await q.enqueue('annotate_scan', {'scan_id': 's'});
    expect(q.pending, 2);
    kit.server.offline = true;
    final off = await q.flush();
    expect(off.offline, isTrue);
    expect(off.remaining, 2);
    kit.server.offline = false;
    kit.server.json('POST', '/v1/scans/sync', {
      'results': [
        {'idempotency_key': k1, 'outcome': 'applied', 'scan_id': 's1'},
        {'idempotency_key': k2, 'outcome': 'rejected', 'error_code': 'scan.not_found'},
      ],
    });
    final r = await q.flush();
    expect((r.applied, r.rejected, r.remaining, r.offline), (1, 1, 0, false));
    final sent = kit.server.seen.last.body! as Map;
    expect((sent['operations'] as List).map((o) => (o as Map)['seq']), [1, 2]);
    final all = await q.all();
    expect(all.single.errorCode, 'scan.not_found');
  });

  test('connectivity follows the platform and reports', () async {
    final platform = FakeConnectivity()..current = [ConnectivityResult.none];
    ConnectivityPlatform.instance = platform;
    final c = ConnectivityService();
    var changes = 0;
    c.addListener(() => changes++);
    await c.start();
    expect(c.online, isFalse);
    platform.controller.add([ConnectivityResult.mobile]);
    await Future<void>.delayed(Duration.zero);
    expect(c.online, isTrue);
    c.report(true);
    expect(changes, 2);
    c.dispose();
  });
}

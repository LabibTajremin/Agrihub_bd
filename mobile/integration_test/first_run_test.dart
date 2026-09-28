// On-device first-run journey. Run on an emulator or phone:
//   flutter test integration_test
// The same flow runs headless in CI via test/features/onboarding/first_run_flow_test.dart.
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';

import '../test/flows/first_run.dart';
import '../test/support/fakes.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('first run: splash to home in Bangla', (tester) async {
    final kit = await tester.runAsync(() => TestKit.create());
    await runFirstRunFlow(tester, kit!);
  });
}

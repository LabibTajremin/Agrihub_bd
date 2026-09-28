import 'package:flutter_test/flutter_test.dart';

import '../../flows/first_run.dart';
import '../../support/fakes.dart';

void main() {
  testWidgets('first run: splash to home in Bangla', (tester) async {
    final kit = await tester.runAsync(() => TestKit.create());
    await runFirstRunFlow(tester, kit!);
  });
}

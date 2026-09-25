import 'package:agrismart/main.dart' as app;
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('app boots and shows the product name', (tester) async {
    app.main();
    await tester.pump();
    expect(find.text('AgriSmart'), findsOneWidget);
  });
}

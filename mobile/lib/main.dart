import 'package:flutter/widgets.dart';

import 'app/app.dart';
import 'app/bootstrap.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(AgriSmartApp(services: await bootstrap()));
}

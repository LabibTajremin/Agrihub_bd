import 'dart:io';

import 'package:drift/native.dart';
import 'package:flutter/services.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';

import '../core/clock.dart';
import '../core/l10n/localization.dart';
import '../core/models/offline_model.dart';
import '../core/network/api_client.dart';
import '../core/network/connectivity.dart';
import '../core/storage/local_db.dart';
import '../core/storage/token_store.dart';
import '../core/sync/auto_sync.dart';
import '../core/sync/sync_queue.dart';
import '../features/onboarding/auth_repository.dart';
import 'services.dart';

/// API base URL, set at build time: --dart-define=API_BASE_URL=https://…
const apiBaseUrl = String.fromEnvironment('API_BASE_URL', defaultValue: 'http://10.0.2.2:8080');

/// Builds the production services.
Future<AppServices> bootstrap({String baseUrl = apiBaseUrl}) async {
  final dir = await getApplicationSupportDirectory();
  final db = LocalDb(NativeDatabase.createInBackground(File(p.join(dir.path, 'agrismart.db'))));
  const clock = SystemClock();
  final ids = UuidV7Generator();
  final tokens = SecureTokenStore();
  final connectivity = ConnectivityService();
  final api = ApiClient(baseUrl: baseUrl, tokens: tokens);
  final l10n = LocalizationRepository(bundle: rootBundle, db: db, api: api);
  await l10n.init();
  await connectivity.start();
  final sync = SyncQueue(db: db, api: api, ids: ids, clock: clock);
  await sync.refresh();
  final models = OfflineModelManager(db: db);
  await models.load();
  final services = AppServices(
      clock: clock, ids: ids, db: db, tokens: tokens, api: api, connectivity: connectivity, l10n: l10n,
      sync: sync,
      auth: AuthRepository(api: api, tokens: tokens, db: db),
      models: models);
  AutoSync(connectivity: connectivity, run: services.syncAll).start();
  return services;
}

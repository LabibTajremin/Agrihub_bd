import 'dart:async';
import 'dart:convert';

import 'package:agrismart/app/app.dart';
import 'package:agrismart/app/router.dart';
import 'package:agrismart/app/services.dart';
import 'package:agrismart/core/clock.dart';
import 'package:agrismart/core/l10n/localization.dart';
import 'package:agrismart/core/models/offline_model.dart';
import 'package:agrismart/core/network/api_client.dart';
import 'package:agrismart/core/network/connectivity.dart';
import 'package:agrismart/core/storage/local_db.dart';
import 'package:agrismart/core/storage/token_store.dart';
import 'package:agrismart/core/sync/sync_queue.dart';
import 'package:agrismart/features/onboarding/auth_repository.dart';
import 'package:connectivity_plus_platform_interface/connectivity_plus_platform_interface.dart';
import 'package:dio/dio.dart';
import 'package:drift/native.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:plugin_platform_interface/plugin_platform_interface.dart';

/// A recorded request seen by [FakeServer].
class Seen {
  Seen(this.method, this.path, this.query, this.headers, this.body);
  final String method;
  final String path;
  final Map<String, dynamic> query;
  final Map<String, dynamic> headers;
  final Object? body;
}

/// A canned reply: status + JSON body. `status: 0` simulates no network.
class Reply {
  const Reply(this.status, [this.body]);
  final int status;
  final Object? body;
}

typedef Route = FutureOr<Reply> Function(Seen req);

/// In-memory HTTP backend for Dio: routes keyed by "METHOD /path".
class FakeServer implements HttpClientAdapter {
  final routes = <String, Route>{};
  final seen = <Seen>[];
  bool offline = false;

  void on(String method, String path, Route r) => routes['$method $path'] = r;
  void json(String method, String path, Object? body, [int status = 200]) => on(method, path, (_) => Reply(status, body));

  @override
  Future<ResponseBody> fetch(RequestOptions o, Stream<Uint8List>? requestStream, Future<void>? cancelFuture) async {
    Object? body = o.data;
    final req = Seen(o.method, o.path, o.queryParameters, o.headers, body);
    seen.add(req);
    final route = routes['${o.method} ${o.path}'];
    if (offline || route == null) {
      throw DioException.connectionError(requestOptions: o, reason: 'offline');
    }
    final r = await route(req);
    if (r.status == 0) {
      throw DioException.connectionError(requestOptions: o, reason: 'offline');
    }
    return ResponseBody.fromString(r.body == null ? '' : jsonEncode(r.body), r.status,
        headers: {Headers.contentTypeHeader: [Headers.jsonContentType], 'etag': ['"e"']});
  }

  @override
  void close({bool force = false}) {}
}

class MemoryTokenStore implements TokenStore {
  Tokens? value;
  @override
  Future<Tokens?> read() async => value;
  @override
  Future<void> write(Tokens tokens) async => value = tokens;
  @override
  Future<void> clear() async => value = null;
}

class FakeConnectivity extends ConnectivityPlatform with MockPlatformInterfaceMixin {
  final controller = StreamController<List<ConnectivityResult>>.broadcast();
  List<ConnectivityResult> current = [ConnectivityResult.wifi];
  @override
  Future<List<ConnectivityResult>> checkConnectivity() async => current;
  @override
  Stream<List<ConnectivityResult>> get onConnectivityChanged => controller.stream;
}

final epoch = DateTime.utc(2026, 3, 1, 6);

/// A model source the test drives step by step.
class ControlledModelSource implements ModelSource {
  final controller = StreamController<double>();
  @override
  ModelManifest get manifest => const StubModelSource().manifest;
  @override
  Stream<double> fetch() => controller.stream;
}

/// Everything a test needs, built from fakes.
class TestKit {
  TestKit._(this.services, this.server, this.tokens, this.clock);
  final AppServices services;
  final FakeServer server;
  final MemoryTokenStore tokens;
  final FixedClock clock;

  static Future<TestKit> create({String lang = 'en', bool strict = true, ModelSource? modelSource}) async {
    ConnectivityPlatform.instance = FakeConnectivity();
    final server = FakeServer();
    final tokens = MemoryTokenStore();
    final clock = FixedClock(epoch);
    final ids = SequenceIdGenerator();
    final db = LocalDb(NativeDatabase.memory());
    final api = ApiClient(baseUrl: 'http://api.test', tokens: tokens, adapter: server, sleep: (_) async {});
    final l10n = LocalizationRepository(bundle: rootBundle, db: db, api: api, strict: strict);
    await db.write('i18n.lang', lang);
    await l10n.init();
    final services = AppServices(
      clock: clock,
      ids: ids,
      db: db,
      tokens: tokens,
      api: api,
      connectivity: ConnectivityService(),
      l10n: l10n,
      sync: SyncQueue(db: db, api: api, ids: ids, clock: clock),
      auth: AuthRepository(api: api, tokens: tokens, db: db),
      models: OfflineModelManager(db: db, source: modelSource ?? const StubModelSource()),
    );
    return TestKit._(services, server, tokens, clock);
  }

  Future<void> signIn({String role = 'farmer'}) =>
      tokens.write(Tokens(access: 'access-1', refresh: 'refresh-1', role: role, userId: '00000000-0000-7000-8000-0000000000a1'));

  /// Marks first-run onboarding finished so launches land on the app.
  Future<void> onboarded() => services.onboarding.complete();

  /// Installs the server's auth endpoints (verify accepts [code]).
  void stubAuth({String code = '123456', String role = 'farmer'}) {
    server.json('POST', '/v1/auth/otp/request', {'expires_at': '2026-03-01T06:05:00Z'}, 202);
    server.on('POST', '/v1/auth/otp/verify', (req) {
      final body = (req.body! as Map).cast<String, Object?>();
      return body['code'] == code
          ? Reply(200, session(role))
          : const Reply(400, {'error': {'code': 'auth.otp_invalid', 'message': 'errors.auth.otp_invalid'}});
    });
    server.json('POST', '/v1/auth/guest', session('guest'), 201);
    server.on('PATCH', '/v1/me', (req) => Reply(200, {...user(role), ...(req.body! as Map).cast<String, Object?>()}));
  }

  static Map<String, Object?> user(String role) =>
      {'id': '00000000-0000-7000-8000-0000000000a1', 'name': '', 'district': '', 'language': 'en', 'role': role};

  static Map<String, Object?> session(String role) => {
        'token_type': 'Bearer',
        'access_token': 'access-$role',
        'refresh_token': 'refresh-$role',
        'user': user(role),
      };

  Future<void> pump(WidgetTester tester, {String location = '/home', GoRouter? router}) async {
    await tester.pumpWidget(AgriSmartApp(services: services, router: router ?? buildRouter(initialLocation: location)));
    await tester.pumpAndSettle();
  }
}

import 'dart:async';
import 'dart:convert';

import 'package:agrismart/app/app.dart';
import 'package:agrismart/app/router.dart';
import 'package:agrismart/app/services.dart';
import 'package:agrismart/core/clock.dart';
import 'package:agrismart/core/l10n/localization.dart';
import 'package:agrismart/core/models/offline_model.dart';
import 'package:agrismart/features/doctor/diagnosis_engine.dart';
import 'package:agrismart/core/network/api_client.dart';
import 'package:agrismart/core/network/connectivity.dart';
import 'package:agrismart/core/storage/local_db.dart';
import 'package:agrismart/core/storage/token_store.dart';
import 'package:agrismart/core/sync/sync_queue.dart';
import 'package:agrismart/features/onboarding/auth_repository.dart';
import 'package:audioplayers_platform_interface/audioplayers_platform_interface.dart';
import 'package:crypto/crypto.dart';
import 'package:geolocator_platform_interface/geolocator_platform_interface.dart';
import 'package:camera_platform_interface/camera_platform_interface.dart';
import 'package:connectivity_plus_platform_interface/connectivity_plus_platform_interface.dart';
import 'package:dio/dio.dart';
import 'package:drift/native.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:image/image.dart' as img;
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

/// A small JPEG whose content (and so dHash) depends on [seed].
Uint8List leafPhoto(int seed) {
  final image = img.Image(width: 36, height: 32);
  for (var y = 0; y < 32; y++) {
    for (var x = 0; x < 36; x++) {
      final v = ((x * (seed + 1) * 7 + y * (seed + 3) * 13) % 256);
      image.setPixelRgb(x, y, v, 255 - v, (v * seed) % 256);
    }
  }
  return img.encodeJpg(image, quality: 95);
}

/// Camera plugin fake: one back camera by default; [photo] is what it shoots.
class FakeCameraPlatform extends CameraPlatform with MockPlatformInterfaceMixin {
  List<CameraDescription> cameras = const [
    CameraDescription(name: 'front', lensDirection: CameraLensDirection.front, sensorOrientation: 0),
    CameraDescription(name: 'back', lensDirection: CameraLensDirection.back, sensorOrientation: 90),
  ];
  Uint8List photo = leafPhoto(1);
  bool failInit = false;
  int disposed = 0;

  @override
  Future<List<CameraDescription>> availableCameras() async => cameras;
  @override
  Future<int> createCameraWithSettings(CameraDescription d, MediaSettings? s) async => 7;
  @override
  Stream<CameraInitializedEvent> onCameraInitialized(int id) =>
      Stream.value(CameraInitializedEvent(id, 480, 640, ExposureMode.auto, true, FocusMode.auto, true));
  @override
  Stream<CameraErrorEvent> onCameraError(int id) => StreamController<CameraErrorEvent>().stream;
  @override
  Stream<DeviceOrientationChangedEvent> onDeviceOrientationChanged() => StreamController<DeviceOrientationChangedEvent>().stream;
  @override
  Future<void> initializeCamera(int id, {ImageFormatGroup imageFormatGroup = ImageFormatGroup.unknown}) async {
    if (failInit) {
      throw PlatformException(code: 'CameraAccessDenied');
    }
  }
  @override
  Widget buildPreview(int id) => const ColoredBox(key: Key('fake-preview'), color: Color(0xFF335533));
  @override
  Future<XFile> takePicture(int id) async => XFile.fromData(photo, mimeType: 'image/jpeg');
  @override
  Future<void> dispose(int id) async => disposed++;
}

abstract final class CameraDescriptionFake {
  static const front = CameraDescription(name: 'front', lensDirection: CameraLensDirection.front, sensorOrientation: 0);
}

/// audioplayers fake: records what plays; [finish] ends the current clip.
class FakeAudioPlatform extends AudioplayersPlatformInterface with MockPlatformInterfaceMixin {
  final events = <String, StreamController<AudioEvent>>{};
  final played = <Uint8List>[];
  int stops = 0;

  StreamController<AudioEvent> _c(String id) => events.putIfAbsent(id, StreamController<AudioEvent>.broadcast);

  @override
  Stream<AudioEvent> getEventStream(String playerId) => _c(playerId).stream;
  @override
  Future<void> setSourceBytes(String playerId, Uint8List bytes, {String? mimeType}) async {
    played.add(bytes);
    _c(playerId).add(const AudioEvent(eventType: AudioEventType.prepared, isPrepared: true));
  }
  @override
  Future<void> stop(String playerId) async => stops++;
  @override
  Future<int?> getDuration(String playerId) async => 3000;
  @override
  Future<int?> getCurrentPosition(String playerId) async => 0;

  void finish() {
    for (final c in events.values) {
      c.add(const AudioEvent(eventType: AudioEventType.complete));
    }
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => Future<void>.value();
}

class FakeGlobalAudioPlatform extends GlobalAudioplayersPlatformInterface with MockPlatformInterfaceMixin {
  @override
  Stream<GlobalAudioEvent> getGlobalEventStream() => const Stream.empty();
  @override
  dynamic noSuchMethod(Invocation invocation) => Future<void>.value();
}

/// geolocator fake: a fixed position unless disabled or refused.
class FakeGeolocator extends GeolocatorPlatform with MockPlatformInterfaceMixin {
  bool enabled = true;
  LocationPermission permission = LocationPermission.denied;
  LocationPermission onRequest = LocationPermission.whileInUse;
  ({double lat, double lng}) at = (lat: 24.8481, lng: 89.3730);

  @override
  Future<bool> isLocationServiceEnabled() async => enabled;
  @override
  Future<LocationPermission> checkPermission() async => permission;
  @override
  Future<LocationPermission> requestPermission() async => permission = onRequest;
  @override
  Future<Position> getCurrentPosition({LocationSettings? locationSettings}) async => Position(
      latitude: at.lat,
      longitude: at.lng,
      timestamp: epoch,
      accuracy: 5,
      altitude: 0,
      altitudeAccuracy: 0,
      heading: 0,
      headingAccuracy: 0,
      speed: 0,
      speedAccuracy: 0);
}

/// Returns scripted diagnoses in order and counts calls.
class ScriptedEngine implements DiagnosisEngine {
  ScriptedEngine(this.results);
  final List<LeafDiagnosis> results;
  int calls = 0;

  /// When set, diagnosis waits for it (to observe the analysing state).
  Completer<void>? gate;
  @override
  String get modelVersion => 'scripted';
  @override
  Future<LeafDiagnosis> diagnose(Uint8List photo, String cropCode) async {
    await gate?.future;
    return results[calls++ % results.length];
  }
}

LeafDiagnosis diagnosisOf(String disease, double confidence, {String severity = 'medium'}) =>
    StubDiagnosisEngine.build(disease, confidence, severity, 'scripted');

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
  TestKit._(this.services, this.server, this.tokens, this.clock, this.camera, this.audio, this.geo);
  final AppServices services;
  final FakeServer server;
  final MemoryTokenStore tokens;
  final FixedClock clock;
  final FakeCameraPlatform camera;
  final FakeAudioPlatform audio;
  final FakeGeolocator geo;

  static Future<TestKit> create(
      {String lang = 'en', bool strict = true, ModelSource? modelSource, DiagnosisEngine? engine}) async {
    ConnectivityPlatform.instance = FakeConnectivity();
    final camera = FakeCameraPlatform();
    CameraPlatform.instance = camera;
    final audio = FakeAudioPlatform();
    AudioplayersPlatformInterface.instance = audio;
    GlobalAudioplayersPlatformInterface.instance = FakeGlobalAudioPlatform();
    final geo = FakeGeolocator();
    GeolocatorPlatform.instance = geo;
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
      engine: engine ?? const StubDiagnosisEngine(),
    );
    return TestKit._(services, server, tokens, clock, camera, audio, geo);
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

  /// Installs weather, alert and scan endpoints with realistic payloads.
  void stubHome({bool stale = false, int unread = 1, List<Map<String, Object?>>? scans}) {
    server.json('GET', '/v1/weather', {
      'cell_key': 'c',
      'fetched_at': '2026-03-01T05:50:00Z',
      'data_age_seconds': 600,
      'stale': stale,
      'days': [
        for (var i = 0; i < 7; i++)
          {'date': '2026-03-0${i + 1}', 'temp_min_c': 18.4, 'temp_max_c': 29.6, 'rain_mm': 12.0 + i, 'humidity_pct': 70, 'wind_kph': 11},
      ],
    });
    final alerts = [
      alertJson('a1', 'heavy_rain', 'warning', {'mm': '60'}, read: unread == 0),
      alertJson('a2', 'disease_followup', 'info', {'disease': 'disease.brown_spot.name'}, read: true),
      alertJson('a3', 'heat', 'critical', {'temp': '38'}, read: true),
    ];
    server.json('GET', '/v1/alerts', {'alerts': alerts, 'unread_count': unread});
    for (final a in alerts) {
      server.json('GET', '/v1/alerts/${a['id']}', a);
      server.json('POST', '/v1/alerts/${a['id']}/read', a);
    }
    server.json('POST', '/v1/alerts/read-all', {'updated': unread});
    final all = scans ??
        [
          scanJson('s1', 'rice', saved: true, disease: 'disease.brown_spot.name'),
          scanJson('s2', 'potato'),
          scanJson('s3', 'tomato', status: 'failed'),
          scanJson('s4', 'jute'),
        ];
    server.on('GET', '/v1/scans', (req) => Reply(200, {
          'scans': req.query['saved'] == true ? all.where((s) => s['saved'] == true).toList() : all,
        }));
  }

  /// Installs media upload and scan sync endpoints; [outcome] per operation.
  void stubUpload({String outcome = 'applied'}) {
    var n = 0;
    server.on('POST', '/v1/media/tickets', (_) {
      n++;
      return Reply(201, {
        'media_id': '00000000-0000-7000-8000-00000000m00$n',
        'upload_url': 'http://blob.test/put/$n',
        'method': 'PUT',
        'headers': {'Content-Type': 'image/jpeg'},
        'expires_at': '2026-03-01T07:00:00Z',
      });
    });
    for (var i = 1; i < 10; i++) {
      server.json('PUT', 'http://blob.test/put/$i', null);
      server.json('POST', '/v1/media/00000000-0000-7000-8000-00000000m00$i/complete', {'id': 'm$i', 'status': 'uploaded'});
    }
    server.on('POST', '/v1/scans/sync', (req) {
      final ops = ((req.body! as Map)['operations'] as List).cast<Map>();
      return Reply(200, {
        'results': [
          for (final o in ops)
            {
              'idempotency_key': o['idempotency_key'],
              'outcome': outcome,
              if (outcome != 'rejected') 'scan_id': 'srv-${o['seq']}',
              if (outcome == 'rejected') 'error_code': 'scan.invalid',
            },
        ],
      });
    });
  }

  /// Voice manifest for [lang] with a clip for each of [keys]; clip bodies are
  /// the key itself (JSON-encoded by the fake server).
  void stubVoice(String lang, List<String> keys, {bool corrupt = false}) {
    server.json('GET', '/v1/voice/$lang', {
      'lang': lang,
      'version': 1,
      'full': true,
      'clips': {
        for (final k in keys)
          k: {
            'url': 'http://cdn.test/$lang/$k.mp3',
            'duration_ms': 3000,
            'checksum': sha256.convert(utf8.encode(jsonEncode(corrupt ? 'x' : k))).toString(),
          },
      },
    });
    for (final k in keys) {
      server.json('GET', 'http://cdn.test/$lang/$k.mp3', k);
    }
  }

  static Map<String, Object?> fieldJson(String id, String name) => {
        'id': id,
        'owner_id': 'u',
        'name': name,
        'area': {'unit': 'bigha', 'value_milli': 1500, 'decimal_milli': 0, 'bigha_milli': 1500, 'acre_milli': 0, 'hectare_milli': 0},
        'location': {'lat': 24.85, 'lng': 89.37},
        'district': '',
        'irrigation': 'partial',
        'soil': {'texture': 'loam', 'ph': 6.5},
        'plots': <Object>[],
        'created_at': '2026-03-01T06:00:00Z',
        'updated_at': '2026-03-01T06:00:00Z',
      };

  static Map<String, Object?> recJson(String code, double score) => {
        'crop_code': code,
        'name_key': 'crop.$code.name',
        'score': score,
        'criteria': {'soil': 0.9, 'water': 0.7, 'pest': 0.6, 'market': 0.8, 'seed': 1.0},
        'yield_kg': {'low': 3000, 'likely': 4200, 'high': 5100},
      };

  static Map<String, Object?> roiJson({int seed = 800000}) => {
        'yield_kg': {'low': 3000, 'likely': 4200, 'high': 5100},
        'costs_poisha': {'seed': seed, 'fertiliser': 1200000, 'pesticide': 500000, 'irrigation': 900000, 'labour': 2000000},
        'gross_poisha': {'low': 7500000, 'likely': 10500000, 'high': 12750000},
        'total_cost_poisha': 5400000 + seed - 800000,
        'net_poisha': {'low': 2100000, 'likely': 5100000 - seed + 800000, 'high': 7350000},
        'return_bp': 9444,
      };

  /// Installs fields, advisory and seasonal endpoints for one field `f1`.
  void stubAdvisor({List<Map<String, Object?>>? fields}) {
    server.json('GET', '/v1/fields', {'fields': fields ?? [fieldJson('f1', 'North plot')]});
    server.on('POST', '/v1/fields', (req) => Reply(201, {...fieldJson('f2', (req.body! as Map)['name'] as String)}));
    server.json('GET', '/v1/advisory/fields/f1/recommendations', {
      'season': 'boro',
      'outlook': {'rain_mm': 180, 'data_age_seconds': 0, 'stale': false},
      'items': [recJson('rice_boro', 86), recJson('wheat', 71), recJson('mustard', 64), recJson('okra', 40)],
    });
    server.json('GET', '/v1/advisory/fields/f1/crops/wheat',
        {...recJson('wheat', 71), 'season': 'boro', 'outlook': {'rain_mm': 180}, 'roi': roiJson()});
    server.on('POST', '/v1/advisory/fields/f1/crops/wheat/roi',
        (req) => Reply(200, roiJson(seed: ((req.body! as Map)['costs_poisha'] as Map)['seed'] as int)));
    server.json('GET', '/v1/advisory/fields/f1/rotation', {
      'steps': [
        {'season': 'boro', 'crop_code': 'rice_boro', 'name_key': 'crop.rice_boro.name', 'nitrogen_before_kg_ha': 60, 'nitrogen_after_kg_ha': 20, 'score': 80},
        {'season': 'aus', 'crop_code': 'mungbean', 'name_key': 'crop.mungbean.name', 'nitrogen_before_kg_ha': 20, 'nitrogen_after_kg_ha': 55, 'score': 70},
        {'season': 'aman', 'crop_code': 'rice_aman', 'name_key': 'crop.rice_aman.name', 'nitrogen_before_kg_ha': 55, 'nitrogen_after_kg_ha': 15, 'score': 75},
      ],
      'nitrogen_deficit': true,
    });
    server.json('GET', '/v1/weather/seasonal',
        {'current_season': 'boro', 'rain_mm': {'aus': 600, 'aman': 1400, 'boro': 150}, 'data_age_seconds': 0, 'stale': false});
  }

  static Map<String, Object?> alertJson(String id, String kind, String severity, Map<String, String> params,
          {bool read = false}) =>
      {
        'id': id,
        'kind': kind,
        'severity': severity,
        'title_key': 'alerts.$kind.title',
        'body_key': 'alerts.$kind.body',
        'params': params,
        'created_at': '2026-03-01T05:00:00Z',
        if (read) 'read_at': '2026-03-01T05:30:00Z',
      };

  static Map<String, Object?> scanJson(String id, String crop,
          {bool saved = false, String status = 'completed', String? disease}) =>
      {
        'id': id,
        'media_id': 'm-$id',
        'crop_code': crop,
        'status': status,
        'captured_at': '2026-02-28T09:00:00Z',
        'created_at': '2026-02-28T09:00:00Z',
        'updated_at': '2026-02-28T09:00:00Z',
        'note': '',
        'saved': saved,
        if (disease != null) 'diagnosis': {'name_key': disease},
      };

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

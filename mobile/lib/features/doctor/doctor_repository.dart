import 'package:crypto/crypto.dart';
import 'package:flutter/foundation.dart';

import '../../core/clock.dart';
import '../../core/network/api_client.dart';
import '../../core/storage/local_db.dart';
import '../../core/sync/sync_queue.dart';
import 'dhash.dart';
import 'diagnosis_engine.dart';

/// Where a phone-side scan is on its way to the server.
enum ScanState { local, queued, synced, rejected }

/// A scan made on this phone. The diagnosis is the on-device one; it is
/// available immediately, online or not.
class LocalScan {
  const LocalScan({
    required this.id,
    required this.cropCode,
    required this.capturedAt,
    required this.diagnosis,
    this.phash,
    this.saved = false,
    this.state = ScanState.local,
    this.opKey,
    this.serverId,
  });
  final String id;
  final String cropCode;
  final DateTime capturedAt;
  final LeafDiagnosis diagnosis;
  final String? phash;
  final bool saved;
  final ScanState state;
  final String? opKey;
  final String? serverId;

  LocalScan copyWith({bool? saved, ScanState? state, String? opKey, String? serverId}) => LocalScan(
      id: id,
      cropCode: cropCode,
      capturedAt: capturedAt,
      diagnosis: diagnosis,
      phash: phash,
      saved: saved ?? this.saved,
      state: state ?? this.state,
      opKey: opKey ?? this.opKey,
      serverId: serverId ?? this.serverId);

  Map<String, Object?> toJson() => {
        'id': id,
        'crop_code': cropCode,
        'captured_at': capturedAt.toIso8601String(),
        'diagnosis': diagnosis.toJson(),
        'phash': phash,
        'saved': saved,
        'state': state.name,
        'op_key': opKey,
        'server_id': serverId,
      };

  factory LocalScan.fromJson(Map<String, Object?> j) => LocalScan(
      id: j['id']! as String,
      cropCode: j['crop_code']! as String,
      capturedAt: DateTime.parse(j['captured_at']! as String),
      diagnosis: LeafDiagnosis.fromJson((j['diagnosis']! as Map).cast()),
      phash: j['phash'] as String?,
      saved: j['saved']! as bool,
      state: ScanState.values.byName(j['state']! as String),
      opKey: j['op_key'] as String?,
      serverId: j['server_id'] as String?);
}

/// Crops the doctor can scan (codes from the server catalogue).
const scanCrops = ['rice_boro', 'rice_aman', 'rice_aus', 'wheat', 'maize', 'potato', 'tomato', 'jute'];

/// The Plant Doctor's data: on-device analysis with a perceptual-hash cache,
/// local scan records, and the upload + sync pipeline for when a network is back.
class DoctorRepository extends ChangeNotifier {
  DoctorRepository({
    required this.db,
    required this.api,
    required this.sync,
    required this.ids,
    required this.clock,
    required this.language,
    this.engine = const StubDiagnosisEngine(),
  });
  final LocalDb db;
  final ApiClient api;
  final SyncQueue sync;
  final IdGenerator ids;
  final Clock clock;
  final String Function() language;
  final DiagnosisEngine engine;

  /// The photo being reviewed between viewfinder and result.
  Uint8List? photo;
  String crop = scanCrops.first;

  static const cacheSize = 50;
  static const cacheDistance = 4;

  Future<List<String>> _index() async => ((await db.read('scans.local') as List?) ?? const []).cast<String>();

  Future<LocalScan?> scan(String id) async {
    final raw = await db.read('scan.$id') as Map?;
    return raw == null ? null : LocalScan.fromJson(raw.cast());
  }

  /// All phone-side scans, newest first.
  Future<List<LocalScan>> scans() async => [for (final id in (await _index()).reversed) (await scan(id))!];

  Future<List<LocalScan>> unsynced() async => [for (final s in await scans()) if (s.state != ScanState.synced) s];

  /// Scans not yet on the server (shown as a badge on the viewfinder).
  int waiting = 0;

  Future<void> refresh() async {
    waiting = (await unsynced()).length;
    notifyListeners();
  }

  Future<void> _put(LocalScan s) async {
    await db.write('scan.${s.id}', s.toJson());
    await refresh();
  }

  /// Diagnoses [bytes] on the device (or from the cache when a near-identical
  /// photo was seen) and records the scan for later upload.
  Future<LocalScan> analyse(Uint8List bytes, String cropCode) async {
    final hash = dHash(bytes);
    final diagnosis = await _cached(hash, cropCode) ?? await engine.diagnose(bytes, cropCode);
    if (hash != null) {
      await _remember(hash, cropCode, diagnosis);
    }
    final s = LocalScan(id: ids.next(), cropCode: cropCode, capturedAt: clock.now(), diagnosis: diagnosis, phash: hash);
    await db.putBlob('photo.${s.id}', bytes);
    await db.write('scans.local', [...await _index(), s.id]);
    await _put(s);
    return s;
  }

  Future<LeafDiagnosis?> _cached(String? hash, String cropCode) async {
    if (hash == null) {
      return null;
    }
    for (final e in ((await db.read('leafcache') as List?) ?? const []).cast<Map>()) {
      if (e['crop'] == cropCode && hamming(e['phash']! as String, hash) <= cacheDistance) {
        return LeafDiagnosis.fromJson((e['diagnosis']! as Map).cast());
      }
    }
    return null;
  }

  Future<void> _remember(String hash, String cropCode, LeafDiagnosis d) async {
    final entries = ((await db.read('leafcache') as List?) ?? const []).where((e) => (e as Map)['phash'] != hash);
    await db.write('leafcache', [
      {'phash': hash, 'crop': cropCode, 'diagnosis': d.toJson()},
      ...entries.take(cacheSize - 1),
    ]);
  }

  Future<Uint8List?> photoOf(String id) => db.blob('photo.$id');

  /// Saves the scan to the farmer's log (synced with the next flush).
  Future<void> setSaved(String id, bool saved) async {
    final s = (await scan(id))!;
    await _put(s.copyWith(saved: saved));
    if (s.state == ScanState.synced) {
      await sync.enqueue('annotate_scan', {'scan_id': s.serverId, 'annotation': {'saved': saved}});
    }
  }

  /// Uploads waiting photos, queues their scans and flushes the queue.
  /// Returns false when the network is unavailable.
  Future<bool> syncNow() async {
    try {
      for (final s in await unsynced()) {
        if (s.state == ScanState.local) {
          await _upload(s);
        }
      }
    } on ApiException catch (e) {
      if (e.isOffline) {
        return false;
      }
      rethrow;
    }
    final report = await sync.flush();
    final ops = {for (final o in await db.ops()) o.idempotencyKey: o};
    for (final s in await unsynced()) {
      final op = ops[s.opKey];
      if (s.state == ScanState.queued && op == null) {
        await _put(s.copyWith(state: ScanState.synced, serverId: report.scanIds[s.opKey]));
        if (s.saved) {
          await sync.enqueue('annotate_scan', {'scan_id': report.scanIds[s.opKey], 'annotation': {'saved': true}});
        }
      } else if (op?.status == 'rejected') {
        await _put(s.copyWith(state: ScanState.rejected));
      }
    }
    return !report.offline;
  }

  Future<void> _upload(LocalScan s) async {
    final bytes = (await photoOf(s.id))!;
    final ticket = await api.post('/v1/media/tickets', {
      'content_type': 'image/jpeg',
      'size_bytes': bytes.length,
      'checksum_sha256': sha256.convert(bytes).toString(),
    });
    final mediaId = ticket['media_id']! as String;
    await api.upload(ticket['method']! as String, ticket['upload_url']! as String, bytes,
        ((ticket['headers'] as Map?) ?? const {}).cast<String, String>());
    await api.post('/v1/media/$mediaId/complete');
    final key = await sync.enqueue('create_scan', {
      'create': {
        'media_id': mediaId,
        'crop_code': s.cropCode,
        'captured_at': s.capturedAt.toUtc().toIso8601String(),
        'lang': language(),
      },
    });
    await _put(s.copyWith(state: ScanState.queued, opKey: key));
  }
}

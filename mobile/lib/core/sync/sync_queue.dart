import 'package:flutter/foundation.dart';

import '../clock.dart';
import '../network/api_client.dart';
import '../storage/local_db.dart';

/// Result of one flush.
class SyncReport {
  const SyncReport({required this.applied, required this.rejected, required this.remaining, required this.offline});
  final int applied;
  final int rejected;
  final int remaining;
  final bool offline;
}

/// The client half of offline sync (§6.7): an append-only queue with a
/// monotonic local sequence and a UUIDv7 idempotency key per operation. The
/// server deduplicates on the key, so re-sending after a lost response is safe.
class SyncQueue extends ChangeNotifier {
  SyncQueue({required this.db, required this.api, required this.ids, required this.clock});
  final LocalDb db;
  final ApiClient api;
  final IdGenerator ids;
  final Clock clock;
  int _pending = 0;

  int get pending => _pending;

  Future<void> refresh() async {
    _pending = (await db.ops(pendingOnly: true)).length;
    notifyListeners();
  }

  /// Queues an operation and returns its idempotency key.
  Future<String> enqueue(String kind, Map<String, Object?> payload) async {
    final key = ids.next();
    await db.enqueue(key, kind, payload, clock.now());
    await refresh();
    return key;
  }

  Future<List<QueuedOp>> all() => db.ops();

  /// Sends pending operations in sequence order.
  Future<SyncReport> flush() async {
    final ops = await db.ops(pendingOnly: true);
    if (ops.isEmpty) {
      return const SyncReport(applied: 0, rejected: 0, remaining: 0, offline: false);
    }
    final body = {
      'operations': [
        for (final o in ops) {'idempotency_key': o.idempotencyKey, 'seq': o.seq, 'kind': o.kind, ...o.payload},
      ],
    };
    try {
      final res = await api.post('/v1/scans/sync', body);
      var applied = 0, rejected = 0;
      for (final r in (res['results']! as List).cast<Map>()) {
        final key = r['idempotency_key']! as String;
        if (r['outcome'] == 'rejected') {
          rejected++;
          await db.reject(key, '${r['error_code']}');
        } else {
          applied++;
          await db.dequeue(key);
        }
      }
      await refresh();
      return SyncReport(applied: applied, rejected: rejected, remaining: _pending, offline: false);
    } on ApiException catch (e) {
      return SyncReport(applied: 0, rejected: 0, remaining: ops.length, offline: e.isOffline);
    }
  }
}

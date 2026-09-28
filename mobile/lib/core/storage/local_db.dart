import 'dart:convert';

import 'package:drift/drift.dart';

/// On-device SQLite store (drift, raw SQL — no generated code).
///
/// Tables:
/// * `kv`       — small JSON documents (settings, dictionaries, caches)
/// * `sync_ops` — the append-only offline operation queue (§6.7)
/// * `blobs`    — binary payloads (captured leaf photos awaiting upload)
class LocalDb extends GeneratedDatabase {
  LocalDb(super.executor);

  @override
  Iterable<TableInfo<Table, dynamic>> get allTables => const [];

  @override
  int get schemaVersion => 2;

  @override
  MigrationStrategy get migration => MigrationStrategy(
        onCreate: (m) async {
          await customStatement('CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT NOT NULL)');
          await customStatement('CREATE TABLE sync_ops ('
              'seq INTEGER PRIMARY KEY AUTOINCREMENT, idempotency_key TEXT NOT NULL UNIQUE, '
              'kind TEXT NOT NULL, payload TEXT NOT NULL, created_at TEXT NOT NULL, '
              "status TEXT NOT NULL DEFAULT 'pending', error_code TEXT NOT NULL DEFAULT '')");
          await _createBlobs();
        },
        onUpgrade: (m, from, to) async {
          if (from < 2) {
            await _createBlobs();
          }
        },
      );

  Future<void> _createBlobs() => customStatement('CREATE TABLE blobs (k TEXT PRIMARY KEY, v BLOB NOT NULL)');

  /// Stores binary data under [key].
  Future<void> putBlob(String key, Uint8List data) => customStatement(
      'INSERT INTO blobs (k, v) VALUES (?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v', [key, data]);

  /// Reads binary data, or null.
  Future<Uint8List?> blob(String key) async {
    final rows = await customSelect('SELECT v FROM blobs WHERE k = ?', variables: [Variable(key)]).get();
    return rows.isEmpty ? null : rows.single.read<Uint8List>('v');
  }

  Future<void> removeBlob(String key) => customStatement('DELETE FROM blobs WHERE k = ?', [key]);

  /// Reads a JSON document.
  Future<Object?> read(String key) async {
    final rows = await customSelect('SELECT v FROM kv WHERE k = ?', variables: [Variable(key)]).get();
    return rows.isEmpty ? null : jsonDecode(rows.single.read<String>('v'));
  }

  /// Writes a JSON document.
  Future<void> write(String key, Object? value) => customStatement(
      'INSERT INTO kv (k, v) VALUES (?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v', [key, jsonEncode(value)]);

  /// Deletes a document.
  Future<void> remove(String key) => customStatement('DELETE FROM kv WHERE k = ?', [key]);

  /// Appends an operation to the offline queue; returns its local sequence.
  Future<int> enqueue(String key, String kind, Map<String, Object?> payload, DateTime at) async {
    await customStatement('INSERT INTO sync_ops (idempotency_key, kind, payload, created_at) VALUES (?, ?, ?, ?)',
        [key, kind, jsonEncode(payload), at.toIso8601String()]);
    final row = await customSelect('SELECT seq FROM sync_ops WHERE idempotency_key = ?', variables: [Variable(key)])
        .getSingle();
    return row.read<int>('seq');
  }

  /// Lists queued operations in sequence order.
  Future<List<QueuedOp>> ops({bool pendingOnly = false}) async {
    final rows = await customSelect(
            "SELECT * FROM sync_ops ${pendingOnly ? "WHERE status = 'pending' " : ''}ORDER BY seq")
        .get();
    return [
      for (final r in rows)
        QueuedOp(
          seq: r.read<int>('seq'),
          idempotencyKey: r.read<String>('idempotency_key'),
          kind: r.read<String>('kind'),
          payload: (jsonDecode(r.read<String>('payload')) as Map).cast<String, Object?>(),
          createdAt: DateTime.parse(r.read<String>('created_at')),
          status: r.read<String>('status'),
          errorCode: r.read<String>('error_code'),
        ),
    ];
  }

  /// Removes an operation that the server has applied.
  Future<void> dequeue(String key) => customStatement('DELETE FROM sync_ops WHERE idempotency_key = ?', [key]);

  /// Marks an operation rejected by the server (kept for the user to see).
  Future<void> reject(String key, String code) =>
      customStatement("UPDATE sync_ops SET status = 'rejected', error_code = ? WHERE idempotency_key = ?", [code, key]);
}

/// One queued offline operation.
class QueuedOp {
  const QueuedOp({
    required this.seq,
    required this.idempotencyKey,
    required this.kind,
    required this.payload,
    required this.createdAt,
    required this.status,
    required this.errorCode,
  });
  final int seq;
  final String idempotencyKey;
  final String kind;
  final Map<String, Object?> payload;
  final DateTime createdAt;
  final String status;
  final String errorCode;
}

import 'package:flutter/foundation.dart';

import '../storage/local_db.dart';

/// Describes a downloadable on-device model package.
class ModelManifest {
  const ModelManifest({required this.id, required this.version, required this.sizeBytes});
  final String id;
  final String version;
  final int sizeBytes;

  /// "24 MB" — the size in whole megabytes, for display.
  String get sizeLabel => '${(sizeBytes / (1024 * 1024)).round()} MB';
}

/// Where model packages come from. OPEN SLOT (§14): the real model and its
/// distribution channel are chosen later; only the port and a stub ship.
abstract interface class ModelSource {
  ModelManifest get manifest;

  /// Streams download progress in [0, 1]; completes when the package is on disk.
  Stream<double> fetch();
}

/// Stub source: reports deterministic progress and installs nothing.
class StubModelSource implements ModelSource {
  const StubModelSource();

  @override
  ModelManifest get manifest => const ModelManifest(id: 'plant-doctor-stub', version: '0.0.0', sizeBytes: 24 * 1024 * 1024);

  @override
  Stream<double> fetch() => Stream.fromIterable(const [0.25, 0.5, 0.75, 1.0]);
}

enum ModelStatus { absent, downloading, ready }

/// Tracks the offline model's install state (persisted in the local store).
class OfflineModelManager extends ChangeNotifier {
  OfflineModelManager({required this.db, this.source = const StubModelSource()});
  final LocalDb db;
  final ModelSource source;

  ModelStatus status = ModelStatus.absent;
  double progress = 0;

  ModelManifest get manifest => source.manifest;

  Future<void> load() async {
    final saved = await db.read('model.offline') as Map?;
    if (saved != null && saved['version'] == manifest.version) {
      status = ModelStatus.ready;
      progress = 1;
      notifyListeners();
    }
  }

  Future<void> download() async {
    status = ModelStatus.downloading;
    progress = 0;
    notifyListeners();
    await for (final p in source.fetch()) {
      progress = p;
      notifyListeners();
    }
    await db.write('model.offline', {'id': manifest.id, 'version': manifest.version});
    status = ModelStatus.ready;
    notifyListeners();
  }

  Future<void> remove() async {
    await db.remove('model.offline');
    status = ModelStatus.absent;
    progress = 0;
    notifyListeners();
  }
}

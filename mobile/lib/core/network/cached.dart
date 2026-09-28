import '../clock.dart';
import '../storage/local_db.dart';
import 'api_client.dart';
import 'connectivity.dart';

/// A value plus where it came from. [age] is how old the underlying data is
/// (server-side age at fetch time + time since the fetch).
class Cached<T> {
  const Cached(this.value, {required this.age, required this.fromCache});
  final T value;
  final Duration age;
  final bool fromCache;
}

/// Offline-first reads: fetch from the API and remember the JSON; when the
/// network is unavailable, serve the remembered copy with its age.
class CachedReader {
  CachedReader({required this.api, required this.db, required this.clock, required this.connectivity});
  final ApiClient api;
  final LocalDb db;
  final Clock clock;
  final ConnectivityService connectivity;

  /// Reads [path] (cached under [key]). [serverAge] extracts the data's age
  /// as reported by the server, if any. Throws the offline [ApiException]
  /// when offline with nothing cached.
  Future<Cached<T>> read<T>(String key, String path, T Function(Map<String, Object?>) decode,
      {Map<String, Object?>? query, Duration Function(Map<String, Object?>)? serverAge}) async {
    try {
      final json = await api.get(path, query: query);
      connectivity.report(true);
      await db.write('cache.$key', {'at': clock.now().toIso8601String(), 'json': json});
      return Cached(decode(json), age: serverAge?.call(json) ?? Duration.zero, fromCache: false);
    } on ApiException catch (e) {
      if (!e.isOffline) {
        rethrow;
      }
      connectivity.report(false);
      final saved = await db.read('cache.$key') as Map?;
      if (saved == null) {
        rethrow;
      }
      final json = (saved['json']! as Map).cast<String, Object?>();
      final since = clock.now().difference(DateTime.parse(saved['at']! as String));
      return Cached(decode(json), age: (serverAge?.call(json) ?? Duration.zero) + since, fromCache: true);
    }
  }
}

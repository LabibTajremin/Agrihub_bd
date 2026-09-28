import '../../core/network/api_client.dart';
import '../../core/network/cached.dart';
import '../../core/storage/local_db.dart';
import 'models.dart';

/// A map point; the farmer's home location until a field is pinned.
typedef LatLng = ({double lat, double lng});

/// Bangladesh's centre, used until the farmer shares a location.
const defaultLocation = (lat: 23.81, lng: 90.41);

/// Home-tab data: forecast, alerts and scan history — all offline-first.
class HomeRepository {
  HomeRepository({required this.reader, required this.api, required this.db});
  final CachedReader reader;
  final ApiClient api;
  final LocalDb db;

  Future<LatLng> location() async {
    final saved = await db.read('location') as Map?;
    return saved == null ? defaultLocation : (lat: saved['lat']! as double, lng: saved['lng']! as double);
  }

  Future<void> setLocation(LatLng p) => db.write('location', {'lat': p.lat, 'lng': p.lng});

  Future<Cached<Forecast>> forecast() async {
    final p = await location();
    return reader.read('weather', '/v1/weather', Forecast.fromJson,
        query: {'lat': p.lat, 'lng': p.lng}, serverAge: Forecast.age);
  }

  Future<Cached<Inbox>> alerts() => reader.read('alerts', '/v1/alerts', Inbox.fromJson);

  Future<Cached<AlertItem>> alert(String id) => reader.read('alert.$id', '/v1/alerts/$id', AlertItem.fromJson);

  /// Marks an alert read; offline, the next inbox refresh shows it unread again.
  Future<void> markRead(String id) async {
    try {
      await api.post('/v1/alerts/$id/read');
    } on ApiException catch (e) {
      if (!e.isOffline) {
        rethrow;
      }
    }
  }

  Future<void> markAllRead() => api.post('/v1/alerts/read-all');

  Future<Cached<List<ScanSummary>>> scans({bool saved = false}) => reader.read(
      saved ? 'scans.saved' : 'scans', '/v1/scans', ScanSummary.list, query: {'saved': saved, 'limit': 50});
}

import '../../core/network/api_client.dart';
import '../../core/network/cached.dart';
import '../home/home_repository.dart';
import 'models.dart';

/// Crop Advisor data over /v1/fields, /v1/advisory and /v1/weather/seasonal.
/// Reads are offline-first.
class AdvisorRepository {
  AdvisorRepository({required this.reader, required this.api, required this.home});
  final CachedReader reader;
  final ApiClient api;
  final HomeRepository home;

  Future<Cached<List<FieldInfo>>> fields() => reader.read('fields', '/v1/fields', FieldInfo.list);

  Future<FieldInfo> createField({
    required String name,
    required String unit,
    required int valueMilli,
    required LatLng location,
    required String irrigation,
    required String texture,
    required double ph,
  }) async {
    final res = await api.post('/v1/fields', {
      'name': name,
      'area': {'value_milli': valueMilli, 'unit': unit},
      'location': {'lat': location.lat, 'lng': location.lng},
      'irrigation': irrigation,
      'soil': {
        'texture': texture,
        'ph': ph,
        'nitrogen_kg_ha': 0,
        'phosphorus_kg_ha': 0,
        'potassium_kg_ha': 0,
        'organic_matter_pct': 0,
      },
    });
    await home.setLocation(location);
    return FieldInfo.fromJson(res);
  }

  Future<Cached<Recommendations>> recommendations(String field) => reader.read(
      'advice.$field', '/v1/advisory/fields/$field/recommendations', Recommendations.fromJson);

  Future<Cached<CropDetail>> crop(String field, String code) =>
      reader.read('advice.$field.$code', '/v1/advisory/fields/$field/crops/$code', CropDetail.fromJson);

  Future<Roi> roi(String field, String code, Map<String, int> costs) async =>
      Roi.fromJson(await api.post('/v1/advisory/fields/$field/crops/$code/roi', {'costs_poisha': costs}));

  Future<Cached<Rotation>> rotation(String field) =>
      reader.read('rotation.$field', '/v1/advisory/fields/$field/rotation', Rotation.fromJson);

  Future<Cached<SeasonOutlook>> outlook() async {
    final p = await home.location();
    return reader.read('outlook', '/v1/weather/seasonal', SeasonOutlook.fromJson, query: {'lat': p.lat, 'lng': p.lng});
  }
}

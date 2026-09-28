import '../home/home_repository.dart';

typedef Triple = ({int low, int likely, int high});

Triple _triple(Object? j) {
  final m = (j! as Map).cast<String, Object?>();
  return (low: m['low']! as int, likely: m['likely']! as int, high: m['high']! as int);
}

/// Units a farmer can enter an area in.
const areaUnits = ['decimal', 'bigha', 'acre', 'hectare'];
const irrigationKinds = ['none', 'partial', 'full'];
const soilTextures = ['clay', 'clay_loam', 'loam', 'silt_loam', 'sandy_loam', 'sandy'];

/// A farmer's field as listed.
class FieldInfo {
  const FieldInfo({
    required this.id,
    required this.name,
    required this.unit,
    required this.valueMilli,
    required this.location,
    required this.irrigation,
    this.texture,
    this.ph,
  });
  final String id;
  final String name;
  final String unit;
  final int valueMilli;
  final LatLng location;
  final String irrigation;
  final String? texture;
  final double? ph;

  factory FieldInfo.fromJson(Map<String, Object?> j) {
    final area = (j['area']! as Map).cast<String, Object?>();
    final loc = (j['location']! as Map).cast<String, Object?>();
    final soil = (j['soil'] as Map?)?.cast<String, Object?>();
    return FieldInfo(
      id: j['id']! as String,
      name: j['name']! as String,
      unit: area['unit']! as String,
      valueMilli: area['value_milli']! as int,
      location: (lat: (loc['lat']! as num).toDouble(), lng: (loc['lng']! as num).toDouble()),
      irrigation: j['irrigation']! as String,
      texture: soil?['texture'] as String?,
      ph: (soil?['ph'] as num?)?.toDouble(),
    );
  }

  static List<FieldInfo> list(Map<String, Object?> j) =>
      [for (final f in j['fields']! as List) FieldInfo.fromJson((f as Map).cast())];
}

/// One ranked crop for a field.
class Recommendation {
  const Recommendation({required this.cropCode, required this.score, required this.criteria, required this.yieldKg});
  final String cropCode;
  final double score;
  final Map<String, double> criteria;
  final Triple yieldKg;

  factory Recommendation.fromJson(Map<String, Object?> j) => Recommendation(
      cropCode: j['crop_code']! as String,
      score: (j['score']! as num).toDouble(),
      criteria: (j['criteria']! as Map).map((k, v) => MapEntry('$k', (v as num).toDouble())),
      yieldKg: _triple(j['yield_kg']));
}

class Recommendations {
  const Recommendations(this.season, this.rainMm, this.items);
  final String season;
  final int rainMm;
  final List<Recommendation> items;

  factory Recommendations.fromJson(Map<String, Object?> j) => Recommendations(
      j['season']! as String,
      ((j['outlook']! as Map)['rain_mm']! as int),
      [for (final i in j['items']! as List) Recommendation.fromJson((i as Map).cast())]);
}

/// Profit estimate; money in poisha (1/100 taka).
class Roi {
  const Roi(
      {required this.yieldKg, required this.costs, required this.gross, required this.totalCost, required this.net, required this.returnBp});
  final Triple yieldKg;
  final Map<String, int> costs;
  final Triple gross;
  final int totalCost;
  final Triple net;
  final int returnBp;

  factory Roi.fromJson(Map<String, Object?> j) => Roi(
      yieldKg: _triple(j['yield_kg']),
      costs: (j['costs_poisha']! as Map).map((k, v) => MapEntry('$k', v as int)),
      gross: _triple(j['gross_poisha']),
      totalCost: j['total_cost_poisha']! as int,
      net: _triple(j['net_poisha']),
      returnBp: j['return_bp']! as int);
}

class CropDetail {
  const CropDetail(this.recommendation, this.roi);
  final Recommendation recommendation;
  final Roi roi;

  factory CropDetail.fromJson(Map<String, Object?> j) =>
      CropDetail(Recommendation.fromJson(j), Roi.fromJson((j['roi']! as Map).cast()));
}

class RotationStep {
  const RotationStep(this.season, this.cropCode, this.nitrogenBefore, this.nitrogenAfter);
  final String season;
  final String cropCode;
  final int nitrogenBefore;
  final int nitrogenAfter;
}

class Rotation {
  const Rotation(this.steps, this.deficit);
  final List<RotationStep> steps;
  final bool deficit;

  factory Rotation.fromJson(Map<String, Object?> j) => Rotation([
        for (final s in (j['steps']! as List).cast<Map>())
          RotationStep(s['season']! as String, s['crop_code']! as String, s['nitrogen_before_kg_ha']! as int,
              s['nitrogen_after_kg_ha']! as int),
      ], j['nitrogen_deficit']! as bool);
}

/// Seasonal rainfall outlook (mm per season).
class SeasonOutlook {
  const SeasonOutlook(this.current, this.rainMm);
  final String current;
  final Map<String, int> rainMm;

  factory SeasonOutlook.fromJson(Map<String, Object?> j) => SeasonOutlook(
      j['current_season']! as String, (j['rain_mm']! as Map).map((k, v) => MapEntry('$k', v as int)));
}

/// "৳12,345" from poisha, via the dictionary's currency pattern.
String takaDigits(int poisha) {
  final taka = (poisha / 100).round();
  final s = taka.abs().toString();
  final b = StringBuffer(taka < 0 ? '-' : '');
  for (var i = 0; i < s.length; i++) {
    if (i > 0 && (s.length - i) % 3 == 0) {
      b.write(',');
    }
    b.write(s[i]);
  }
  return b.toString();
}

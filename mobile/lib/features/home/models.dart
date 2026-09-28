/// One forecast day.
class ForecastDay {
  const ForecastDay(
      {required this.date, required this.tempMin, required this.tempMax, required this.rainMm, required this.humidity, required this.windKph});
  final DateTime date;
  final double tempMin;
  final double tempMax;
  final double rainMm;
  final int humidity;
  final int windKph;

  factory ForecastDay.fromJson(Map<String, Object?> j) => ForecastDay(
      date: DateTime.parse(j['date']! as String),
      tempMin: (j['temp_min_c']! as num).toDouble(),
      tempMax: (j['temp_max_c']! as num).toDouble(),
      rainMm: (j['rain_mm']! as num).toDouble(),
      humidity: j['humidity_pct']! as int,
      windKph: j['wind_kph']! as int);
}

/// The 7-day forecast for the farmer's location.
class Forecast {
  const Forecast({required this.stale, required this.days});
  final bool stale;
  final List<ForecastDay> days;

  factory Forecast.fromJson(Map<String, Object?> j) => Forecast(
      stale: j['stale']! as bool,
      days: [for (final d in j['days']! as List) ForecastDay.fromJson((d as Map).cast())]);

  static Duration age(Map<String, Object?> j) => Duration(seconds: j['data_age_seconds']! as int);
}

/// An alert; title/body are dictionary keys filled with [params].
class AlertItem {
  const AlertItem({
    required this.id,
    required this.kind,
    required this.severity,
    required this.titleKey,
    required this.bodyKey,
    required this.params,
    required this.createdAt,
    required this.read,
  });
  final String id;
  final String kind;
  final String severity;
  final String titleKey;
  final String bodyKey;
  final Map<String, String> params;
  final DateTime createdAt;
  final bool read;

  factory AlertItem.fromJson(Map<String, Object?> j) => AlertItem(
      id: j['id']! as String,
      kind: j['kind']! as String,
      severity: j['severity']! as String,
      titleKey: j['title_key']! as String,
      bodyKey: j['body_key']! as String,
      params: ((j['params'] as Map?) ?? const {}).map((k, v) => MapEntry('$k', '$v')),
      createdAt: DateTime.parse(j['created_at']! as String),
      read: j['read_at'] != null);
}

class Inbox {
  const Inbox(this.alerts, this.unread);
  final List<AlertItem> alerts;
  final int unread;

  factory Inbox.fromJson(Map<String, Object?> j) =>
      Inbox([for (final a in j['alerts']! as List) AlertItem.fromJson((a as Map).cast())], j['unread_count']! as int);
}

/// A scan as listed in history and the saved log.
class ScanSummary {
  const ScanSummary(
      {required this.id, required this.cropCode, required this.status, required this.capturedAt, this.diseaseKey, this.saved = false});
  final String id;
  final String cropCode;
  final String status;
  final DateTime capturedAt;
  final String? diseaseKey;
  final bool saved;

  factory ScanSummary.fromJson(Map<String, Object?> j) => ScanSummary(
      id: j['id']! as String,
      cropCode: j['crop_code']! as String,
      status: j['status']! as String,
      capturedAt: DateTime.parse(j['captured_at']! as String),
      diseaseKey: (j['diagnosis'] as Map?)?['name_key'] as String?,
      saved: (j['saved'] as bool?) ?? false);

  static List<ScanSummary> list(Map<String, Object?> j) =>
      [for (final s in j['scans']! as List) ScanSummary.fromJson((s as Map).cast())];
}

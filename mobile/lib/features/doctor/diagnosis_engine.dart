import 'dart:typed_data';

import 'package:crypto/crypto.dart';

/// Below this confidence the result is not shown as a diagnosis; the farmer
/// is asked to retake the photo or ask an expert (§6.9, mirrors the server).
const confidenceThreshold = 0.7;

/// A treatment plan: dictionary keys for each step.
class TreatmentPlan {
  const TreatmentPlan(this.variant, this.steps, {this.safetyKey});
  final String variant;
  final List<String> steps;
  final String? safetyKey;

  Map<String, Object?> toJson() => {'variant': variant, 'steps': steps, 'safety_key': ?safetyKey};

  factory TreatmentPlan.fromJson(Map<String, Object?> j) => TreatmentPlan(
      j['variant']! as String, (j['steps']! as List).cast<String>(),
      safetyKey: j['safety_key'] as String?);
}

/// What the model concluded about one leaf photo.
class LeafDiagnosis {
  const LeafDiagnosis({
    required this.diseaseCode,
    required this.confidence,
    required this.severity,
    required this.modelVersion,
    required this.plans,
  });
  final String diseaseCode;
  final double confidence;
  final String severity;
  final String modelVersion;
  final List<TreatmentPlan> plans;

  bool get healthy => diseaseCode == 'healthy';
  bool get confident => confidence >= confidenceThreshold;
  String get nameKey => 'disease.$diseaseCode.name';
  String get descriptionKey => 'disease.$diseaseCode.description';

  TreatmentPlan? plan(String variant) => plans.where((p) => p.variant == variant).firstOrNull;

  Map<String, Object?> toJson() => {
        'disease_code': diseaseCode,
        'confidence': confidence,
        'severity': severity,
        'model_version': modelVersion,
        'plans': [for (final p in plans) p.toJson()],
      };

  factory LeafDiagnosis.fromJson(Map<String, Object?> j) => LeafDiagnosis(
      diseaseCode: j['disease_code']! as String,
      confidence: (j['confidence']! as num).toDouble(),
      severity: j['severity']! as String,
      modelVersion: j['model_version']! as String,
      plans: [for (final p in j['plans']! as List) TreatmentPlan.fromJson((p as Map).cast())]);
}

/// The on-device model. OPEN SLOT (§14): the real model is chosen later;
/// only this port and a deterministic stub ship.
abstract interface class DiagnosisEngine {
  String get modelVersion;
  Future<LeafDiagnosis> diagnose(Uint8List photo, String cropCode);
}

/// Deterministic stub: the photo's SHA-256 picks the disease and confidence,
/// so the same photo always gives the same answer. It knows nothing about plants.
class StubDiagnosisEngine implements DiagnosisEngine {
  const StubDiagnosisEngine();

  static const diseases = ['healthy', 'brown_spot', 'rice_blast', 'bacterial_leaf_blight', 'sheath_blight', 'tungro'];
  static const severities = ['low', 'medium', 'high'];

  @override
  String get modelVersion => 'stub-0';

  @override
  Future<LeafDiagnosis> diagnose(Uint8List photo, String cropCode) async {
    final h = sha256.convert(photo).bytes;
    return build(diseases[h[0] % diseases.length], 0.5 + (h[1] / 255) * 0.49, severities[h[2] % 3], modelVersion);
  }

  /// Assembles a diagnosis with the treatment plans from the dictionary.
  static LeafDiagnosis build(String disease, double confidence, String severity, String version) {
    final plans = disease == 'healthy'
        ? const [TreatmentPlan('advisory', ['treatment.healthy.step1'])]
        : [
            TreatmentPlan('chemical', ['treatment.$disease.chemical.step1', 'treatment.$disease.chemical.step2'],
                safetyKey: 'treatment.safety'),
            TreatmentPlan('organic', ['treatment.$disease.organic.step1', 'treatment.$disease.organic.step2']),
          ];
    return LeafDiagnosis(
        diseaseCode: disease,
        confidence: confidence,
        severity: disease == 'healthy' ? 'low' : severity,
        modelVersion: version,
        plans: plans);
  }
}

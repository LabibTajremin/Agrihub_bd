import 'package:flutter/material.dart';

import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';
import 'models.dart';

/// One scan row: crop, diagnosis (or status) and capture date.
class ScanTile extends StatelessWidget {
  const ScanTile({super.key, required this.scan});
  final ScanSummary scan;

  @override
  Widget build(BuildContext context) {
    final crop = context.tOr('crop.${scan.cropCode}.name', scan.cropCode);
    final disease = scan.diseaseKey;
    final status = disease == null
        ? context.tOr('diagnosis.status.${scan.status}', scan.status)
        : context.tOr(disease, disease);
    return Card(
      child: ListTile(
        leading: Icon(scan.saved ? Icons.bookmark : Icons.eco_outlined, color: Tokens.primary),
        title: Text(crop, style: Tokens.title),
        subtitle: Text(status),
        trailing: Text(MaterialLocalizations.of(context).formatShortDate(scan.capturedAt), style: Tokens.caption),
      ),
    );
  }
}

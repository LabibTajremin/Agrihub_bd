import 'package:flutter/material.dart';

import '../l10n/localization.dart';
import '../theme/tokens.dart';

/// "just now" / "5 min" / "3 h" / "2 d".
String formatAge(BuildContext context, Duration age) {
  if (age.inMinutes < 1) {
    return context.t('common.just_now');
  }
  if (age.inHours < 1) {
    return context.t('common.minutes', {'n': age.inMinutes});
  }
  if (age.inDays < 1) {
    return context.t('common.hours', {'n': age.inHours});
  }
  return context.t('common.days', {'n': age.inDays});
}

/// The offline banner with the age of the data shown beneath it.
class OfflineBanner extends StatelessWidget {
  const OfflineBanner({super.key, required this.age});
  final Duration age;

  @override
  Widget build(BuildContext context) => Container(
        key: const Key('offline.banner'),
        width: double.infinity,
        color: Tokens.warning,
        padding: const EdgeInsets.symmetric(horizontal: Tokens.spaceLg, vertical: Tokens.spaceSm),
        child: Row(children: [
          const Icon(Icons.cloud_off, color: Tokens.textOnPrimary, size: Tokens.spaceLg),
          const SizedBox(width: Tokens.spaceSm),
          Expanded(
            child: Text(
              '${context.t('home.offline.banner')} ${context.t('common.updated_ago', {'age': formatAge(context, age)})}',
              style: Tokens.label.copyWith(color: Tokens.textOnPrimary),
            ),
          ),
        ]),
      );
}

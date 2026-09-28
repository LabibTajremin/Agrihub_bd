import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';
import 'widgets.dart';

/// 00.9 Permissions primer: explains why each permission is needed before the
/// OS asks (the system prompt appears on first use of each capability).
class PermissionsScreen extends StatelessWidget {
  const PermissionsScreen({super.key});

  static const items = <(String, IconData)>[
    ('onboarding.permissions.camera', Icons.camera_alt_outlined),
    ('onboarding.permissions.location', Icons.location_on_outlined),
    ('onboarding.permissions.microphone', Icons.mic_none),
  ];

  @override
  Widget build(BuildContext context) {
    return OnboardingFrame(
      title: context.t('onboarding.permissions.title'),
      body: ListView(
        children: [
          for (final (key, icon) in items)
            ListTile(
              leading: Icon(icon, color: Tokens.primary),
              title: Text(context.t(key), style: Tokens.body),
            ),
        ],
      ),
      primaryLabel: context.t('onboarding.permissions.allow'),
      onPrimary: () async {
        await AppScope.of(context).onboarding.primePermissions();
        if (context.mounted) {
          context.go('/onboarding/model');
        }
      },
    );
  }
}

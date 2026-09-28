import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';
import 'widgets.dart';

/// 00.2 Language picker: all seven languages, shown in their own script.
/// Selecting one switches the UI immediately.
class LanguageScreen extends StatelessWidget {
  const LanguageScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final l10n = L10nScope.of(context);
    final selected = l10n.current.language.code;
    return OnboardingFrame(
      title: context.t('onboarding.language.title'),
      subtitle: context.t('onboarding.language.subtitle'),
      body: ListView(
        children: [
          for (final lang in languages)
            Card(
              child: ListTile(
                key: Key('lang.${lang.code}'),
                title: Text(lang.nativeName, style: Tokens.title),
                trailing: lang.code == selected ? const Icon(Icons.check_circle, color: Tokens.primary) : null,
                onTap: () => l10n.setLanguage(lang.code),
              ),
            ),
        ],
      ),
      primaryLabel: context.t('common.continue'),
      onPrimary: () => context.go('/onboarding/slides'),
    );
  }
}

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/models/offline_model.dart';
import '../../core/theme/tokens.dart';
import 'widgets.dart';

/// 00.10 Offline model download (the package source is an open-slot stub),
/// the last first-run step.
class ModelScreen extends StatelessWidget {
  const ModelScreen({super.key});

  Future<void> _finish(BuildContext context) async {
    await AppScope.of(context).onboarding.complete();
    if (context.mounted) {
      context.go('/home');
    }
  }

  @override
  Widget build(BuildContext context) {
    final models = AppScope.of(context).models;
    return ListenableBuilder(
      listenable: models,
      builder: (context, _) {
        final status = models.status;
        return OnboardingFrame(
          title: context.t('onboarding.model.title'),
          subtitle: context.t('onboarding.model.body', {'size': models.manifest.sizeLabel}),
          body: Column(
            children: [
              const Icon(Icons.download_for_offline_outlined, size: Tokens.spaceXxxl * 2, color: Tokens.primary),
              const SizedBox(height: Tokens.spaceXl),
              if (status == ModelStatus.downloading) ...[
                LinearProgressIndicator(value: models.progress),
                const SizedBox(height: Tokens.spaceSm),
                Text(context.t('onboarding.model.downloading', {'percent': (models.progress * 100).round()})),
              ],
              if (status == ModelStatus.ready)
                Text(context.t('onboarding.model.ready'), style: Tokens.title.copyWith(color: Tokens.success)),
            ],
          ),
          primaryLabel: context.t(status == ModelStatus.ready ? 'common.continue' : 'onboarding.model.download'),
          onPrimary: switch (status) {
            ModelStatus.absent => models.download,
            ModelStatus.downloading => null,
            ModelStatus.ready => () => _finish(context),
          },
          secondary: status == ModelStatus.ready
              ? null
              : TextButton(onPressed: () => _finish(context), child: Text(context.t('common.skip'))),
        );
      },
    );
  }
}

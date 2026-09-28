import 'package:flutter/material.dart';

import '../../core/l10n/localization.dart';
import '../../core/network/api_client.dart';
import '../../core/theme/tokens.dart';

/// The shared layout of every onboarding step: title, optional subtitle,
/// a body, an inline error and a primary action pinned to the bottom.
class OnboardingFrame extends StatelessWidget {
  const OnboardingFrame({
    super.key,
    required this.title,
    this.subtitle,
    required this.body,
    required this.primaryLabel,
    required this.onPrimary,
    this.secondary,
    this.error,
  });

  final String title;
  final String? subtitle;
  final Widget body;
  final String primaryLabel;
  final VoidCallback? onPrimary;
  final Widget? secondary;
  final String? error;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(Tokens.spaceXl),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(title, style: Tokens.headline),
              if (subtitle != null) ...[
                const SizedBox(height: Tokens.spaceSm),
                Text(subtitle!, style: Tokens.body.copyWith(color: Tokens.textSecondary)),
              ],
              const SizedBox(height: Tokens.spaceXl),
              Expanded(child: body),
              if (error != null)
                Padding(
                  padding: const EdgeInsets.only(bottom: Tokens.spaceMd),
                  child: Text(error!, key: const Key('frame.error'), style: Tokens.label.copyWith(color: Tokens.error)),
                ),
              FilledButton(onPressed: onPrimary, child: Text(primaryLabel)),
              ?secondary,
            ],
          ),
        ),
      ),
    );
  }
}

/// Runs one async action at a time and turns an [ApiException] into a
/// translated inline error.
mixin BusyAction<T extends StatefulWidget> on State<T> {
  bool busy = false;
  String? error;

  Future<void> run(Future<void> Function() action) async {
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await action();
    } on ApiException catch (e) {
      if (mounted) {
        error = context.tError(e);
      }
    }
    if (mounted) {
      setState(() => busy = false);
    }
  }
}

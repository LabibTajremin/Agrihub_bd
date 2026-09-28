import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';

/// 00.1 Splash: decides where a launch lands — the app for returning users,
/// the language picker on first run.
class SplashScreen extends StatefulWidget {
  const SplashScreen({super.key});

  @override
  State<SplashScreen> createState() => _SplashScreenState();
}

class _SplashScreenState extends State<SplashScreen> {
  @override
  void initState() {
    super.initState();
    _route();
  }

  Future<void> _route() async {
    final done = await AppScope.of(context).onboarding.completed;
    if (mounted) {
      context.go(done ? '/home' : '/onboarding/language');
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Tokens.primary,
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.eco, size: Tokens.spaceXxxl * 2, color: Tokens.textOnPrimary),
            const SizedBox(height: Tokens.spaceLg),
            Text(context.t('app.name'), style: Tokens.display.copyWith(color: Tokens.textOnPrimary)),
            const SizedBox(height: Tokens.spaceSm),
            Text(context.t('onboarding.splash.loading'), style: Tokens.body.copyWith(color: Tokens.textOnPrimary)),
          ],
        ),
      ),
    );
  }
}

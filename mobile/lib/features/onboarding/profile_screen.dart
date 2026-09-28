import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';
import 'widgets.dart';

/// 00.8 Profile: name and district; the chosen language is saved with them.
class ProfileScreen extends StatefulWidget {
  const ProfileScreen({super.key});

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> with BusyAction {
  final _name = TextEditingController();
  final _district = TextEditingController();

  @override
  void initState() {
    super.initState();
    _prefill();
  }

  Future<void> _prefill() async {
    final p = await AppScope.of(context).auth.cachedProfile();
    if (p != null) {
      _name.text = p.name;
      _district.text = p.district;
    }
  }

  @override
  void dispose() {
    _name.dispose();
    _district.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final services = AppScope.of(context);
    await run(() async {
      await services.auth.updateProfile(
          name: _name.text.trim(), district: _district.text.trim(), language: services.l10n.current.language.code);
      if (mounted) {
        context.go('/onboarding/permissions');
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return OnboardingFrame(
      title: context.t('onboarding.profile.title'),
      body: ListView(
        children: [
          TextField(
            key: const Key('profile.name'),
            controller: _name,
            maxLength: 80,
            decoration: InputDecoration(labelText: context.t('onboarding.profile.name')),
          ),
          const SizedBox(height: Tokens.spaceMd),
          TextField(
            key: const Key('profile.district'),
            controller: _district,
            maxLength: 60,
            decoration: InputDecoration(labelText: context.t('onboarding.profile.district')),
          ),
        ],
      ),
      error: error,
      primaryLabel: context.t('common.save'),
      onPrimary: busy ? null : _save,
      secondary: TextButton(
          onPressed: busy ? null : () => context.go('/onboarding/permissions'), child: Text(context.t('common.skip'))),
    );
  }
}

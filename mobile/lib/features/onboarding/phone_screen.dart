import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import 'auth_repository.dart';
import 'widgets.dart';

/// 00.6 Phone sign-in, with the guest path for a first scan without an account.
class PhoneScreen extends StatefulWidget {
  const PhoneScreen({super.key});

  @override
  State<PhoneScreen> createState() => _PhoneScreenState();
}

class _PhoneScreenState extends State<PhoneScreen> with BusyAction {
  final _phone = TextEditingController();

  @override
  void dispose() {
    _phone.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    final phone = normalizePhone(_phone.text);
    if (phone == null) {
      setState(() => error = context.t('errors.auth.invalid_phone'));
      return;
    }
    final auth = AppScope.of(context).auth;
    await run(() async {
      await auth.requestOtp(phone);
      if (mounted) {
        context.go(Uri(path: '/onboarding/otp', queryParameters: {'phone': phone}).toString());
      }
    });
  }

  Future<void> _guest() async {
    final auth = AppScope.of(context).auth;
    await run(() async {
      await auth.continueAsGuest();
      if (mounted) {
        context.go('/onboarding/permissions');
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return OnboardingFrame(
      title: context.t('onboarding.phone.title'),
      body: Align(
        alignment: Alignment.topCenter,
        child: TextField(
          key: const Key('phone.input'),
          controller: _phone,
          keyboardType: TextInputType.phone,
          decoration: InputDecoration(hintText: context.t('onboarding.phone.hint'), prefixIcon: const Icon(Icons.phone)),
        ),
      ),
      error: error,
      primaryLabel: context.t('onboarding.phone.send'),
      onPrimary: busy ? null : _send,
      secondary: TextButton(onPressed: busy ? null : _guest, child: Text(context.t('onboarding.guest.continue'))),
    );
  }
}

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';
import 'widgets.dart';

/// 00.7 One-time code entry.
class OtpScreen extends StatefulWidget {
  const OtpScreen({super.key, required this.phone});
  final String phone;

  static const length = 6;

  @override
  State<OtpScreen> createState() => _OtpScreenState();
}

class _OtpScreenState extends State<OtpScreen> with BusyAction {
  final _code = TextEditingController();

  @override
  void dispose() {
    _code.dispose();
    super.dispose();
  }

  Future<void> _verify() async {
    final auth = AppScope.of(context).auth;
    await run(() async {
      await auth.verifyOtp(widget.phone, _code.text.trim());
      if (mounted) {
        context.go('/onboarding/profile');
      }
    });
  }

  Future<void> _resend() async {
    final auth = AppScope.of(context).auth;
    _code.clear();
    await run(() => auth.requestOtp(widget.phone));
  }

  @override
  Widget build(BuildContext context) {
    return OnboardingFrame(
      title: context.t('onboarding.otp.title'),
      subtitle: context.t('onboarding.otp.subtitle', {'phone': widget.phone}),
      body: Column(
        children: [
          TextField(
            key: const Key('otp.input'),
            controller: _code,
            keyboardType: TextInputType.number,
            maxLength: OtpScreen.length,
            textAlign: TextAlign.center,
            style: Tokens.headline.copyWith(letterSpacing: Tokens.spaceMd),
            onChanged: (_) => setState(() {}),
          ),
          TextButton(onPressed: busy ? null : _resend, child: Text(context.t('onboarding.otp.resend'))),
        ],
      ),
      error: error,
      primaryLabel: context.t('onboarding.otp.verify'),
      onPrimary: busy || _code.text.trim().length != OtpScreen.length ? null : _verify,
    );
  }
}

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';

/// 2.2 Preview: keep the photo or retake it.
class PreviewScreen extends StatelessWidget {
  const PreviewScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final photo = AppScope.of(context).doctor.photo!;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('diagnosis.preview.title'))),
      body: Column(children: [
        Expanded(child: Image.memory(photo, fit: BoxFit.contain, gaplessPlayback: true)),
        Padding(
          padding: const EdgeInsets.all(Tokens.spaceLg),
          child: Row(children: [
            Expanded(child: OutlinedButton(onPressed: () => context.pop(), child: Text(context.t('diagnosis.preview.retake')))),
            const SizedBox(width: Tokens.spaceMd),
            Expanded(
              child: FilledButton(
                  onPressed: () => context.go('/doctor/analysing'), child: Text(context.t('diagnosis.preview.use'))),
            ),
          ]),
        ),
      ]),
    );
  }
}

/// 2.3 Analysing: runs the on-device model, then shows the result.
class AnalysingScreen extends StatefulWidget {
  const AnalysingScreen({super.key});

  @override
  State<AnalysingScreen> createState() => _AnalysingScreenState();
}

class _AnalysingScreenState extends State<AnalysingScreen> {
  bool _started = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_started) {
      _started = true;
      _run();
    }
  }

  Future<void> _run() async {
    final doctor = AppScope.of(context).doctor;
    final scan = await doctor.analyse(doctor.photo!, doctor.crop);
    doctor.photo = null;
    if (mounted) {
      context.go('/doctor/result/${scan.id}');
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
        body: Center(
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            const CircularProgressIndicator(),
            const SizedBox(height: Tokens.spaceXl),
            Text(context.t('diagnosis.analysing.title'), style: Tokens.headline),
            const SizedBox(height: Tokens.spaceSm),
            Text(context.t('diagnosis.analysing.body'), style: Tokens.body),
          ]),
        ),
      );
}

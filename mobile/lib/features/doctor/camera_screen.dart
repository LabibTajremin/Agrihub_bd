import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/async_view.dart';
import 'camera_gateway.dart';
import 'doctor_repository.dart';

/// 2.1 Viewfinder with the leaf alignment guide and crop picker.
class CameraScreen extends StatefulWidget {
  const CameraScreen({super.key});

  @override
  State<CameraScreen> createState() => _CameraScreenState();
}

class _CameraScreenState extends State<CameraScreen> {
  late final CameraGateway _camera = AppScope.of(context).camera();
  bool _started = false;
  bool? _ready;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_started) {
      _started = true;
      _start();
    }
  }

  Future<void> _start() async {
    setState(() => _ready = null);
    final ok = await _camera.start();
    if (mounted) {
      setState(() => _ready = ok);
    }
  }

  @override
  void dispose() {
    _camera.stop();
    super.dispose();
  }

  Future<void> _capture(DoctorRepository doctor) async {
    doctor.photo = await _camera.capture();
    if (mounted) {
      await context.push('/doctor/preview');
    }
  }

  @override
  Widget build(BuildContext context) {
    final doctor = AppScope.of(context).doctor;
    return ListenableBuilder(
      listenable: doctor,
      builder: (context, _) => Scaffold(
        appBar: AppBar(
          title: Text(context.t('diagnosis.camera.title')),
          actions: [
            IconButton(
              key: const Key('doctor-sync'),
              tooltip: context.t('diagnosis.sync.title'),
              onPressed: () => context.push('/doctor/sync'),
              icon: Badge(
                isLabelVisible: doctor.waiting > 0,
                label: Text('${doctor.waiting}'),
                child: const Icon(Icons.sync),
              ),
            ),
          ],
        ),
        body: Column(children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: Tokens.spaceLg),
            child: DropdownButton<String>(
              key: const Key('doctor-crop'),
              isExpanded: true,
              value: doctor.crop,
              items: [
                for (final c in scanCrops) DropdownMenuItem(value: c, child: Text(context.t('crop.$c.name'))),
              ],
              onChanged: (c) => setState(() => doctor.crop = c!),
            ),
          ),
          Expanded(
            child: switch (_ready) {
              true => Stack(fit: StackFit.expand, children: [
                  _camera.preview(),
                  const _AlignmentGuide(),
                ]),
              false => ErrorPanel(message: context.t('common.error_title'), onRetry: _start),
              null => const Center(child: CircularProgressIndicator()),
            },
          ),
          Padding(
            padding: const EdgeInsets.all(Tokens.spaceLg),
            child: FilledButton.icon(
              onPressed: _ready == true ? () => _capture(doctor) : null,
              icon: const Icon(Icons.camera_alt),
              label: Text(context.t('diagnosis.camera.capture')),
            ),
          ),
        ]),
      ),
    );
  }
}

/// A leaf-shaped frame with a hint, drawn over the preview.
class _AlignmentGuide extends StatelessWidget {
  const _AlignmentGuide();

  @override
  Widget build(BuildContext context) => Center(
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          Container(
            key: const Key('alignment-guide'),
            width: 240,
            height: 240,
            decoration: BoxDecoration(
              border: Border.all(color: Tokens.accent, width: 3),
              borderRadius: BorderRadius.circular(Tokens.radiusLg),
            ),
          ),
          const SizedBox(height: Tokens.spaceMd),
          Container(
            color: Tokens.overlay,
            padding: const EdgeInsets.all(Tokens.spaceSm),
            child: Text(context.t('diagnosis.camera.align_hint'), style: Tokens.label.copyWith(color: Tokens.textOnPrimary)),
          ),
        ]),
      );
}

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';
import 'diagnosis_engine.dart';
import 'doctor_repository.dart';

/// 2.4 Result: diagnosis with confidence meter and chemical/organic plans;
/// 2.4b low-confidence escalation; 2.4c healthy.
class ResultScreen extends StatefulWidget {
  const ResultScreen({super.key, required this.id});
  final String id;

  @override
  State<ResultScreen> createState() => _ResultScreenState();
}

class _ResultScreenState extends State<ResultScreen> {
  LocalScan? _scan;
  bool _loaded = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_loaded) {
      _loaded = true;
      _load();
    }
  }

  Future<void> _load() async {
    final s = await AppScope.of(context).doctor.scan(widget.id);
    if (mounted) {
      setState(() => _scan = s);
    }
  }

  Future<void> _toggleSaved(LocalScan s) async {
    await AppScope.of(context).doctor.setSaved(s.id, !s.saved);
    await _load();
  }

  @override
  Widget build(BuildContext context) {
    final s = _scan;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('diagnosis.result.title'))),
      body: s == null
          ? const Center(child: CircularProgressIndicator())
          : ListView(padding: const EdgeInsets.all(Tokens.spaceLg), children: [
              if (s.state != ScanState.synced)
                Card(
                  key: const Key('pending-sync'),
                  color: Tokens.surfaceVariant,
                  child: ListTile(
                    leading: const Icon(Icons.cloud_upload_outlined),
                    title: Text(context.t('diagnosis.result.pending_sync'), style: Tokens.label),
                  ),
                ),
              if (!s.diagnosis.confident)
                _LowConfidence(onRetake: () => context.go('/doctor'))
              else if (s.diagnosis.healthy)
                _Healthy(diagnosis: s.diagnosis)
              else
                _Disease(diagnosis: s.diagnosis),
              const SizedBox(height: Tokens.spaceLg),
              OutlinedButton.icon(
                key: const Key('save-toggle'),
                onPressed: () => _toggleSaved(s),
                icon: Icon(s.saved ? Icons.bookmark : Icons.bookmark_border),
                label: Text(context.t(s.saved ? 'diagnosis.result.saved' : 'diagnosis.result.save')),
              ),
            ]),
    );
  }
}

class _LowConfidence extends StatelessWidget {
  const _LowConfidence({required this.onRetake});
  final VoidCallback onRetake;

  @override
  Widget build(BuildContext context) => Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        const Icon(Icons.help_outline, size: Tokens.spaceXxxl * 2, color: Tokens.warning),
        Text(context.t('diagnosis.low_confidence.title'), style: Tokens.headline, textAlign: TextAlign.center),
        const SizedBox(height: Tokens.spaceSm),
        Text(context.t('diagnosis.low_confidence.body'), style: Tokens.body, textAlign: TextAlign.center),
        const SizedBox(height: Tokens.spaceXl),
        FilledButton(onPressed: onRetake, child: Text(context.t('diagnosis.preview.retake'))),
        const SizedBox(height: Tokens.spaceSm),
        OutlinedButton(
            onPressed: () => context.go('/settings/expert'), child: Text(context.t('diagnosis.low_confidence.ask_expert'))),
      ]);
}

class _Healthy extends StatelessWidget {
  const _Healthy({required this.diagnosis});
  final LeafDiagnosis diagnosis;

  @override
  Widget build(BuildContext context) => Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        const Icon(Icons.check_circle, size: Tokens.spaceXxxl * 2, color: Tokens.success),
        Text(context.t('diagnosis.healthy.title'), style: Tokens.headline, textAlign: TextAlign.center),
        const SizedBox(height: Tokens.spaceSm),
        Text(context.t('diagnosis.healthy.body'), style: Tokens.body, textAlign: TextAlign.center),
        const SizedBox(height: Tokens.spaceLg),
        Text(context.t('diagnosis.result.advisory'), style: Tokens.title),
        _Steps(plan: diagnosis.plan('advisory')!),
      ]);
}

class _Disease extends StatelessWidget {
  const _Disease({required this.diagnosis});
  final LeafDiagnosis diagnosis;

  @override
  Widget build(BuildContext context) {
    final pct = (diagnosis.confidence * 100).round();
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text(context.t(diagnosis.nameKey), style: Tokens.headline),
      const SizedBox(height: Tokens.spaceXs),
      Text(context.t(diagnosis.descriptionKey), style: Tokens.body),
      const SizedBox(height: Tokens.spaceLg),
      Row(children: [
        Expanded(child: Text(context.t('diagnosis.result.confidence_label'), style: Tokens.label)),
        Text('$pct%', style: Tokens.label),
      ]),
      const SizedBox(height: Tokens.spaceXs),
      LinearProgressIndicator(
        key: const Key('confidence-meter'),
        value: diagnosis.confidence,
        minHeight: Tokens.spaceSm,
        color: Tokens.primary,
        backgroundColor: Tokens.primaryLight,
      ),
      const SizedBox(height: Tokens.spaceMd),
      Row(children: [
        Text('${context.t('diagnosis.result.severity')}: ', style: Tokens.label),
        Chip(label: Text(context.t('diagnosis.severity.${diagnosis.severity}'))),
      ]),
      DefaultTabController(
        length: 2,
        child: Column(children: [
          TabBar(tabs: [
            Tab(text: context.t('diagnosis.result.chemical')),
            Tab(text: context.t('diagnosis.result.organic')),
          ]),
          SizedBox(
            height: 260,
            child: TabBarView(children: [
              _Steps(plan: diagnosis.plan('chemical')!),
              _Steps(plan: diagnosis.plan('organic')!),
            ]),
          ),
        ]),
      ),
    ]);
  }
}

class _Steps extends StatelessWidget {
  const _Steps({required this.plan});
  final TreatmentPlan plan;

  @override
  Widget build(BuildContext context) => Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        for (final (i, step) in plan.steps.indexed)
          ListTile(
            leading: CircleAvatar(backgroundColor: Tokens.primaryLight, child: Text('${i + 1}')),
            title: Text(context.t(step), style: Tokens.body),
          ),
        if (plan.safetyKey case final safety?)
          Container(
            key: const Key('safety-note'),
            color: Tokens.surfaceVariant,
            padding: const EdgeInsets.all(Tokens.spaceSm),
            child: Row(children: [
              const Icon(Icons.warning_amber, color: Tokens.warning),
              const SizedBox(width: Tokens.spaceSm),
              Expanded(child: Text(context.t(safety), style: Tokens.caption)),
            ]),
          ),
      ]);
}

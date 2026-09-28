import 'package:flutter/material.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/network/api_client.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/async_view.dart';
import 'doctor_repository.dart';

/// 2.5 Offline sync queue: scans made on this phone that the server has not
/// seen yet, with a manual "sync now".
class SyncScreen extends StatefulWidget {
  const SyncScreen({super.key});

  @override
  State<SyncScreen> createState() => _SyncScreenState();
}

class _SyncScreenState extends State<SyncScreen> {
  List<LocalScan>? _scans;
  bool _busy = false;
  bool _offline = false;
  ApiException? _error;
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
    final s = await AppScope.of(context).doctor.unsynced();
    if (mounted) {
      setState(() => _scans = s);
    }
  }

  Future<void> _sync() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      _offline = !await AppScope.of(context).doctor.syncNow();
    } on ApiException catch (e) {
      _error = e;
    }
    await _load();
    if (mounted) {
      setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final scans = _scans;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('diagnosis.sync.title'))),
      body: scans == null
          ? const Center(child: CircularProgressIndicator())
          : Column(children: [
              if (_offline)
                Container(
                  key: const Key('sync-offline'),
                  width: double.infinity,
                  color: Tokens.warning,
                  padding: const EdgeInsets.all(Tokens.spaceSm),
                  child: Text(context.t('home.offline.banner'), style: Tokens.label.copyWith(color: Tokens.textOnPrimary)),
                ),
              if (_error case final error?)
                Padding(
                  padding: const EdgeInsets.all(Tokens.spaceSm),
                  child: Text(context.tError(error), key: const Key('sync-error'), style: Tokens.label.copyWith(color: Tokens.error)),
                ),
              Expanded(
                child: scans.isEmpty
                    ? EmptyState(message: context.t('diagnosis.sync.empty'), icon: Icons.cloud_done_outlined)
                    : ListView(padding: const EdgeInsets.all(Tokens.spaceLg), children: [
                        Text(context.t('diagnosis.sync.pending', {'count': scans.length}), style: Tokens.title),
                        for (final s in scans)
                          Card(
                            child: ListTile(
                              leading: Icon(s.state == ScanState.rejected ? Icons.error_outline : Icons.schedule,
                                  color: s.state == ScanState.rejected ? Tokens.error : Tokens.textSecondary),
                              title: Text(context.t('crop.${s.cropCode}.name'), style: Tokens.title),
                              subtitle: Text(context.t(s.diagnosis.nameKey)),
                              trailing: Text(context.t(
                                  s.state == ScanState.rejected ? 'diagnosis.status.failed' : 'diagnosis.status.queued')),
                            ),
                          ),
                      ]),
              ),
              if (scans.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.all(Tokens.spaceLg),
                  child: FilledButton.icon(
                    onPressed: _busy ? null : _sync,
                    icon: const Icon(Icons.sync),
                    label: Text(context.t('diagnosis.sync.now')),
                  ),
                ),
            ]),
    );
  }
}

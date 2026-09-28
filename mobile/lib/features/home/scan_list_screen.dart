import 'package:flutter/material.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/network/cached.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/async_view.dart';
import '../../core/widgets/data_age.dart';
import 'models.dart';
import 'scan_tile.dart';

/// 1.3 Scan history and 1.4 saved log (the same list, filtered).
class ScanListScreen extends StatelessWidget {
  const ScanListScreen({super.key, required this.saved});
  final bool saved;

  @override
  Widget build(BuildContext context) {
    final home = AppScope.of(context).home;
    return Scaffold(
      appBar: AppBar(title: Text(context.t(saved ? 'home.saved.title' : 'home.history.title'))),
      body: AsyncView<Cached<List<ScanSummary>>>(
        load: () => home.scans(saved: saved),
        builder: (context, scans, _) => Column(children: [
          if (scans.fromCache) OfflineBanner(age: scans.age),
          Expanded(
            child: scans.value.isEmpty
                ? EmptyState(message: context.t(saved ? 'common.empty' : 'diagnosis.history.empty'))
                : ListView(
                    padding: const EdgeInsets.all(Tokens.spaceLg),
                    children: [for (final s in scans.value) ScanTile(scan: s)],
                  ),
          ),
        ]),
      ),
    );
  }
}

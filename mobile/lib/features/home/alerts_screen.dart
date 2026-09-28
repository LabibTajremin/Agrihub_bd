import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/network/cached.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/async_view.dart';
import '../../core/widgets/data_age.dart';
import 'models.dart';

Color severityColor(String severity) => switch (severity) {
      'critical' => Tokens.error,
      'warning' => Tokens.warning,
      _ => Tokens.info,
    };

/// Params may themselves be dictionary keys (e.g. a disease name).
Map<String, Object?> alertParams(BuildContext context, AlertItem a) =>
    a.params.map((k, v) => MapEntry(k, context.tOr(v, v)));

/// 1.2 Alerts inbox.
class AlertsScreen extends StatelessWidget {
  const AlertsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final home = AppScope.of(context).home;
    return AsyncView<Cached<Inbox>>(
      load: home.alerts,
      builder: (context, inbox, reload) => Scaffold(
        appBar: AppBar(
          title: Text(context.t('alerts.title')),
          actions: [
            if (inbox.value.unread > 0 && !inbox.fromCache)
              TextButton(
                onPressed: () async {
                  await home.markAllRead();
                  await reload();
                },
                child: Text(context.t('alerts.mark_all_read')),
              ),
          ],
        ),
        body: Column(children: [
          if (inbox.fromCache) OfflineBanner(age: inbox.age),
          Expanded(
            child: inbox.value.alerts.isEmpty
                ? EmptyState(message: context.t('alerts.empty'), icon: Icons.notifications_none)
                : ListView(padding: const EdgeInsets.all(Tokens.spaceLg), children: [
                    for (final a in inbox.value.alerts)
                      Card(
                        child: ListTile(
                          leading: Icon(Icons.warning_amber, color: severityColor(a.severity)),
                          title: Text(context.t(a.titleKey),
                              style: Tokens.title.copyWith(fontWeight: a.read ? FontWeight.w400 : FontWeight.w700)),
                          subtitle: Text(context.t('alerts.severity.${a.severity}')),
                          trailing: Text(MaterialLocalizations.of(context).formatShortDate(a.createdAt), style: Tokens.caption),
                          onTap: () async {
                            await context.push('/home/alerts/${a.id}');
                            await reload();
                          },
                        ),
                      ),
                  ]),
          ),
        ]),
      ),
    );
  }
}

/// 1.2b Alert detail; opening it marks the alert read.
class AlertDetailScreen extends StatelessWidget {
  const AlertDetailScreen({super.key, required this.id});
  final String id;

  @override
  Widget build(BuildContext context) {
    final home = AppScope.of(context).home;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('alerts.title'))),
      body: AsyncView<Cached<AlertItem>>(
        load: () async {
          final a = await home.alert(id);
          await home.markRead(id);
          return a;
        },
        builder: (context, a, _) => Column(children: [
          if (a.fromCache) OfflineBanner(age: a.age),
          Padding(
            padding: const EdgeInsets.all(Tokens.spaceXl),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Chip(
                label: Text(context.t('alerts.severity.${a.value.severity}')),
                backgroundColor: severityColor(a.value.severity).withValues(alpha: 0.15),
              ),
              const SizedBox(height: Tokens.spaceMd),
              Text(context.t(a.value.titleKey, alertParams(context, a.value)), style: Tokens.headline),
              const SizedBox(height: Tokens.spaceMd),
              Text(context.t(a.value.bodyKey, alertParams(context, a.value)), style: Tokens.body),
            ]),
          ),
        ]),
      ),
    );
  }
}

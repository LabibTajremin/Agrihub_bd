import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/network/api_client.dart';
import '../../core/network/cached.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/async_view.dart';
import '../../core/widgets/data_age.dart';
import 'models.dart';
import 'scan_tile.dart';

/// Everything the dashboard shows; each section loads independently so one
/// failure (or a guest without an inbox) never blanks the page.
class DashboardData {
  const DashboardData({this.name, this.forecast, this.inbox, this.scans});
  final String? name;
  final Cached<Forecast>? forecast;
  final Cached<Inbox>? inbox;
  final Cached<List<ScanSummary>>? scans;

  /// The age of the oldest cached section, or null when everything is live.
  Duration? get offlineAge {
    final cached = [forecast, inbox, scans].whereType<Cached<Object?>>().where((c) => c.fromCache);
    return cached.isEmpty ? null : cached.map((c) => c.age).reduce((a, b) => a > b ? a : b);
  }
}

Future<T?> _section<T>(Future<T> Function() load) async {
  try {
    return await load();
  } on ApiException {
    return null;
  }
}

/// 1.0 Home dashboard (1.0b: the offline variant with the data-age banner).
class DashboardScreen extends StatelessWidget {
  const DashboardScreen({super.key});

  Future<DashboardData> _load(AppServices s) async {
    final profile = await s.auth.cachedProfile();
    final results = await Future.wait<Object?>([
      _section(s.home.forecast),
      _section(s.home.alerts),
      _section(s.home.scans),
    ]);
    return DashboardData(
      name: profile?.name,
      forecast: results[0] as Cached<Forecast>?,
      inbox: results[1] as Cached<Inbox>?,
      scans: results[2] as Cached<List<ScanSummary>>?,
    );
  }

  @override
  Widget build(BuildContext context) {
    final services = AppScope.of(context);
    return AsyncView<DashboardData>(
      load: () => _load(services),
      builder: (context, data, reload) {
        final age = data.offlineAge;
        final name = data.name ?? '';
        return Column(children: [
          if (age != null) OfflineBanner(age: age),
          Expanded(
            child: RefreshIndicator(
              onRefresh: reload,
              child: ListView(padding: const EdgeInsets.all(Tokens.spaceLg), children: [
                Text(name.isEmpty ? context.t('home.greeting_guest') : context.t('home.greeting', {'name': name}),
                    style: Tokens.headline),
                const SizedBox(height: Tokens.spaceLg),
                _WeatherCard(forecast: data.forecast?.value),
                _ActionCard(
                  icon: Icons.camera_alt_outlined,
                  title: context.t('home.quickscan.title'),
                  subtitle: context.t('home.quickscan.subtitle'),
                  highlight: true,
                  onTap: () => context.go('/doctor'),
                ),
                _ActionCard(
                  icon: Icons.notifications_none,
                  title: context.t('home.alerts.title'),
                  badge: data.inbox?.value.unread,
                  onTap: () => context.go('/home/alerts'),
                ),
                _ActionCard(icon: Icons.insights_outlined, title: context.t('home.advisor.title'), onTap: () => context.go('/advisor')),
                _ActionCard(icon: Icons.mic_none, title: context.t('home.voice.title'), onTap: () => context.go('/voice')),
                _ActionCard(icon: Icons.bookmark_border, title: context.t('home.saved.title'), onTap: () => context.go('/home/saved')),
                const SizedBox(height: Tokens.spaceLg),
                Row(children: [
                  Expanded(child: Text(context.t('home.history.title'), style: Tokens.title)),
                  TextButton(onPressed: () => context.go('/home/history'), child: Text(context.t('common.see_all'))),
                ]),
                if ((data.scans?.value ?? const []).isEmpty)
                  Text(context.t('diagnosis.history.empty'), style: Tokens.body.copyWith(color: Tokens.textSecondary))
                else
                  for (final s in data.scans!.value.take(3)) ScanTile(scan: s),
              ]),
            ),
          ),
        ]);
      },
    );
  }
}

class _WeatherCard extends StatelessWidget {
  const _WeatherCard({required this.forecast});
  final Forecast? forecast;

  @override
  Widget build(BuildContext context) {
    final today = forecast?.days.firstOrNull;
    return Card(
      key: const Key('card-weather'),
      child: ListTile(
        leading: const Icon(Icons.wb_sunny_outlined, color: Tokens.accent, size: Tokens.spaceXxl),
        title: Text(context.t('home.weather.title'), style: Tokens.title),
        subtitle: Text(today == null
            ? context.t('weather.stale')
            : '${today.tempMin.round()}–${today.tempMax.round()}°C · ${context.t('weather.rainfall')} ${today.rainMm.round()} mm'),
        trailing: const Icon(Icons.chevron_right),
        onTap: () => context.go('/home/weather'),
      ),
    );
  }
}

class _ActionCard extends StatelessWidget {
  const _ActionCard({required this.icon, required this.title, this.subtitle, this.badge, this.highlight = false, required this.onTap});
  final IconData icon;
  final String title;
  final String? subtitle;
  final int? badge;
  final bool highlight;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final fg = highlight ? Tokens.textOnPrimary : Tokens.textPrimary;
    return Card(
      color: highlight ? Tokens.primary : null,
      child: ListTile(
        leading: Icon(icon, color: highlight ? fg : Tokens.primary),
        title: Text(title, style: Tokens.title.copyWith(color: fg)),
        subtitle: subtitle == null ? null : Text(subtitle!, style: Tokens.body.copyWith(color: fg)),
        trailing: (badge ?? 0) > 0 ? Badge(label: Text('$badge')) : Icon(Icons.chevron_right, color: fg),
        onTap: onTap,
      ),
    );
  }
}

import 'package:flutter/material.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/network/cached.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/async_view.dart';
import '../../core/widgets/chart.dart';
import '../../core/widgets/data_age.dart';
import 'models.dart';

/// 1.1 Weather detail: the 7-day forecast with humidity and wind.
class WeatherScreen extends StatelessWidget {
  const WeatherScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final home = AppScope.of(context).home;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('weather.detail.title'))),
      body: AsyncView<Cached<Forecast>>(
        load: home.forecast,
        builder: (context, f, _) => Column(children: [
          if (f.fromCache) OfflineBanner(age: f.age),
          if (f.value.stale && !f.fromCache)
            Container(
              key: const Key('weather.stale'),
              width: double.infinity,
              color: Tokens.surfaceVariant,
              padding: const EdgeInsets.all(Tokens.spaceSm),
              child: Text(context.t('weather.stale'), style: Tokens.label),
            ),
          Expanded(
            child: ListView(padding: const EdgeInsets.all(Tokens.spaceLg), children: [
              BarChart(kind: 'weather', title: context.t('weather.rainfall'), bars: [
                for (final d in f.value.days)
                  Bar(MaterialLocalizations.of(context).narrowWeekdays[d.date.weekday % 7], d.rainMm,
                      caption: '${d.rainMm.round()}', color: Tokens.info),
              ]),
              Text(context.t('weather.forecast'), style: Tokens.title),
              for (final d in f.value.days)
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(Tokens.spaceMd),
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      Text(MaterialLocalizations.of(context).formatMediumDate(d.date), style: Tokens.label),
                      const SizedBox(height: Tokens.spaceXs),
                      Wrap(spacing: Tokens.spaceLg, children: [
                        Text('${context.t('weather.temperature')} ${d.tempMin.round()}–${d.tempMax.round()}°C'),
                        Text('${context.t('weather.rainfall')} ${d.rainMm.round()} mm'),
                        Text('${context.t('weather.humidity')} ${d.humidity}%'),
                        Text('${context.t('weather.wind')} ${d.windKph} km/h'),
                      ]),
                    ]),
                  ),
                ),
            ]),
          ),
        ]),
      ),
    );
  }
}

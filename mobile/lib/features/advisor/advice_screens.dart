import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/network/api_client.dart';
import '../../core/network/cached.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/async_view.dart';
import '../../core/widgets/chart.dart';
import '../../core/widgets/data_age.dart';
import 'models.dart';

/// Wraps a cached screen body with the offline banner.
Widget _cachedBody<T>(Cached<T> c, List<Widget> children) => Column(children: [
      if (c.fromCache) OfflineBanner(age: c.age),
      Expanded(child: ListView(padding: const EdgeInsets.all(Tokens.spaceLg), children: children)),
    ]);

String cropName(BuildContext context, String code) => context.tOr('crop.$code.name', code);

String money(BuildContext context, int poisha) => context.t('unit.taka', {'amount': takaDigits(poisha)});

/// 3.2 Seasonal forecast.
class SeasonScreen extends StatelessWidget {
  const SeasonScreen({super.key});

  static const seasons = ['aus', 'aman', 'boro'];

  @override
  Widget build(BuildContext context) {
    final advisor = AppScope.of(context).advisor;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('weather.seasonal.title'))),
      body: AsyncView<Cached<SeasonOutlook>>(
        load: advisor.outlook,
        builder: (context, o, _) => _cachedBody(o, [
          BarChart(
            kind: 'forecast',
            title: context.t('advisor.forecast.title'),
            bars: [
              for (final s in seasons)
                Bar(context.t('weather.season.$s'), (o.value.rainMm[s] ?? 0).toDouble(),
                    caption: '${o.value.rainMm[s] ?? 0} mm', color: s == o.value.current ? Tokens.info : Tokens.border),
            ],
          ),
        ]),
      ),
    );
  }
}

/// 3.2b Ranked recommendations for a field.
class RecommendationsScreen extends StatelessWidget {
  const RecommendationsScreen({super.key, required this.field});
  final String field;

  @override
  Widget build(BuildContext context) {
    final advisor = AppScope.of(context).advisor;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('advisor.recommendations.title'))),
      body: AsyncView<Cached<Recommendations>>(
        load: () => advisor.recommendations(field),
        builder: (context, r, _) => _cachedBody(r, [
          Text('${context.tOr('weather.season.${r.value.season}', r.value.season)} · ${r.value.rainMm} mm',
              style: Tokens.label),
          BarChart(
            kind: 'scores',
            title: context.t('advisor.recommendations.title'),
            bars: [
              for (final i in r.value.items.take(5))
                Bar(cropName(context, i.cropCode), i.score, caption: '${i.score.round()}'),
            ],
          ),
          for (final (n, i) in r.value.items.indexed)
            Card(
              child: ListTile(
                leading: CircleAvatar(backgroundColor: Tokens.primaryLight, child: Text('${n + 1}')),
                title: Text(cropName(context, i.cropCode), style: Tokens.title),
                subtitle: Text(context.t('advisor.recommendations.score', {'score': i.score.round()})),
                trailing: const Icon(Icons.chevron_right),
                onTap: () => context.push('/advisor/field/$field/crop/${i.cropCode}'),
              ),
            ),
          OutlinedButton.icon(
            onPressed: () => context.push('/advisor/field/$field/rotation'),
            icon: const Icon(Icons.autorenew),
            label: Text(context.t('advisor.rotation.title')),
          ),
        ]),
      ),
    );
  }
}

/// 3.3 Crop detail: criteria, expected yield, profit summary.
class CropDetailScreen extends StatelessWidget {
  const CropDetailScreen({super.key, required this.field, required this.code});
  final String field;
  final String code;

  static const criteria = ['soil', 'water', 'pest', 'market', 'seed'];

  @override
  Widget build(BuildContext context) {
    final advisor = AppScope.of(context).advisor;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('advisor.crop_detail.title'))),
      body: AsyncView<Cached<CropDetail>>(
        load: () => advisor.crop(field, code),
        builder: (context, d, _) {
          final rec = d.value.recommendation;
          final y = rec.yieldKg;
          final kg = context.t('unit.kg');
          return _cachedBody(d, [
            Text(cropName(context, code), style: Tokens.headline),
            Text(context.t('advisor.recommendations.score', {'score': rec.score.round()}), style: Tokens.label),
            const SizedBox(height: Tokens.spaceMd),
            for (final c in criteria)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: Tokens.spaceXs),
                child: Row(children: [
                  Expanded(flex: 2, child: Text(context.t('advisor.criteria.$c'))),
                  Expanded(flex: 3, child: LinearProgressIndicator(value: rec.criteria[c] ?? 0)),
                ]),
              ),
            BarChart(kind: 'yield', title: context.t('advisor.yield.title'), bars: [
              Bar(context.t('advisor.yield.low'), y.low.toDouble(), caption: '${y.low} $kg', color: Tokens.border),
              Bar(context.t('advisor.yield.likely'), y.likely.toDouble(), caption: '${y.likely} $kg'),
              Bar(context.t('advisor.yield.high'), y.high.toDouble(), caption: '${y.high} $kg', color: Tokens.primaryDark),
            ]),
            Card(
              child: ListTile(
                title: Text(context.t('advisor.roi.net'), style: Tokens.label),
                subtitle: Text(money(context, d.value.roi.net.likely), style: Tokens.headline),
                trailing: Text(context.t('advisor.roi.return', {'percent': (d.value.roi.returnBp / 100).round()})),
              ),
            ),
            FilledButton(
              onPressed: () => context.push('/advisor/field/$field/crop/$code/roi'),
              child: Text(context.t('advisor.roi.title')),
            ),
          ]);
        },
      ),
    );
  }
}

/// 3.3.2 Profit estimate with editable costs.
class RoiScreen extends StatefulWidget {
  const RoiScreen({super.key, required this.field, required this.code});
  final String field;
  final String code;

  static const costKeys = ['seed', 'fertiliser', 'pesticide', 'irrigation', 'labour'];

  @override
  State<RoiScreen> createState() => _RoiScreenState();
}

class _RoiScreenState extends State<RoiScreen> {
  final _costs = {for (final k in RoiScreen.costKeys) k: TextEditingController()};
  Roi? _roi;
  String? _error;

  @override
  void dispose() {
    for (final c in _costs.values) {
      c.dispose();
    }
    super.dispose();
  }

  void _show(Roi roi) {
    _roi = roi;
    for (final k in RoiScreen.costKeys) {
      _costs[k]!.text = '${((roi.costs[k] ?? 0) / 100).round()}';
    }
  }

  Future<void> _recalculate() async {
    final advisor = AppScope.of(context).advisor;
    final costs = {for (final k in RoiScreen.costKeys) k: ((int.tryParse(_costs[k]!.text.trim()) ?? 0) * 100)};
    try {
      final roi = await advisor.roi(widget.field, widget.code, costs);
      setState(() {
        _error = null;
        _show(roi);
      });
    } on ApiException catch (e) {
      setState(() => _error = e.messageKey);
    }
  }

  @override
  Widget build(BuildContext context) {
    final advisor = AppScope.of(context).advisor;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('advisor.roi.title'))),
      body: AsyncView<Cached<CropDetail>>(
        load: () async {
          final d = await advisor.crop(widget.field, widget.code);
          _show(d.value.roi);
          return d;
        },
        builder: (context, d, _) {
          final roi = _roi!;
          return _cachedBody(d, [
            Text(cropName(context, widget.code), style: Tokens.headline),
            BarChart(kind: 'roi', title: context.t('advisor.roi.title'), bars: [
              Bar(context.t('advisor.roi.gross'), roi.gross.likely.toDouble(), caption: money(context, roi.gross.likely)),
              Bar(context.t('advisor.roi.costs'), roi.totalCost.toDouble(),
                  caption: money(context, roi.totalCost), color: Tokens.warning),
              Bar(context.t('advisor.roi.net'), roi.net.likely.toDouble(),
                  caption: money(context, roi.net.likely), color: Tokens.success),
            ]),
            Text(context.t('advisor.roi.return', {'percent': (roi.returnBp / 100).round()}), style: Tokens.title),
            const SizedBox(height: Tokens.spaceMd),
            Text(context.t('advisor.roi.costs'), style: Tokens.title),
            for (final k in RoiScreen.costKeys)
              TextField(
                key: Key('cost-$k'),
                controller: _costs[k],
                keyboardType: TextInputType.number,
                decoration: InputDecoration(labelText: context.t('advisor.roi.cost.$k'), prefixText: '৳ '),
              ),
            if (_error case final e?)
              Text(context.tOr(e, context.t('errors.internal')), key: const Key('roi-error'),
                  style: Tokens.label.copyWith(color: Tokens.error)),
            const SizedBox(height: Tokens.spaceMd),
            FilledButton(onPressed: _recalculate, child: Text(context.t('common.save'))),
          ]);
        },
      ),
    );
  }
}

/// 3.3.3 Rotation plan with the soil-nitrogen chart.
class RotationScreen extends StatelessWidget {
  const RotationScreen({super.key, required this.field});
  final String field;

  @override
  Widget build(BuildContext context) {
    final advisor = AppScope.of(context).advisor;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('advisor.rotation.title'))),
      body: AsyncView<Cached<Rotation>>(
        load: () => advisor.rotation(field),
        builder: (context, r, _) => _cachedBody(r, [
          BarChart(kind: 'rotation', title: context.t('advisor.rotation.nitrogen'), bars: [
            for (final s in r.value.steps)
              Bar(context.tOr('weather.season.${s.season}', s.season), s.nitrogenAfter.toDouble(),
                  caption: '${s.nitrogenAfter}'),
          ]),
          if (r.value.deficit)
            Container(
              key: const Key('rotation-deficit'),
              color: Tokens.surfaceVariant,
              padding: const EdgeInsets.all(Tokens.spaceSm),
              child: Row(children: [
                const Icon(Icons.warning_amber, color: Tokens.warning),
                const SizedBox(width: Tokens.spaceSm),
                Expanded(child: Text(context.t('advisor.rotation.nitrogen'))),
              ]),
            ),
          for (final (n, s) in r.value.steps.indexed)
            Card(
              child: ListTile(
                leading: CircleAvatar(backgroundColor: Tokens.primaryLight, child: Text('${n + 1}')),
                title: Text(cropName(context, s.cropCode), style: Tokens.title),
                subtitle: Text(
                    '${context.tOr('weather.season.${s.season}', s.season)} · N ${s.nitrogenBefore} → ${s.nitrogenAfter} kg/ha'),
              ),
            ),
        ]),
      ),
    );
  }
}

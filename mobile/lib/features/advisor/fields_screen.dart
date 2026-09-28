import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/network/cached.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/async_view.dart';
import '../../core/widgets/data_age.dart';
import 'models.dart';

String areaLabel(BuildContext context, int valueMilli, String unit) {
  final v = valueMilli / 1000;
  return '${v == v.roundToDouble() ? v.round() : v} ${context.t('unit.$unit')}';
}

/// 3.0 My fields (3.0b empty state).
class FieldsScreen extends StatelessWidget {
  const FieldsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final advisor = AppScope.of(context).advisor;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('advisor.fields.title'))),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () => context.push('/advisor/new'),
        icon: const Icon(Icons.add),
        label: Text(context.t('advisor.fields.add')),
      ),
      body: AsyncView<Cached<List<FieldInfo>>>(
        load: advisor.fields,
        builder: (context, fields, _) => Column(children: [
          if (fields.fromCache) OfflineBanner(age: fields.age),
          Card(
            margin: const EdgeInsets.all(Tokens.spaceLg),
            child: ListTile(
              leading: const Icon(Icons.water_drop_outlined, color: Tokens.info),
              title: Text(context.t('advisor.forecast.title'), style: Tokens.title),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => context.push('/advisor/season'),
            ),
          ),
          Expanded(
            child: fields.value.isEmpty
                ? EmptyState(message: context.t('advisor.fields.empty'), icon: Icons.agriculture_outlined)
                : ListView(padding: const EdgeInsets.symmetric(horizontal: Tokens.spaceLg), children: [
                    for (final f in fields.value)
                      Card(
                        child: ListTile(
                          leading: const Icon(Icons.grass, color: Tokens.primary),
                          title: Text(f.name, style: Tokens.title),
                          subtitle: Text('${areaLabel(context, f.valueMilli, f.unit)} · ${context.t('irrigation.${f.irrigation}')}'),
                          trailing: const Icon(Icons.chevron_right),
                          onTap: () => context.push('/advisor/field/${f.id}'),
                        ),
                      ),
                  ]),
          ),
        ]),
      ),
    );
  }
}

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';
import '../home/home_repository.dart';
import '../onboarding/widgets.dart';
import 'models.dart';

/// 3.1 Field setup.
class FieldSetupScreen extends StatefulWidget {
  const FieldSetupScreen({super.key});

  @override
  State<FieldSetupScreen> createState() => _FieldSetupScreenState();
}

class _FieldSetupScreenState extends State<FieldSetupScreen> with BusyAction {
  final _name = TextEditingController();
  final _area = TextEditingController();
  final _ph = TextEditingController(text: '6.5');
  String _unit = areaUnits[1];
  String _irrigation = irrigationKinds[1];
  String _texture = soilTextures[2];
  LatLng? _location;

  @override
  void dispose() {
    _name.dispose();
    _area.dispose();
    _ph.dispose();
    super.dispose();
  }

  Future<void> _pickLocation() async {
    final p = await context.push<LatLng>('/advisor/new/location');
    if (p != null) {
      setState(() => _location = p);
    }
  }

  Future<void> _save() async {
    final area = double.tryParse(_area.text.trim()) ?? 0;
    final ph = double.tryParse(_ph.text.trim()) ?? 0;
    final location = _location;
    final problem = switch (()) {
      _ when _name.text.trim().isEmpty || ph < 3 || ph > 10 => 'errors.farm.invalid_field',
      _ when area <= 0 => 'errors.farm.invalid_area',
      _ when location == null => 'errors.farm.invalid_location',
      _ => null,
    };
    if (problem != null) {
      setState(() => error = context.t(problem));
      return;
    }
    final advisor = AppScope.of(context).advisor;
    await run(() async {
      final f = await advisor.createField(
          name: _name.text.trim(),
          unit: _unit,
          valueMilli: (area * 1000).round(),
          location: location!,
          irrigation: _irrigation,
          texture: _texture,
          ph: ph);
      if (mounted) {
        context.go('/advisor/field/${f.id}');
      }
    });
  }

  Widget _dropdown(String key, String label, String value, List<String> options, String prefix, ValueChanged<String> set) =>
      DropdownButtonFormField<String>(
        key: Key(key),
        initialValue: value,
        decoration: InputDecoration(labelText: label),
        items: [for (final o in options) DropdownMenuItem(value: o, child: Text(context.t('$prefix.$o')))],
        onChanged: (v) => setState(() => set(v!)),
      );

  @override
  Widget build(BuildContext context) {
    final loc = _location;
    return OnboardingFrame(
      title: context.t('advisor.setup.title'),
      body: ListView(children: [
        TextField(
            key: const Key('setup-name'),
            controller: _name,
            decoration: InputDecoration(labelText: context.t('advisor.setup.name'))),
        Row(children: [
          Expanded(
            child: TextField(
                key: const Key('setup-area'),
                controller: _area,
                keyboardType: const TextInputType.numberWithOptions(decimal: true),
                decoration: InputDecoration(labelText: context.t('advisor.setup.area'))),
          ),
          const SizedBox(width: Tokens.spaceMd),
          Expanded(child: _dropdown('setup-unit', '', _unit, areaUnits, 'unit', (v) => _unit = v)),
        ]),
        _dropdown('setup-irrigation', context.t('advisor.setup.irrigation'), _irrigation, irrigationKinds, 'irrigation',
            (v) => _irrigation = v),
        _dropdown('setup-soil', context.t('advisor.setup.soil'), _texture, soilTextures, 'soil', (v) => _texture = v),
        TextField(
            key: const Key('setup-ph'),
            controller: _ph,
            keyboardType: const TextInputType.numberWithOptions(decimal: true),
            decoration: InputDecoration(labelText: context.t('advisor.setup.ph'))),
        const SizedBox(height: Tokens.spaceMd),
        Card(
          child: ListTile(
            key: const Key('setup-location'),
            leading: const Icon(Icons.location_on_outlined, color: Tokens.primary),
            title: Text(context.t('advisor.gps.title')),
            subtitle: loc == null ? null : Text('${loc.lat.toStringAsFixed(4)}, ${loc.lng.toStringAsFixed(4)}'),
            trailing: const Icon(Icons.chevron_right),
            onTap: _pickLocation,
          ),
        ),
      ]),
      error: error,
      primaryLabel: context.t('common.save'),
      onPrimary: busy ? null : _save,
    );
  }
}

/// 3.1b GPS pin: the phone's position, or coordinates typed in.
class GpsScreen extends StatefulWidget {
  const GpsScreen({super.key});

  @override
  State<GpsScreen> createState() => _GpsScreenState();
}

class _GpsScreenState extends State<GpsScreen> with BusyAction {
  final _lat = TextEditingController();
  final _lng = TextEditingController();
  bool _loaded = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_loaded) {
      _loaded = true;
      _prefill();
    }
  }

  Future<void> _prefill() async => _fill(await AppScope.of(context).home.location());

  void _fill(LatLng p) {
    _lat.text = p.lat.toStringAsFixed(5);
    _lng.text = p.lng.toStringAsFixed(5);
  }

  @override
  void dispose() {
    _lat.dispose();
    _lng.dispose();
    super.dispose();
  }

  Future<void> _useMine() async {
    setState(() => error = null);
    final p = await AppScope.of(context).location.current();
    if (!mounted) {
      return;
    }
    if (p == null) {
      setState(() => error = context.t('errors.farm.invalid_location'));
    } else {
      setState(() => _fill(p));
    }
  }

  void _done() {
    final lat = double.tryParse(_lat.text.trim());
    final lng = double.tryParse(_lng.text.trim());
    if (lat == null || lng == null || lat.abs() > 90 || lng.abs() > 180) {
      setState(() => error = context.t('errors.farm.invalid_location'));
      return;
    }
    context.pop<LatLng>((lat: lat, lng: lng));
  }

  @override
  Widget build(BuildContext context) {
    return OnboardingFrame(
      title: context.t('advisor.gps.title'),
      body: ListView(children: [
        OutlinedButton.icon(
          onPressed: _useMine,
          icon: const Icon(Icons.my_location),
          label: Text(context.t('advisor.gps.use_location')),
        ),
        TextField(
            key: const Key('gps-lat'),
            controller: _lat,
            decoration: InputDecoration(labelText: context.t('advisor.gps.latitude'))),
        TextField(
            key: const Key('gps-lng'),
            controller: _lng,
            decoration: InputDecoration(labelText: context.t('advisor.gps.longitude'))),
      ]),
      error: error,
      primaryLabel: context.t('common.done'),
      onPrimary: _done,
    );
  }
}

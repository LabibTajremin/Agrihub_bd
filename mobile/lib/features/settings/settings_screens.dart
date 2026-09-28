import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/models/offline_model.dart';
import '../../core/network/api_client.dart';
import '../../core/theme/tokens.dart';
import '../onboarding/auth_repository.dart';
import '../onboarding/widgets.dart';

/// Shown in Settings → version.
const appVersion = '0.1.0';

/// The agriculture helpline (Krishi Call Centre).
const helpline = '16123';

/// 5.0 Settings.
class SettingsScreen extends StatefulWidget {
  const SettingsScreen({super.key});

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  Profile? _profile;
  bool _signedIn = false;
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
    final auth = AppScope.of(context).auth;
    final signedIn = await auth.signedIn;
    final profile = await auth.cachedProfile();
    if (mounted) {
      setState(() {
        _signedIn = signedIn;
        _profile = profile;
      });
    }
  }

  Future<void> _signOut() async {
    await AppScope.of(context).auth.signOut();
    if (mounted) {
      context.go('/onboarding/phone');
    }
  }

  Future<void> _open(String path) async {
    await context.push(path);
    await _load();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = L10nScope.of(context);
    final guest = !_signedIn || _profile?.role == 'guest';
    final name = _profile?.name ?? '';
    Widget tile(IconData icon, String title, {String? subtitle, VoidCallback? onTap, Key? key}) => ListTile(
          key: key,
          leading: Icon(icon, color: Tokens.primary),
          title: Text(title, style: Tokens.title),
          subtitle: subtitle == null ? null : Text(subtitle),
          trailing: onTap == null ? null : const Icon(Icons.chevron_right),
          onTap: onTap,
        );
    return Scaffold(
      appBar: AppBar(title: Text(context.t('settings.title'))),
      body: ListView(children: [
        if (guest)
          tile(Icons.login, context.t('settings.sign_in'), onTap: () => context.go('/onboarding/phone'))
        else
          tile(Icons.person_outline, context.t('settings.profile'),
              subtitle: name.isEmpty ? null : name, onTap: () => _open('/settings/profile')),
        tile(Icons.translate, context.t('settings.language'),
            subtitle: l10n.current.language.nativeName, onTap: () => _open('/settings/language')),
        tile(Icons.download_for_offline_outlined, context.t('settings.offline_models'),
            onTap: () => _open('/settings/models')),
        tile(Icons.support_agent, context.t('settings.expert_help'), onTap: () => _open('/settings/expert')),
        tile(Icons.privacy_tip_outlined, context.t('settings.privacy'), onTap: () => _open('/settings/privacy')),
        if (_signedIn) tile(Icons.logout, context.t('settings.sign_out'), key: const Key('sign-out'), onTap: _signOut),
        tile(Icons.info_outline, context.t('settings.version', {'version': appVersion})),
      ]),
    );
  }
}

/// 5.1 Language selector; the choice also goes to the profile when signed in.
class LanguageSettingsScreen extends StatelessWidget {
  const LanguageSettingsScreen({super.key});

  Future<void> _choose(BuildContext context, String code) async {
    final services = AppScope.of(context);
    await services.l10n.setLanguage(code);
    if (await services.auth.signedIn) {
      try {
        await services.auth.updateProfile(language: code);
      } on ApiException {
        // offline: the local choice stands; the profile catches up on the next save
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final selected = L10nScope.of(context).current.language.code;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('settings.language'))),
      body: ListView(children: [
        for (final lang in languages)
          ListTile(
            key: Key('settings-lang-${lang.code}'),
            title: Text(lang.nativeName, style: Tokens.title),
            trailing: lang.code == selected ? const Icon(Icons.check_circle, color: Tokens.primary) : null,
            onTap: () => _choose(context, lang.code),
          ),
      ]),
    );
  }
}

/// 5.2 Offline model manager.
class ModelsScreen extends StatelessWidget {
  const ModelsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final models = AppScope.of(context).models;
    return Scaffold(
      appBar: AppBar(title: Text(context.t('settings.offline_models'))),
      body: ListenableBuilder(
        listenable: models,
        builder: (context, _) => ListView(padding: const EdgeInsets.all(Tokens.spaceLg), children: [
          Text(context.t('settings.offline_models.body'), style: Tokens.body),
          Card(
            child: ListTile(
              leading: const Icon(Icons.eco, color: Tokens.primary),
              title: Text(context.t('onboarding.model.title'), style: Tokens.title),
              subtitle: Text(switch (models.status) {
                ModelStatus.ready => '${context.t('settings.offline_models.installed')} · ${models.manifest.version}',
                ModelStatus.downloading =>
                  context.t('onboarding.model.downloading', {'percent': (models.progress * 100).round()}),
                ModelStatus.absent => models.manifest.sizeLabel,
              }),
              trailing: switch (models.status) {
                ModelStatus.ready =>
                  TextButton(onPressed: models.remove, child: Text(context.t('settings.offline_models.remove'))),
                ModelStatus.downloading => SizedBox.square(
                    dimension: Tokens.spaceXl, child: CircularProgressIndicator(value: models.progress, strokeWidth: 2)),
                ModelStatus.absent =>
                  TextButton(onPressed: models.download, child: Text(context.t('onboarding.model.download'))),
              },
            ),
          ),
        ]),
      ),
    );
  }
}

/// 5.3 Expert help: the free agriculture helpline.
class ExpertScreen extends StatelessWidget {
  const ExpertScreen({super.key});

  @override
  Widget build(BuildContext context) => Scaffold(
        appBar: AppBar(title: Text(context.t('settings.expert_help'))),
        body: Padding(
          padding: const EdgeInsets.all(Tokens.spaceXl),
          child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            const Icon(Icons.support_agent, size: Tokens.spaceXxxl * 2, color: Tokens.primary),
            Text(context.t('settings.expert_help.body'), style: Tokens.body, textAlign: TextAlign.center),
            const SizedBox(height: Tokens.spaceXl),
            FilledButton.icon(
              onPressed: () async {
                // No dialer plugin: the number is copied for the phone app.
                await Clipboard.setData(const ClipboardData(text: helpline));
                if (context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(helpline)));
                }
              },
              icon: const Icon(Icons.call),
              label: Text(context.t('settings.expert_help.call')),
            ),
          ]),
        ),
      );
}

/// 5.4 Profile (signed-in farmers).
class ProfileSettingsScreen extends StatefulWidget {
  const ProfileSettingsScreen({super.key});

  @override
  State<ProfileSettingsScreen> createState() => _ProfileSettingsScreenState();
}

class _ProfileSettingsScreenState extends State<ProfileSettingsScreen> with BusyAction {
  final _name = TextEditingController();
  final _district = TextEditingController();
  bool _loaded = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_loaded) {
      _loaded = true;
      AppScope.of(context).auth.cachedProfile().then((p) {
        _name.text = p?.name ?? '';
        _district.text = p?.district ?? '';
      });
    }
  }

  @override
  void dispose() {
    _name.dispose();
    _district.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final auth = AppScope.of(context).auth;
    await run(() async {
      await auth.updateProfile(name: _name.text.trim(), district: _district.text.trim());
      if (mounted) {
        context.pop();
      }
    });
  }

  @override
  Widget build(BuildContext context) => OnboardingFrame(
        title: context.t('settings.profile'),
        body: ListView(children: [
          TextField(
              key: const Key('settings-name'),
              controller: _name,
              decoration: InputDecoration(labelText: context.t('onboarding.profile.name'))),
          TextField(
              key: const Key('settings-district'),
              controller: _district,
              decoration: InputDecoration(labelText: context.t('onboarding.profile.district'))),
        ]),
        error: error,
        primaryLabel: context.t('common.save'),
        onPrimary: busy ? null : _save,
      );
}

/// 5.5 Privacy.
class PrivacyScreen extends StatelessWidget {
  const PrivacyScreen({super.key});

  @override
  Widget build(BuildContext context) => Scaffold(
        appBar: AppBar(title: Text(context.t('settings.privacy'))),
        body: Padding(
          padding: const EdgeInsets.all(Tokens.spaceXl),
          child: Text(context.t('settings.privacy.body'), style: Tokens.body),
        ),
      );
}

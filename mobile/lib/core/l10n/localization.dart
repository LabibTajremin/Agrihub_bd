import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';

import '../network/api_client.dart';
import '../storage/local_db.dart';

/// A supported language. Layout direction comes from [isRtl], never from a
/// hardcoded list of codes.
class Language {
  const Language(this.code, this.nativeName, {this.isRtl = false});
  final String code;
  final String nativeName;
  final bool isRtl;
}

/// Bundled language list (the server's /v1/i18n/languages mirrors it).
const languages = <Language>[
  Language('bn', 'বাংলা'),
  Language('en', 'English'),
  Language('hi', 'हिन्दी'),
  Language('es', 'Español'),
  Language('fr', 'Français'),
  Language('ar', 'العربية', isRtl: true),
  Language('pt', 'Português'),
];

/// Raised in strict (debug/test) mode for a key missing from the dictionary.
class MissingTranslation implements Exception {
  const MissingTranslation(this.key);
  final String key;
  @override
  String toString() => 'Missing translation: $key';
}

/// An immutable dictionary for one language.
class LanguageSnapshot {
  const LanguageSnapshot(this.language, this.version, this.entries);
  final Language language;
  final int version;
  final Map<String, String> entries;

  bool get isRtl => language.isRtl;

  bool has(String key) => entries.containsKey(key);
  TextDirection get direction => isRtl ? TextDirection.rtl : TextDirection.ltr;

  /// Looks up [key] and fills `{param}` placeholders. A missing key throws in
  /// strict mode (tests/debug) and falls back to the key itself in release.
  String translate(String key, Map<String, Object?> params, {required bool strict}) {
    var v = entries[key];
    if (v == null) {
      if (strict) {
        throw MissingTranslation(key);
      }
      return key;
    }
    params.forEach((k, p) => v = v!.replaceAll('{$k}', '$p'));
    return v!;
  }
}

/// Loads dictionaries (bundled seed → persisted overlay → server delta) and
/// publishes snapshots; switching language never requires a restart.
class LocalizationRepository extends ChangeNotifier {
  LocalizationRepository({required this.bundle, required this.db, this.api, bool? strict}) : strict = strict ?? kDebugMode;

  final AssetBundle bundle;
  final LocalDb db;
  final ApiClient? api;
  final bool strict;
  final _controller = StreamController<LanguageSnapshot>.broadcast();
  LanguageSnapshot? _current;

  LanguageSnapshot get current => _current!;

  /// Emits every published snapshot.
  Stream<LanguageSnapshot> watch() => _controller.stream;

  static Language byCode(String code) => languages.firstWhere((l) => l.code == code, orElse: () => languages.first);

  /// Restores the saved language (Bangla by default).
  Future<void> init() async => _publish(await _load(byCode((await db.read('i18n.lang') as String?) ?? 'bn')));

  /// Switches language at runtime and remembers the choice.
  Future<void> setLanguage(String code) async {
    final snap = await _load(byCode(code));
    await db.write('i18n.lang', snap.language.code);
    _publish(snap);
  }

  Future<LanguageSnapshot> _load(Language lang) async {
    final saved = await db.read('i18n.dict.${lang.code}') as Map?;
    if (saved != null) {
      return LanguageSnapshot(lang, saved['version']! as int, (saved['entries']! as Map).cast());
    }
    final raw = await bundle.loadString('assets/i18n/${lang.code}.json');
    return LanguageSnapshot(lang, 0, (jsonDecode(raw) as Map).cast());
  }

  /// Pulls the delta since the current version. Returns false when offline.
  Future<bool> sync() async {
    final snap = current;
    try {
      final res = await api!.get('/v1/i18n/${snap.language.code}', query: {'since': snap.version});
      final merged = {...snap.entries, ...(res['entries']! as Map).cast<String, String>()};
      final version = res['version']! as int;
      await db.write('i18n.dict.${snap.language.code}', {'version': version, 'entries': merged});
      if (_current!.language.code == snap.language.code) {
        _publish(LanguageSnapshot(snap.language, version, merged));
      }
      return true;
    } on ApiException {
      return false;
    }
  }

  String t(String key, [Map<String, Object?> params = const {}]) => current.translate(key, params, strict: strict);

  void _publish(LanguageSnapshot s) {
    _current = s;
    _controller.add(s);
    notifyListeners();
  }

  @override
  void dispose() {
    _controller.close();
    super.dispose();
  }
}

/// Makes the repository available below the app root; widgets depending on it
/// rebuild when the language changes.
class L10nScope extends InheritedNotifier<LocalizationRepository> {
  const L10nScope({super.key, required LocalizationRepository repository, required super.child}) : super(notifier: repository);

  static LocalizationRepository of(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<L10nScope>()!.notifier!;
}

extension Translate on BuildContext {
  /// `context.t('home.quickscan.title')`
  String t(String key, [Map<String, Object?> params = const {}]) => L10nScope.of(this).t(key, params);

  /// Translates an API failure; unknown server keys fall back to a generic message.
  String tError(ApiException e) {
    final l10n = L10nScope.of(this);
    return l10n.t(l10n.current.has(e.messageKey) ? e.messageKey : 'errors.internal');
  }
}

import 'dart:convert';
import 'dart:io';

import 'package:agrismart/core/l10n/localization.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/fakes.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('defaults to Bangla, switches at runtime and remembers', () async {
    final kit = await TestKit.create(lang: 'bn');
    final l = kit.services.l10n;
    expect(l.current.language.code, 'bn');
    expect(l.t('home.quickscan.title'), 'পাতা স্ক্যান করুন');
    final seen = <String>[];
    final sub = l.watch().listen((s) => seen.add(s.language.code));
    await l.setLanguage('ar');
    expect(l.current.isRtl, isTrue);
    expect(l.current.direction, TextDirection.rtl);
    await l.setLanguage('xx'); // unknown falls back to the default
    await Future<void>.delayed(Duration.zero);
    expect(seen, ['ar', 'bn']);
    await sub.cancel();
    expect(await kit.services.db.read('i18n.lang'), 'bn');
    expect(LocalizationRepository.byCode('ar').nativeName, 'العربية');
  });

  test('delta sync merges, persists and survives restart', () async {
    final kit = await TestKit.create();
    kit.server.json('GET', '/v1/i18n/en', {'lang': 'en', 'version': 42, 'full': false, 'entries': {'home.quickscan.title': 'Scan now'}});
    expect(await kit.services.l10n.sync(), isTrue);
    expect(kit.server.seen.single.query['since'], 0);
    expect(kit.services.l10n.t('home.quickscan.title'), 'Scan now');
    expect(kit.services.l10n.current.version, 42);
    final again = LocalizationRepository(bundle: kit.services.l10n.bundle, db: kit.services.db);
    await again.init();
    expect(again.current.version, 42);
    expect(again.t('home.quickscan.title'), 'Scan now');
    kit.server.offline = true;
    expect(await kit.services.l10n.sync(), isFalse);
  });

  test('language switched during sync is not overwritten', () async {
    final kit = await TestKit.create();
    final l = kit.services.l10n;
    kit.server.on('GET', '/v1/i18n/en', (_) async {
      await l.setLanguage('fr');
      return const Reply(200, {'version': 1, 'entries': <String, String>{}});
    });
    await l.sync();
    expect(l.current.language.code, 'fr');
  });

  test('missing keys throw when strict and fall back to the key otherwise', () async {
    final strict = await TestKit.create();
    expect(() => strict.services.l10n.t('nope.key'), throwsA(isA<MissingTranslation>()));
    expect(const MissingTranslation('a.b').toString(), contains('a.b'));
    final lenient = await TestKit.create(strict: false);
    expect(lenient.services.l10n.t('nope.key'), 'nope.key');
    expect(lenient.services.l10n.t('home.greeting', {'name': 'Rahim'}), 'Hello, Rahim');
    lenient.services.l10n.dispose();
  });

  // Every key referenced from Dart source must exist in all seven dictionaries.
  test('all keys used in lib/ exist in every dictionary', () {
    final used = <String>{};
    final pattern = RegExp(r'''['"]((?:[a-z][a-z0-9_]*)(?:\.[a-z][a-z0-9_]*)+)['"]''');
    final dicts = {
      for (final f in Directory('assets/i18n').listSync().whereType<File>())
        f.uri.pathSegments.last: (jsonDecode(f.readAsStringSync()) as Map).cast<String, String>(),
    };
    expect(dicts.length, 7);
    final known = dicts['en.json']!.keys.toSet();
    for (final f in Directory('lib').listSync(recursive: true).whereType<File>().where((f) => f.path.endsWith('.dart'))) {
      for (final m in pattern.allMatches(f.readAsStringSync())) {
        final k = m.group(1)!;
        final prefix = k.split('.').first;
        if (known.any((x) => x.startsWith('$prefix.'))) {
          used.add(k);
        }
      }
    }
    for (final entry in dicts.entries) {
      for (final k in used.where((k) => !k.endsWith('.'))) {
        if (!k.contains('{') && known.contains(k)) {
          expect(entry.value.containsKey(k), isTrue, reason: '${entry.key} misses $k');
        }
      }
    }
    final unknown = used.where((k) => !known.contains(k) && !_dynamicPrefix(k)).toList();
    expect(unknown, isEmpty, reason: 'keys used in code but missing from the dictionaries');
  });
}

// Prefixes completed at runtime (e.g. 'crop.' + code + '.name') are checked by backend tests.
bool _dynamicPrefix(String k) => k.startsWith('assets.') || k.endsWith('.json') || k.endsWith('.dart');

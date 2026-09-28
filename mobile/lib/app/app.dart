import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:go_router/go_router.dart';

import '../core/l10n/localization.dart';
import '../core/theme/theme.dart';
import 'router.dart';
import 'services.dart';

/// The application root. It listens to the localization repository, so a
/// language change rebuilds the tree in place — no restart.
class AgriSmartApp extends StatefulWidget {
  const AgriSmartApp({super.key, required this.services, this.router});
  final AppServices services;
  final GoRouter? router;

  @override
  State<AgriSmartApp> createState() => _AgriSmartAppState();
}

class _AgriSmartAppState extends State<AgriSmartApp> {
  late final GoRouter _router = widget.router ?? buildRouter();

  @override
  Widget build(BuildContext context) {
    final l10n = widget.services.l10n;
    return AppScope(
      services: widget.services,
      child: L10nScope(
        repository: l10n,
        child: ListenableBuilder(
          listenable: l10n,
          builder: (context, _) {
            final snap = l10n.current;
            return MaterialApp.router(
              title: 'AgriSmart',
              debugShowCheckedModeBanner: false,
              theme: AppTheme.light(),
              locale: Locale(snap.language.code),
              supportedLocales: [for (final l in languages) Locale(l.code)],
              localizationsDelegates: GlobalMaterialLocalizations.delegates,
              routerConfig: _router,
              // Direction follows the language's is_rtl flag, never a code list.
              builder: (context, child) => Directionality(textDirection: snap.direction, child: child!),
            );
          },
        ),
      ),
    );
  }
}

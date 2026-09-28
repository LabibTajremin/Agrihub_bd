import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../core/l10n/localization.dart';
import '../core/theme/tokens.dart';
import 'shell.dart';

/// Tab roots, in navigation-bar order.
const tabPaths = ['/home', '/doctor', '/advisor', '/voice', '/settings'];

/// Placeholder body until a feature phase provides the real screen.
class ComingSoon extends StatelessWidget {
  const ComingSoon({super.key, required this.titleKey});
  final String titleKey;

  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(Tokens.spaceXl),
          child: Text(context.t(titleKey), style: Tokens.headline),
        ),
      );
}

/// Builds the app router. Feature phases add their routes here.
GoRouter buildRouter({String initialLocation = '/home'}) {
  return GoRouter(
    initialLocation: initialLocation,
    routes: [
      ShellRoute(
        builder: (context, state, child) {
          final i = tabPaths.indexWhere((p) => state.uri.path.startsWith(p));
          return Shell(index: i, onSelect: (n) => context.go(tabPaths[n]), child: child);
        },
        routes: [
          GoRoute(path: '/home', builder: (_, _) => const ComingSoon(titleKey: 'home.quickscan.title')),
          GoRoute(path: '/doctor', builder: (_, _) => const ComingSoon(titleKey: 'diagnosis.camera.title')),
          GoRoute(path: '/advisor', builder: (_, _) => const ComingSoon(titleKey: 'advisor.fields.title')),
          GoRoute(path: '/voice', builder: (_, _) => const ComingSoon(titleKey: 'voice.title')),
          GoRoute(path: '/settings', builder: (_, _) => const ComingSoon(titleKey: 'settings.title')),
        ],
      ),
    ],
  );
}

import 'package:flutter/material.dart';

import '../core/l10n/localization.dart';
import '../core/theme/tokens.dart';

/// The bottom-navigation shell hosting the five product modules.
class Shell extends StatelessWidget {
  const Shell({super.key, required this.index, required this.onSelect, required this.child});
  final int index;
  final ValueChanged<int> onSelect;
  final Widget child;

  static const tabs = <(String, IconData)>[
    ('home.nav.home', Icons.home_outlined),
    ('home.nav.doctor', Icons.eco_outlined),
    ('home.nav.advisor', Icons.insights_outlined),
    ('home.nav.voice', Icons.mic_none),
    ('home.nav.settings', Icons.settings_outlined),
  ];

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(child: child),
      bottomNavigationBar: NavigationBar(
        selectedIndex: index,
        onDestinationSelected: onSelect,
        indicatorColor: Tokens.primaryLight,
        destinations: [
          for (final (key, icon) in tabs) NavigationDestination(icon: Icon(icon), label: context.t(key)),
        ],
      ),
    );
  }
}

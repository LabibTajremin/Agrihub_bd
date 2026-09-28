import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';

/// 00.3–00.5 Value slides.
class SlidesScreen extends StatefulWidget {
  const SlidesScreen({super.key});

  static const slides = <(String, String, IconData)>[
    ('onboarding.slide1.title', 'onboarding.slide1.body', Icons.camera_alt_outlined),
    ('onboarding.slide2.title', 'onboarding.slide2.body', Icons.healing_outlined),
    ('onboarding.slide3.title', 'onboarding.slide3.body', Icons.insights_outlined),
  ];

  @override
  State<SlidesScreen> createState() => _SlidesScreenState();
}

class _SlidesScreenState extends State<SlidesScreen> {
  final _pages = PageController();
  int _page = 0;

  @override
  void dispose() {
    _pages.dispose();
    super.dispose();
  }

  void _next() {
    if (_page == SlidesScreen.slides.length - 1) {
      context.go('/onboarding/phone');
      return;
    }
    _pages.nextPage(duration: const Duration(milliseconds: 250), curve: Curves.easeOut);
  }

  @override
  Widget build(BuildContext context) {
    final last = _page == SlidesScreen.slides.length - 1;
    return Scaffold(
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(Tokens.spaceXl),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Align(
                alignment: AlignmentDirectional.centerEnd,
                child: TextButton(onPressed: () => context.go('/onboarding/phone'), child: Text(context.t('common.skip'))),
              ),
              Expanded(
                child: PageView(
                  controller: _pages,
                  onPageChanged: (p) => setState(() => _page = p),
                  children: [
                    for (final (title, body, icon) in SlidesScreen.slides)
                      Column(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          Icon(icon, size: Tokens.spaceXxxl * 2, color: Tokens.primary),
                          const SizedBox(height: Tokens.spaceXl),
                          Text(context.t(title), style: Tokens.headline, textAlign: TextAlign.center),
                          const SizedBox(height: Tokens.spaceMd),
                          Text(context.t(body), style: Tokens.body, textAlign: TextAlign.center),
                        ],
                      ),
                  ],
                ),
              ),
              Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  for (var i = 0; i < SlidesScreen.slides.length; i++)
                    Container(
                      key: Key('dot.$i'),
                      margin: const EdgeInsets.all(Tokens.spaceXs),
                      width: Tokens.spaceSm,
                      height: Tokens.spaceSm,
                      decoration: BoxDecoration(
                          shape: BoxShape.circle, color: i == _page ? Tokens.primary : Tokens.border),
                    ),
                ],
              ),
              const SizedBox(height: Tokens.spaceLg),
              FilledButton(onPressed: _next, child: Text(context.t(last ? 'common.continue' : 'common.next'))),
            ],
          ),
        ),
      ),
    );
  }
}

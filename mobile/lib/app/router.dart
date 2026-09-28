import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../core/l10n/localization.dart';
import '../core/theme/tokens.dart';
import '../features/doctor/camera_screen.dart';
import '../features/doctor/preview_screen.dart';
import '../features/doctor/result_screen.dart';
import '../features/doctor/sync_screen.dart';
import '../features/home/alerts_screen.dart';
import '../features/home/dashboard_screen.dart';
import '../features/home/scan_list_screen.dart';
import '../features/home/weather_screen.dart';
import '../features/onboarding/language_screen.dart';
import '../features/onboarding/model_screen.dart';
import '../features/onboarding/otp_screen.dart';
import '../features/onboarding/permissions_screen.dart';
import '../features/onboarding/phone_screen.dart';
import '../features/onboarding/profile_screen.dart';
import '../features/onboarding/slides_screen.dart';
import '../features/onboarding/splash_screen.dart';
import 'services.dart';
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
GoRouter buildRouter({String initialLocation = '/'}) {
  return GoRouter(
    initialLocation: initialLocation,
    routes: [
      GoRoute(path: '/', builder: (_, _) => const SplashScreen()),
      GoRoute(path: '/onboarding/language', builder: (_, _) => const LanguageScreen()),
      GoRoute(path: '/onboarding/slides', builder: (_, _) => const SlidesScreen()),
      GoRoute(path: '/onboarding/phone', builder: (_, _) => const PhoneScreen()),
      GoRoute(
          path: '/onboarding/otp',
          builder: (_, state) => OtpScreen(phone: state.uri.queryParameters['phone'] ?? '')),
      GoRoute(path: '/onboarding/profile', builder: (_, _) => const ProfileScreen()),
      GoRoute(path: '/onboarding/permissions', builder: (_, _) => const PermissionsScreen()),
      GoRoute(path: '/onboarding/model', builder: (_, _) => const ModelScreen()),
      ShellRoute(
        builder: (context, state, child) {
          final i = tabPaths.indexWhere((p) => state.uri.path.startsWith(p));
          return Shell(index: i, onSelect: (n) => context.go(tabPaths[n]), child: child);
        },
        routes: [
          GoRoute(path: '/home', builder: (_, _) => const DashboardScreen(), routes: [
            GoRoute(path: 'weather', builder: (_, _) => const WeatherScreen()),
            GoRoute(path: 'alerts', builder: (_, _) => const AlertsScreen(), routes: [
              GoRoute(path: ':id', builder: (_, state) => AlertDetailScreen(id: state.pathParameters['id']!)),
            ]),
            GoRoute(path: 'history', builder: (_, _) => const ScanListScreen(saved: false)),
            GoRoute(path: 'saved', builder: (_, _) => const ScanListScreen(saved: true)),
          ]),
          GoRoute(path: '/doctor', builder: (_, _) => const CameraScreen(), routes: [
            GoRoute(
                path: 'preview',
                redirect: (context, _) => AppScope.of(context).doctor.photo == null ? '/doctor' : null,
                builder: (_, _) => const PreviewScreen()),
            GoRoute(
                path: 'analysing',
                redirect: (context, _) => AppScope.of(context).doctor.photo == null ? '/doctor' : null,
                builder: (_, _) => const AnalysingScreen()),
            GoRoute(path: 'result/:id', builder: (_, state) => ResultScreen(id: state.pathParameters['id']!)),
            GoRoute(path: 'sync', builder: (_, _) => const SyncScreen()),
          ]),
          GoRoute(path: '/advisor', builder: (_, _) => const ComingSoon(titleKey: 'advisor.fields.title')),
          GoRoute(path: '/voice', builder: (_, _) => const ComingSoon(titleKey: 'voice.title')),
          GoRoute(path: '/settings', builder: (_, _) => const ComingSoon(titleKey: 'settings.title'), routes: [
            GoRoute(path: 'expert', builder: (_, _) => const ComingSoon(titleKey: 'settings.expert_help')),
          ]),
        ],
      ),
    ],
  );
}

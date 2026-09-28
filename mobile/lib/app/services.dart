import 'package:flutter/widgets.dart';

import '../core/clock.dart';
import '../core/l10n/localization.dart';
import '../core/models/offline_model.dart';
import '../core/network/api_client.dart';
import '../core/network/cached.dart';
import '../core/network/connectivity.dart';
import '../core/storage/local_db.dart';
import '../core/storage/token_store.dart';
import '../core/sync/sync_queue.dart';
import '../features/doctor/camera_gateway.dart';
import '../features/doctor/diagnosis_engine.dart';
import '../features/doctor/doctor_repository.dart';
import '../features/home/home_repository.dart';
import '../features/onboarding/auth_repository.dart';
import '../features/onboarding/onboarding_store.dart';

/// The app's service container (manual constructor injection — no reflection,
/// no generated code). Production builds it in bootstrap; tests build it from
/// fakes.
class AppServices {
  AppServices({
    required this.clock,
    required this.ids,
    required this.db,
    required this.tokens,
    required this.api,
    required this.connectivity,
    required this.l10n,
    required this.sync,
    required this.auth,
    required this.models,
    this.camera = PluginCamera.new,
    this.engine = const StubDiagnosisEngine(),
  }) : onboarding = OnboardingStore(db);

  final Clock clock;
  final IdGenerator ids;
  final LocalDb db;
  final TokenStore tokens;
  final ApiClient api;
  final ConnectivityService connectivity;
  final LocalizationRepository l10n;
  final SyncQueue sync;
  final AuthRepository auth;
  final OfflineModelManager models;
  final OnboardingStore onboarding;

  /// Opens a camera for the viewfinder (a fresh gateway per screen).
  final CameraGateway Function() camera;
  final DiagnosisEngine engine;
  late final reader = CachedReader(api: api, db: db, clock: clock, connectivity: connectivity);
  late final home = HomeRepository(reader: reader, api: api, db: db);
  late final doctor = DoctorRepository(
      db: db, api: api, sync: sync, ids: ids, clock: clock, engine: engine, language: () => l10n.current.language.code);
}

/// Exposes [AppServices] to the widget tree.
class AppScope extends InheritedWidget {
  const AppScope({super.key, required this.services, required super.child});
  final AppServices services;

  static AppServices of(BuildContext context) => context.getInheritedWidgetOfExactType<AppScope>()!.services;

  @override
  bool updateShouldNotify(AppScope oldWidget) => services != oldWidget.services;
}

import '../../core/storage/local_db.dart';

/// First-run progress flags, persisted locally so a restart resumes cleanly.
class OnboardingStore {
  const OnboardingStore(this.db);
  final LocalDb db;

  Future<bool> get completed async => (await db.read('firstrun.done')) == true;
  Future<void> complete() => db.write('firstrun.done', true);

  /// The primer was shown; the OS prompts themselves appear on first use of
  /// the camera, location and microphone.
  Future<bool> get permissionsPrimed async => (await db.read('firstrun.permissions')) == true;
  Future<void> primePermissions() => db.write('firstrun.permissions', true);
}

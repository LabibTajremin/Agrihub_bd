import '../../core/network/api_client.dart';
import '../../core/storage/local_db.dart';
import '../../core/storage/token_store.dart';

final _bdMobile = RegExp(r'^01[3-9][0-9]{8}$');
final _e164 = RegExp(r'^\+[1-9][0-9]{7,14}$');

/// Mirrors the server's phone normalization: Bangladeshi local numbers
/// (01XXXXXXXXX, 8801XXXXXXXXX) become +8801XXXXXXXXX. Returns null when the
/// input is not a valid number.
String? normalizePhone(String raw) {
  var s = raw.trim().replaceAll(RegExp(r'[\s\-()]'), '');
  if (_bdMobile.hasMatch(s)) {
    s = '+88$s';
  } else if (s.startsWith('880') && _bdMobile.hasMatch(s.substring(2))) {
    s = '+$s';
  }
  return _e164.hasMatch(s) ? s : null;
}

/// The signed-in user's profile.
class Profile {
  const Profile({required this.id, required this.name, required this.district, required this.language, required this.role});
  final String id;
  final String name;
  final String district;
  final String language;
  final String role;

  factory Profile.fromJson(Map<String, Object?> j) => Profile(
      id: j['id']! as String,
      name: (j['name'] as String?) ?? '',
      district: (j['district'] as String?) ?? '',
      language: (j['language'] as String?) ?? 'bn',
      role: j['role']! as String);

  Map<String, Object?> toJson() => {'id': id, 'name': name, 'district': district, 'language': language, 'role': role};
}

/// A pending OTP challenge. [devCode] is only present when the server runs
/// with `expose_otp` (never in production).
class OtpChallenge {
  const OtpChallenge(this.phone, this.expiresAt, this.devCode);
  final String phone;
  final DateTime expiresAt;
  final String? devCode;
}

/// Phone/OTP and guest sign-in over /v1/auth, plus the profile (/v1/me).
/// Failures surface as [ApiException].
class AuthRepository {
  AuthRepository({required this.api, required this.tokens, required this.db});
  final ApiClient api;
  final TokenStore tokens;
  final LocalDb db;

  Future<OtpChallenge> requestOtp(String phone) async {
    final res = await api.post('/v1/auth/otp/request', {'phone': phone});
    return OtpChallenge(phone, DateTime.parse(res['expires_at']! as String), res['dev_code'] as String?);
  }

  Future<Profile> verifyOtp(String phone, String code) =>
      _session(api.post('/v1/auth/otp/verify', {'phone': phone, 'code': code}));

  Future<Profile> continueAsGuest() => _session(api.post('/v1/auth/guest'));

  Future<Profile> _session(Future<Map<String, Object?>> call) async {
    final res = await call;
    final user = Profile.fromJson((res['user']! as Map).cast());
    await tokens.write(Tokens(
        access: res['access_token']! as String,
        refresh: res['refresh_token']! as String,
        role: user.role,
        userId: user.id));
    await db.write('profile', user.toJson());
    return user;
  }

  /// The cached profile, available offline.
  Future<Profile?> cachedProfile() async {
    final raw = await db.read('profile') as Map?;
    return raw == null ? null : Profile.fromJson(raw.cast());
  }

  Future<Profile> updateProfile({String? name, String? district, String? language}) async {
    final res = await api.send('PATCH', '/v1/me', body: {
      'name': ?name,
      'district': ?district,
      'language': ?language,
    });
    final user = Profile.fromJson(res.json);
    await db.write('profile', user.toJson());
    return user;
  }

  Future<bool> get signedIn async => await tokens.read() != null;
}

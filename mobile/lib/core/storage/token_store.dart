import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// An access token plus its rotating refresh token.
class Tokens {
  const Tokens({required this.access, required this.refresh, required this.role, required this.userId});
  final String access;
  final String refresh;
  final String role;
  final String userId;

  bool get isGuest => role == 'guest';

  Map<String, String> toJson() => {'access': access, 'refresh': refresh, 'role': role, 'user_id': userId};

  factory Tokens.fromJson(Map<String, Object?> j) => Tokens(
      access: j['access']! as String,
      refresh: j['refresh']! as String,
      role: j['role']! as String,
      userId: j['user_id']! as String);
}

/// Persists tokens; the production store is the OS keystore.
abstract interface class TokenStore {
  Future<Tokens?> read();
  Future<void> write(Tokens tokens);
  Future<void> clear();
}

class SecureTokenStore implements TokenStore {
  SecureTokenStore([FlutterSecureStorage? storage]) : _s = storage ?? const FlutterSecureStorage();
  final FlutterSecureStorage _s;
  static const _key = 'agrismart.tokens';

  @override
  Future<Tokens?> read() async {
    final raw = await _s.read(key: _key);
    return raw == null ? null : Tokens.fromJson((jsonDecode(raw) as Map).cast());
  }

  @override
  Future<void> write(Tokens tokens) => _s.write(key: _key, value: jsonEncode(tokens.toJson()));

  @override
  Future<void> clear() => _s.delete(key: _key);
}

import 'dart:async';

import 'package:agrismart/core/network/api_client.dart';
import 'package:agrismart/core/storage/token_store.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/fakes.dart';

void main() {
  late FakeServer server;
  late MemoryTokenStore tokens;
  late List<Duration> slept;
  var signedOut = 0;

  ApiClient client({int maxRetries = 2}) => ApiClient(
      baseUrl: 'http://api.test',
      tokens: tokens,
      adapter: server,
      sleep: (d) async => slept.add(d),
      onSignedOut: () => signedOut++,
      maxRetries: maxRetries);

  setUp(() {
    server = FakeServer();
    tokens = MemoryTokenStore();
    slept = [];
    signedOut = 0;
  });

  test('attaches the bearer token and decodes JSON', () async {
    await tokens.write(const Tokens(access: 'A', refresh: 'R', role: 'farmer', userId: 'u'));
    server.json('GET', '/v1/me', {'id': 'u'});
    final res = await client().send('GET', '/v1/me');
    expect(res.status, 200);
    expect(res.json['id'], 'u');
    expect(res.headers['etag'], '"e"');
    expect(server.seen.single.headers['Authorization'], 'Bearer A');
    server.json('POST', '/v1/x', {'ok': true});
    expect(await client().post('/v1/x', {'a': 1}), {'ok': true});
  });

  test('401 refreshes once for concurrent requests and retries them', () async {
    await tokens.write(const Tokens(access: 'old', refresh: 'R1', role: 'farmer', userId: 'u'));
    var refreshes = 0;
    final gate = Completer<void>();
    server.on('POST', '/v1/auth/refresh', (_) async {
      refreshes++;
      await gate.future;
      return const Reply(200, {'access_token': 'new', 'refresh_token': 'R2'});
    });
    server.on('GET', '/v1/me', (r) => r.headers['Authorization'] == 'Bearer new' ? const Reply(200, {'ok': 1}) : const Reply(401));
    final c = client();
    final a = c.get('/v1/me');
    final b = c.get('/v1/me');
    await Future<void>.delayed(Duration.zero);
    gate.complete();
    expect(await a, {'ok': 1});
    expect(await b, {'ok': 1});
    expect(refreshes, 1);
    expect((await tokens.read())!.refresh, 'R2');
    expect((await tokens.read())!.role, 'farmer');
  });

  test('failed refresh signs out and surfaces 401', () async {
    await tokens.write(const Tokens(access: 'old', refresh: 'R1', role: 'farmer', userId: 'u'));
    server.json('POST', '/v1/auth/refresh', {'error': {'code': 'auth.refresh_reused', 'message': 'errors.auth.refresh_reused'}}, 401);
    server.json('GET', '/v1/me', {'error': {'code': 'auth.invalid_token', 'message': 'errors.auth.invalid_token'}}, 401);
    await expectLater(client().get('/v1/me'), throwsA(isA<ApiException>().having((e) => e.code, 'code', 'auth.invalid_token')));
    expect(signedOut, 1);
    expect(await tokens.read(), isNull);
    // without tokens there is nothing to refresh
    await expectLater(client().get('/v1/me'), throwsA(isA<ApiException>()));
    expect(await client().refresh(), isNull);
  });

  test('retried request failing again surfaces the second error', () async {
    await tokens.write(const Tokens(access: 'old', refresh: 'R1', role: 'farmer', userId: 'u'));
    server.json('POST', '/v1/auth/refresh', {'access_token': 'new', 'refresh_token': 'R2'});
    server.json('GET', '/v1/me', {'error': {'code': 'auth.forbidden', 'message': 'errors.auth.forbidden'}}, 401);
    await expectLater(client().get('/v1/me'), throwsA(isA<ApiException>().having((e) => e.status, 'status', 401)));
  });

  test('auth endpoints are never refreshed', () async {
    server.json('POST', '/v1/auth/otp/verify', {'error': {'code': 'auth.otp_invalid', 'message': 'errors.auth.otp_invalid'}}, 401);
    await expectLater(client().post('/v1/auth/otp/verify', {}),
        throwsA(isA<ApiException>().having((e) => e.messageKey, 'key', 'errors.auth.otp_invalid')));
    expect(server.seen.length, 1);
  });

  test('GETs retry transient failures with exponential backoff', () async {
    var n = 0;
    server.on('GET', '/v1/weather', (_) => ++n < 3 ? const Reply(503) : const Reply(200, {'ok': true}));
    expect(await client().get('/v1/weather'), {'ok': true});
    expect(slept, [const Duration(milliseconds: 400), const Duration(milliseconds: 800)]);
    n = -10;
    await expectLater(client(maxRetries: 1).get('/v1/weather'), throwsA(isA<ApiException>().having((e) => e.code, 'code', 'internal')));
  });

  test('writes are not retried; offline maps to a typed error', () async {
    server.json('POST', '/v1/scans', null, 503);
    await expectLater(client().post('/v1/scans'), throwsA(isA<ApiException>()));
    expect(server.seen.length, 1);
    server.offline = true;
    await expectLater(client(maxRetries: 0).get('/v1/x'), throwsA(isA<ApiException>().having((e) => e.isOffline, 'offline', isTrue)));
    expect(ApiException.offline.toString(), contains('network.offline'));
  });

  test('field errors are decoded', () async {
    server.json('POST', '/v1/fields', {'error': {'code': 'request.invalid', 'message': 'errors.request.invalid', 'fields': {'name': 'required'}}}, 400);
    await expectLater(client().post('/v1/fields', {}), throwsA(isA<ApiException>().having((e) => e.fields['name'], 'field', 'required')));
  });
}

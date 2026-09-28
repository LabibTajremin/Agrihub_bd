import 'dart:async';

import 'package:dio/dio.dart';

import '../storage/token_store.dart';

/// A typed API failure. `messageKey` is a dictionary key; status 0 = offline.
class ApiException implements Exception {
  const ApiException(this.status, this.code, this.messageKey, [this.fields = const {}]);
  final int status;
  final String code;
  final String messageKey;
  final Map<String, String> fields;

  bool get isOffline => status == 0;

  static const offline = ApiException(0, 'network.offline', 'home.offline.banner');

  @override
  String toString() => 'ApiException($status, $code)';
}

/// A decoded response.
class ApiResponse {
  const ApiResponse(this.status, this.data, this.headers);
  final int status;
  final Object? data;
  final Map<String, String> headers;

  Map<String, Object?> get json => (data as Map).cast<String, Object?>();
}

typedef Sleeper = Future<void> Function(Duration);

/// Dio client with the auth pipeline: bearer header, refresh-on-401 with a
/// single in-flight refresh, and bounded retry with backoff for idempotent reads.
class ApiClient {
  ApiClient({
    required String baseUrl,
    required this.tokens,
    HttpClientAdapter? adapter,
    Sleeper? sleep,
    this.onSignedOut,
    this.maxRetries = 2,
    this.retryBase = const Duration(milliseconds: 400),
  })  : _sleep = sleep ?? Future<void>.delayed,
        dio = Dio(BaseOptions(baseUrl: baseUrl, connectTimeout: const Duration(seconds: 10), receiveTimeout: const Duration(seconds: 20))),
        _bare = Dio(BaseOptions(baseUrl: baseUrl)) {
    if (adapter != null) {
      dio.httpClientAdapter = adapter;
      _bare.httpClientAdapter = adapter;
    }
    dio.interceptors.add(InterceptorsWrapper(onRequest: _attach, onError: _onError));
  }

  final Dio dio;
  final Dio _bare;
  final TokenStore tokens;
  final Sleeper _sleep;
  final void Function()? onSignedOut;
  final int maxRetries;
  final Duration retryBase;
  Future<Tokens?>? _inflight;

  Future<void> _attach(RequestOptions o, RequestInterceptorHandler h) async {
    final t = await tokens.read();
    if (t != null && !o.headers.containsKey('Authorization')) {
      o.headers['Authorization'] = 'Bearer ${t.access}';
    }
    h.next(o);
  }

  Future<void> _onError(DioException e, ErrorInterceptorHandler h) async {
    final o = e.requestOptions;
    final status = e.response?.statusCode;
    if (status == 401 && !o.path.startsWith('/v1/auth/') && o.extra['refreshed'] != true) {
      final fresh = await refresh();
      if (fresh == null) {
        return h.next(e);
      }
      o.headers['Authorization'] = 'Bearer ${fresh.access}';
      o.extra['refreshed'] = true;
      return _resend(o, e, h);
    }
    final attempt = (o.extra['attempt'] as int?) ?? 0;
    final transient = status == null || status == 502 || status == 503 || status == 504;
    if (o.method == 'GET' && transient && e.type != DioExceptionType.cancel && attempt < maxRetries) {
      await _sleep(retryBase * (1 << attempt));
      o.extra['attempt'] = attempt + 1;
      return _resend(o, e, h);
    }
    h.next(e);
  }

  Future<void> _resend(RequestOptions o, DioException original, ErrorInterceptorHandler h) async {
    try {
      h.resolve(await dio.fetch<Object?>(o));
    } on DioException catch (err) {
      h.next(err);
    }
  }

  /// Rotates the refresh token once for all concurrent 401s. On failure the
  /// session is cleared and [onSignedOut] fires.
  Future<Tokens?> refresh() => _inflight ??= _doRefresh().whenComplete(() => _inflight = null);

  Future<Tokens?> _doRefresh() async {
    final t = await tokens.read();
    if (t == null) {
      return null;
    }
    try {
      final res = await _bare.post<Map<String, Object?>>('/v1/auth/refresh', data: {'refresh_token': t.refresh});
      final fresh = Tokens(
          access: res.data!['access_token']! as String,
          refresh: res.data!['refresh_token']! as String,
          role: t.role,
          userId: t.userId);
      await tokens.write(fresh);
      return fresh;
    } on DioException {
      await tokens.clear();
      onSignedOut?.call();
      return null;
    }
  }

  /// Sends a request and maps failures to [ApiException].
  Future<ApiResponse> send(String method, String path,
      {Object? body, Map<String, Object?>? query, Map<String, String>? headers}) async {
    try {
      final res = await dio.request<Object?>(path,
          data: body,
          queryParameters: query,
          options: Options(method: method, headers: headers, validateStatus: (s) => s != null && s < 400));
      return ApiResponse(res.statusCode!, res.data, {for (final e in res.headers.map.entries) e.key: e.value.join(',')});
    } on DioException catch (e) {
      throw _map(e);
    }
  }

  Future<Map<String, Object?>> get(String path, {Map<String, Object?>? query}) async =>
      (await send('GET', path, query: query)).json;

  Future<Map<String, Object?>> post(String path, [Object? body]) async => (await send('POST', path, body: body)).json;

  /// Sends raw bytes to a signed upload URL. No bearer token: the URL is the
  /// credential (and object stores reject foreign auth headers).
  Future<void> upload(String method, String url, List<int> bytes, Map<String, String> headers) async {
    try {
      await _bare.request<Object?>(url, data: Stream.value(bytes), options: Options(method: method, headers: {
        ...headers,
        Headers.contentLengthHeader: bytes.length,
      }));
    } on DioException catch (e) {
      throw _map(e);
    }
  }

  static ApiException _map(DioException e) {
    final r = e.response;
    if (r == null) {
      return ApiException.offline;
    }
    final data = r.data;
    final err = data is Map ? data['error'] : null;
    if (err is Map) {
      return ApiException(r.statusCode!, '${err['code']}', '${err['message']}',
          ((err['fields'] as Map?) ?? const {}).map((k, v) => MapEntry('$k', '$v')));
    }
    return ApiException(r.statusCode!, 'internal', 'errors.internal');
  }
}

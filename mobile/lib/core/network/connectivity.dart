import 'dart:async';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter/foundation.dart';

/// Online/offline state for the offline-first UI. It follows the platform
/// connectivity signal and is also told about failed requests.
class ConnectivityService extends ChangeNotifier {
  ConnectivityService([Connectivity? connectivity]) : _c = connectivity ?? Connectivity();
  final Connectivity _c;
  StreamSubscription<List<ConnectivityResult>>? _sub;
  bool _online = true;

  bool get online => _online;

  static bool _hasNetwork(List<ConnectivityResult> r) => r.any((x) => x != ConnectivityResult.none);

  Future<void> start() async {
    report(_hasNetwork(await _c.checkConnectivity()));
    _sub = _c.onConnectivityChanged.listen((r) => report(_hasNetwork(r)));
  }

  /// Records the observed state (e.g. a request failed with no network).
  void report(bool online) {
    if (online != _online) {
      _online = online;
      notifyListeners();
    }
  }

  @override
  void dispose() {
    _sub?.cancel();
    super.dispose();
  }
}

import '../network/connectivity.dart';

/// Runs [run] whenever connectivity comes back, so work queued offline
/// (scans, annotations) reaches the server without a manual "sync now".
class AutoSync {
  AutoSync({required this.connectivity, required this.run});
  final ConnectivityService connectivity;
  final Future<void> Function() run;
  bool _online = true;

  void start() {
    _online = connectivity.online;
    connectivity.addListener(_changed);
  }

  void stop() => connectivity.removeListener(_changed);

  void _changed() {
    final back = connectivity.online && !_online;
    _online = connectivity.online;
    if (back) {
      run();
    }
  }
}

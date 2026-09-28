import 'package:geolocator/geolocator.dart';

import '../home/home_repository.dart';

/// The phone's current position, asking for permission when needed.
abstract interface class LocationGateway {
  /// Null when location is off or permission is refused.
  Future<LatLng?> current();
}

/// [LocationGateway] over the geolocator plugin.
class PluginLocation implements LocationGateway {
  const PluginLocation();

  @override
  Future<LatLng?> current() async {
    final g = GeolocatorPlatform.instance;
    if (!await g.isLocationServiceEnabled()) {
      return null;
    }
    var p = await g.checkPermission();
    if (p == LocationPermission.denied) {
      p = await g.requestPermission();
    }
    if (p == LocationPermission.denied || p == LocationPermission.deniedForever) {
      return null;
    }
    final pos = await g.getCurrentPosition(locationSettings: const LocationSettings(accuracy: LocationAccuracy.high));
    return (lat: pos.latitude, lng: pos.longitude);
  }
}

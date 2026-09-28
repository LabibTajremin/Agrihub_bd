import 'dart:typed_data';

import 'package:camera/camera.dart';
import 'package:flutter/widgets.dart';

/// The viewfinder's view of a camera: start, preview, capture, stop.
abstract interface class CameraGateway {
  /// Opens the back camera; false when there is none or it cannot start.
  Future<bool> start();
  Widget preview();
  Future<Uint8List> capture();
  Future<void> stop();
}

/// [CameraGateway] over the camera plugin.
class PluginCamera implements CameraGateway {
  CameraController? _controller;

  @override
  Future<bool> start() async {
    try {
      final cameras = await availableCameras();
      if (cameras.isEmpty) {
        return false;
      }
      final back = cameras.firstWhere((c) => c.lensDirection == CameraLensDirection.back, orElse: () => cameras.first);
      final c = CameraController(back, ResolutionPreset.medium, enableAudio: false);
      _controller = c;
      await c.initialize();
      return true;
    } on CameraException {
      return false;
    }
  }

  @override
  Widget preview() => CameraPreview(_controller!);

  @override
  Future<Uint8List> capture() async => (await _controller!.takePicture()).readAsBytes();

  @override
  Future<void> stop() async => _controller?.dispose();
}

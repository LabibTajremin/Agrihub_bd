import 'dart:typed_data';

import 'package:image/image.dart' as img;

/// Perceptual difference hash (64-bit, hex): the image is reduced to 9×8
/// grey pixels and each bit records whether a pixel is brighter than its right
/// neighbour. Near-identical photos have hashes a few bits apart. Returns null
/// for bytes that are not a JPEG (the camera's format).
String? dHash(Uint8List bytes) {
  final decoded = img.JpegDecoder().isValidFile(bytes) ? img.decodeJpg(bytes) : null;
  if (decoded == null) {
    return null;
  }
  final small = img.copyResize(img.grayscale(decoded), width: 9, height: 8, interpolation: img.Interpolation.average);
  var hash = BigInt.zero;
  for (var y = 0; y < 8; y++) {
    for (var x = 0; x < 8; x++) {
      final brighter = small.getPixel(x, y).r > small.getPixel(x + 1, y).r;
      hash = (hash << 1) | (brighter ? BigInt.one : BigInt.zero);
    }
  }
  return hash.toRadixString(16).padLeft(16, '0');
}

/// Number of differing bits between two dHashes.
int hamming(String a, String b) {
  var x = BigInt.parse(a, radix: 16) ^ BigInt.parse(b, radix: 16);
  var n = 0;
  while (x > BigInt.zero) {
    n += (x & BigInt.one).toInt();
    x >>= 1;
  }
  return n;
}

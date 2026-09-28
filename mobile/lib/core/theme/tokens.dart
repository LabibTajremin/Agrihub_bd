import 'package:flutter/material.dart';

/// The 37 design tokens of the AgriSmart UI kit — the single source of truth
/// for colour, spacing, radius, elevation and type. Widgets never hardcode
/// these values; they read them from here (via [AppTheme]).
abstract final class Tokens {
  // Colour (18)
  static const primary = Color(0xFF1B7F3B);
  static const primaryDark = Color(0xFF0F5A27);
  static const primaryLight = Color(0xFFD7F0DE);
  static const secondary = Color(0xFF8A5A00);
  static const accent = Color(0xFFF2B705);
  static const background = Color(0xFFF7F8F3);
  static const surface = Color(0xFFFFFFFF);
  static const surfaceVariant = Color(0xFFEEF1E8);
  static const error = Color(0xFFC62828);
  static const warning = Color(0xFFE67E00);
  static const success = Color(0xFF2E7D32);
  static const info = Color(0xFF1565C0);
  static const textPrimary = Color(0xFF1C2418);
  static const textSecondary = Color(0xFF5B6655);
  static const textOnPrimary = Color(0xFFFFFFFF);
  static const border = Color(0xFFD5DACD);
  static const disabled = Color(0xFFB5BCAD);
  static const overlay = Color(0x99000000);

  // Spacing (8)
  static const spaceXxs = 2.0;
  static const spaceXs = 4.0;
  static const spaceSm = 8.0;
  static const spaceMd = 12.0;
  static const spaceLg = 16.0;
  static const spaceXl = 24.0;
  static const spaceXxl = 32.0;
  static const spaceXxxl = 48.0;

  // Radius (4)
  static const radiusSm = 4.0;
  static const radiusMd = 8.0;
  static const radiusLg = 16.0;
  static const radiusPill = 999.0;

  // Elevation (1)
  static const elevationCard = 1.0;

  // Typography (6): size / weight / height
  static const display = TextStyle(fontSize: 32, fontWeight: FontWeight.w700, height: 1.2);
  static const headline = TextStyle(fontSize: 24, fontWeight: FontWeight.w700, height: 1.25);
  static const title = TextStyle(fontSize: 18, fontWeight: FontWeight.w600, height: 1.3);
  static const body = TextStyle(fontSize: 16, fontWeight: FontWeight.w400, height: 1.45);
  static const label = TextStyle(fontSize: 14, fontWeight: FontWeight.w600, height: 1.3);
  static const caption = TextStyle(fontSize: 12, fontWeight: FontWeight.w400, height: 1.35);

  /// Count of tokens, asserted by tests to keep this file aligned with the kit.
  static const count = 37;
}

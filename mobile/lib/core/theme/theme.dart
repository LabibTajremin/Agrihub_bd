import 'package:flutter/material.dart';

import 'tokens.dart';

/// Builds the Material theme exclusively from [Tokens].
abstract final class AppTheme {
  static ThemeData light() {
    final scheme = ColorScheme.fromSeed(
      seedColor: Tokens.primary,
      primary: Tokens.primary,
      onPrimary: Tokens.textOnPrimary,
      secondary: Tokens.secondary,
      tertiary: Tokens.accent,
      error: Tokens.error,
      surface: Tokens.surface,
      onSurface: Tokens.textPrimary,
    );
    const text = TextTheme(
      displaySmall: Tokens.display,
      headlineSmall: Tokens.headline,
      titleMedium: Tokens.title,
      bodyLarge: Tokens.body,
      bodyMedium: Tokens.body,
      labelLarge: Tokens.label,
      bodySmall: Tokens.caption,
    );
    final rounded = RoundedRectangleBorder(borderRadius: BorderRadius.circular(Tokens.radiusMd));
    return ThemeData(
      useMaterial3: true,
      colorScheme: scheme,
      scaffoldBackgroundColor: Tokens.background,
      textTheme: text.apply(bodyColor: Tokens.textPrimary, displayColor: Tokens.textPrimary),
      cardTheme: CardThemeData(
        color: Tokens.surface,
        elevation: Tokens.elevationCard,
        margin: EdgeInsets.zero,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(Tokens.radiusLg)),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          minimumSize: const Size.fromHeight(52),
          shape: rounded,
          textStyle: Tokens.label,
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          minimumSize: const Size.fromHeight(52),
          shape: rounded,
          side: const BorderSide(color: Tokens.border),
          textStyle: Tokens.label,
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: Tokens.surface,
        border: OutlineInputBorder(borderRadius: BorderRadius.circular(Tokens.radiusMd)),
        contentPadding: const EdgeInsets.all(Tokens.spaceLg),
      ),
      dividerColor: Tokens.border,
      disabledColor: Tokens.disabled,
    );
  }
}

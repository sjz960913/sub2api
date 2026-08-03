import 'package:flutter/material.dart';

abstract final class AppColors {
  static const background = Color(0xFFF8F7F3);
  static const surface = Color(0xFFFFFEFB);
  static const ink = Color(0xFF101323);
  static const muted = Color(0xFF73747C);
  static const primary = Color(0xFF304FFE);
  static const iconTile = Color(0xFFF0F1FA);
  static const border = Color(0xFFD7D6D1);
  static const success = Color(0xFF22B573);
}

ThemeData buildSub2ApiTheme() {
  final scheme = ColorScheme.fromSeed(
    seedColor: AppColors.primary,
    brightness: Brightness.light,
    surface: AppColors.surface,
  );

  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: AppColors.background,
    fontFamilyFallback: const ['Noto Sans SC', 'Roboto'],
    textTheme: const TextTheme(
      headlineMedium: TextStyle(
        color: AppColors.ink,
        fontSize: 30,
        fontWeight: FontWeight.w800,
        letterSpacing: -0.6,
      ),
      titleLarge: TextStyle(
        color: AppColors.ink,
        fontSize: 20,
        fontWeight: FontWeight.w700,
      ),
      bodyLarge: TextStyle(color: AppColors.ink, fontSize: 16),
      bodyMedium: TextStyle(color: AppColors.muted, fontSize: 14),
    ),
    cardTheme: const CardThemeData(
      color: AppColors.surface,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(16)),
        side: BorderSide(color: AppColors.border),
      ),
    ),
    inputDecorationTheme: const InputDecorationTheme(
      filled: true,
      fillColor: AppColors.surface,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.all(Radius.circular(14)),
        borderSide: BorderSide(color: AppColors.border),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.all(Radius.circular(14)),
        borderSide: BorderSide(color: AppColors.border),
      ),
      contentPadding: EdgeInsets.symmetric(horizontal: 16, vertical: 15),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size(48, 48),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      ),
    ),
    navigationBarTheme: NavigationBarThemeData(
      height: 72,
      backgroundColor: Colors.transparent,
      indicatorColor: Colors.transparent,
      elevation: 0,
      labelTextStyle: WidgetStateProperty.resolveWith((states) {
        final selected = states.contains(WidgetState.selected);
        return TextStyle(
          color: selected ? AppColors.primary : AppColors.muted,
          fontSize: 12,
          fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
        );
      }),
      iconTheme: WidgetStateProperty.resolveWith((states) {
        return IconThemeData(
          color: states.contains(WidgetState.selected) ? AppColors.primary : AppColors.muted,
          size: 25,
        );
      }),
    ),
  );
}

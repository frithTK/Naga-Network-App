import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:qr_flutter/qr_flutter.dart';

import 'android_vpn.dart';
import 'app_update.dart';
import 'control_plane_client.dart';
import 'config_file_picker.dart';
import 'subscription_fetch.dart';
import 'subscription_link.dart';

part 'ui/home.dart';
part 'ui/components.dart';

bool runtimeBlocksProfileDelete(String? status) =>
    status == 'starting' || status == 'connected' || status == 'stopping';

void main(List<String> args) {
  LicenseRegistry.addLicense(() async* {
    yield LicenseEntryWithLineBreaks([
      'Inter',
    ], await rootBundle.loadString('assets/fonts/OFL.txt'));
  });
  runApp(NagaNetworkApp(initialImportURL: launchSubscriptionURL(args)));
}

const _trayChannel = MethodChannel('eu.nagavpn.naga_network/tray');

// Naga brand tokens from Naga_Network_UI_UX_Spec.md.
const _background = Color(0xFF080808);
const _surfaceLow = Color(0xFF111215);
const _surface = Color(0xFF1A1C20);
const _surfaceRaised = Color(0xFF202227);
const _surfaceElevated = Color(0xFF202227);
const _line = Color(0xFF282E36);
const _lineStrong = Color(0xFF34363E);
const _text = Color(0xFFFFFFFF);
const _muted = Color(0xFFABB0B9);
const _tertiary = Color(0xFF9299A5);
const _accent = Color(0xFFF72F38);
const _success = Color(0xFF27C46A);
const _warning = Color(0xFFFFB020);
const _error = Color(0xFFFF454D);
const _accentInk = Color(0xFF080808);
const _brandMarkAsset = 'assets/brand/naga-mark-red.png';
const _brandMascotCutoutAsset = 'assets/brand/mascot-cutout.png';

/// Позы маскота из брендпака, закреплённые за состояниями подключения.
enum NagaMascotMood { welcome, idle, connecting, connected, error, offline }

String _brandMascotStateAsset(NagaMascotMood mood) =>
    'assets/brand/states/${mood.name}.png';

class NagaBreakpoints {
  static const compact = 600.0;
  static const medium = 1024.0;
  static const expanded = 1280.0;

  /// Native Windows min client size. Width still reaches the phone layout.
  static const minWindowWidth = 390.0;
  static const minWindowHeight = 560.0;
}

class NagaSpacing {
  static const xxs = 4.0;
  static const xs = 8.0;
  static const sm = 12.0;
  static const md = 16.0;
  static const lg = 24.0;
  static const xl = 32.0;
  static const xxl = 48.0;
}

class NagaRadius {
  static const small = 10.0;
  static const control = 12.0;
  static const card = 16.0;
  static const large = 20.0;
  static const modal = 20.0;
}

class NagaMotion {
  static const fast = Duration(milliseconds: 120);
  static const normal = Duration(milliseconds: 200);
  static const slow = Duration(milliseconds: 320);
  static const connectPulse = Duration(milliseconds: 1400);
  static const standard = Curves.easeOutCubic;
  static const pulse = Curves.easeInOutCubic;
}

/// Logical pixels; must match C++ kNagaTitleBarHeight / kNagaCaptionButtonSpan.
const nagaTitleBarHeight = 40.0;
const nagaCaptionButtonSpan = 132.0;

bool get nagaUsesWindowsChrome =>
    !kIsWeb && defaultTargetPlatform == TargetPlatform.windows;

String get nagaAppPlatform {
  if (nagaUsesWindowsChrome) return 'windows';
  if (nagaRunsOnAndroid) return 'android';
  return 'linux';
}

class NagaTitleBar extends StatelessWidget {
  const NagaTitleBar({
    super.key = const Key('naga-window-titlebar'),
    this.onMinimize,
    this.onMaximize,
    this.onClose,
    this.onDrag,
  });

  final VoidCallback? onMinimize;
  final VoidCallback? onMaximize;
  final VoidCallback? onClose;
  final VoidCallback? onDrag;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: _background,
      child: SizedBox(
        height: nagaTitleBarHeight,
        child: DecoratedBox(
          decoration: const BoxDecoration(
            border: Border(bottom: BorderSide(color: _line)),
          ),
          child: Row(
            children: [
              Expanded(
                child: GestureDetector(
                  behavior: HitTestBehavior.translucent,
                  onPanStart: onDrag == null ? null : (_) => onDrag!(),
                  onDoubleTap: onMaximize,
                  child: const Row(
                    children: [
                      SizedBox(width: 12),
                      Image(
                        image: AssetImage(_brandMarkAsset),
                        width: 18,
                        height: 18,
                        filterQuality: FilterQuality.medium,
                      ),
                      SizedBox(width: 8),
                      Text(
                        'Naga Network',
                        style: TextStyle(
                          color: _text,
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      Expanded(child: SizedBox.expand()),
                    ],
                  ),
                ),
              ),
              SizedBox(
                width: nagaCaptionButtonSpan,
                height: nagaTitleBarHeight,
                child: Row(
                  children: [
                    _CaptionButton(
                      key: const Key('naga-window-minimize'),
                      kind: _CaptionKind.minimize,
                      tooltip: 'Свернуть',
                      onPressed: onMinimize,
                    ),
                    _CaptionButton(
                      key: const Key('naga-window-maximize'),
                      kind: _CaptionKind.maximize,
                      tooltip: 'Развернуть',
                      onPressed: onMaximize,
                    ),
                    _CaptionButton(
                      key: const Key('naga-window-close'),
                      kind: _CaptionKind.close,
                      tooltip: 'Закрыть',
                      hoverColor: _accent,
                      onPressed: onClose,
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

enum _CaptionKind { minimize, maximize, close }

class _CaptionButton extends StatelessWidget {
  const _CaptionButton({
    super.key,
    required this.kind,
    required this.tooltip,
    this.onPressed,
    this.hoverColor,
  });

  final _CaptionKind kind;
  final String tooltip;
  final VoidCallback? onPressed;
  final Color? hoverColor;

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: tooltip,
      waitDuration: const Duration(milliseconds: 400),
      child: SizedBox(
        width: 44,
        height: nagaTitleBarHeight,
        child: Material(
          color: Colors.transparent,
          child: InkWell(
            onTap: onPressed,
            hoverColor: hoverColor ?? const Color(0x33FFFFFF),
            child: Center(
              child: CustomPaint(
                size: const Size(14, 14),
                painter: _CaptionGlyphPainter(kind: kind, color: _text),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _CaptionGlyphPainter extends CustomPainter {
  const _CaptionGlyphPainter({required this.kind, required this.color});

  final _CaptionKind kind;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 2.2
      ..style = PaintingStyle.stroke
      ..strokeCap = StrokeCap.square;
    switch (kind) {
      case _CaptionKind.minimize:
        final y = size.height - 1;
        canvas.drawLine(Offset(0, y), Offset(size.width, y), paint);
      case _CaptionKind.maximize:
        canvas.drawRect(
          Rect.fromLTWH(0.8, 0.8, size.width - 1.6, size.height - 1.6),
          paint,
        );
      case _CaptionKind.close:
        canvas.drawLine(Offset.zero, Offset(size.width, size.height), paint);
        canvas.drawLine(Offset(size.width, 0), Offset(0, size.height), paint);
    }
  }

  @override
  bool shouldRepaint(covariant _CaptionGlyphPainter oldDelegate) {
    return oldDelegate.kind != kind || oldDelegate.color != color;
  }
}

ThemeData nagaTheme() {
  final controlShape = RoundedRectangleBorder(
    borderRadius: BorderRadius.circular(NagaRadius.control),
  );
  return ThemeData(
    brightness: Brightness.dark,
    useMaterial3: true,
    fontFamily: 'Inter',
    scaffoldBackgroundColor: _background,
    canvasColor: _background,
    dividerColor: _line,
    dividerTheme: const DividerThemeData(color: _line, thickness: 1),
    hoverColor: _surfaceRaised,
    splashFactory: InkRipple.splashFactory,
    visualDensity: VisualDensity.standard,
    colorScheme: const ColorScheme.dark(
      surface: _surface,
      primary: _accent,
      onPrimary: _accentInk,
      secondary: _accent,
      onSurface: _text,
      error: _error,
      onError: _accentInk,
      outline: _line,
    ),
    textTheme: const TextTheme(
      displaySmall: TextStyle(
        fontSize: 32,
        height: 38 / 32,
        fontWeight: FontWeight.w500,
        color: _text,
        letterSpacing: -0.4,
      ),
      headlineMedium: TextStyle(
        fontSize: 26,
        height: 32 / 26,
        fontWeight: FontWeight.w600,
        color: _text,
        letterSpacing: -0.2,
      ),
      headlineSmall: TextStyle(
        fontSize: 22,
        height: 28 / 22,
        fontWeight: FontWeight.w600,
        color: _text,
      ),
      titleLarge: TextStyle(
        fontSize: 18,
        height: 24 / 18,
        fontWeight: FontWeight.w600,
        color: _text,
      ),
      titleMedium: TextStyle(
        fontSize: 15,
        height: 20 / 15,
        fontWeight: FontWeight.w600,
        color: _text,
      ),
      bodyLarge: TextStyle(
        fontSize: 14,
        height: 20 / 14,
        fontWeight: FontWeight.w400,
        color: _text,
      ),
      bodyMedium: TextStyle(
        fontSize: 13,
        height: 18 / 13,
        fontWeight: FontWeight.w400,
        color: _muted,
      ),
      bodySmall: TextStyle(
        fontSize: 12,
        height: 16 / 12,
        fontWeight: FontWeight.w400,
        color: _muted,
      ),
      labelLarge: TextStyle(
        fontSize: 14,
        height: 20 / 14,
        fontWeight: FontWeight.w600,
        color: _text,
        letterSpacing: 0.1,
      ),
    ),
    cardTheme: CardThemeData(
      color: _surfaceLow,
      clipBehavior: Clip.antiAlias,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(NagaRadius.card),
        side: BorderSide(color: _line.withValues(alpha: 0.65)),
      ),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: _accent,
        foregroundColor: _accentInk,
        disabledBackgroundColor: _surfaceRaised,
        disabledForegroundColor: _tertiary,
        minimumSize: const Size(48, 48),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        shape: controlShape,
        textStyle: const TextStyle(
          fontFamily: 'Inter',
          fontWeight: FontWeight.w600,
          fontSize: 14,
        ),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: _text,
        disabledForegroundColor: _tertiary,
        minimumSize: const Size(48, 48),
        shape: controlShape,
      ),
    ),
    iconButtonTheme: IconButtonThemeData(
      style: IconButton.styleFrom(
        minimumSize: const Size(48, 48),
        foregroundColor: _muted,
        hoverColor: _surfaceRaised,
        highlightColor: _accent.withValues(alpha: 0.12),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(NagaRadius.small),
        ),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: _text,
        minimumSize: const Size(48, 48),
        side: const BorderSide(color: _lineStrong),
        shape: controlShape,
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: _surfaceLow,
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
      hintStyle: const TextStyle(color: _tertiary),
      labelStyle: const TextStyle(color: _muted),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(NagaRadius.control),
        borderSide: const BorderSide(color: _lineStrong),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(NagaRadius.control),
        borderSide: const BorderSide(color: _lineStrong),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(NagaRadius.control),
        borderSide: const BorderSide(color: _accent, width: 2),
      ),
    ),
    segmentedButtonTheme: SegmentedButtonThemeData(
      style: ButtonStyle(
        backgroundColor: WidgetStateProperty.resolveWith((states) {
          if (states.contains(WidgetState.selected)) {
            return _accent.withValues(alpha: 0.16);
          }
          return _surfaceLow;
        }),
        foregroundColor: WidgetStateProperty.resolveWith((states) {
          if (states.contains(WidgetState.selected)) {
            return _text;
          }
          return _muted;
        }),
        side: WidgetStateProperty.all(const BorderSide(color: _line)),
        visualDensity: VisualDensity.standard,
        shape: WidgetStateProperty.all(controlShape),
      ),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: _surfaceLow,
      indicatorColor: _accent.withValues(alpha: 0.10),
      surfaceTintColor: Colors.transparent,
      iconTheme: WidgetStateProperty.resolveWith((states) {
        final selected = states.contains(WidgetState.selected);
        return IconThemeData(color: selected ? _accent : _muted, size: 22);
      }),
      labelTextStyle: WidgetStateProperty.resolveWith((states) {
        final selected = states.contains(WidgetState.selected);
        return TextStyle(
          fontSize: 12,
          fontWeight: selected ? FontWeight.w600 : FontWeight.w500,
          color: selected ? _text : _muted,
        );
      }),
    ),
    snackBarTheme: SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      backgroundColor: _surfaceElevated,
      contentTextStyle: const TextStyle(color: _text, fontSize: 13),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(NagaRadius.control),
        side: const BorderSide(color: _line),
      ),
    ),
    tooltipTheme: TooltipThemeData(
      waitDuration: const Duration(milliseconds: 400),
      decoration: BoxDecoration(
        color: _surfaceElevated,
        borderRadius: BorderRadius.circular(NagaRadius.small),
        border: Border.all(color: _line),
      ),
      textStyle: const TextStyle(color: _text, fontSize: 12),
    ),
    switchTheme: SwitchThemeData(
      thumbColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return _text;
        }
        return _muted;
      }),
      trackColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return _accent;
        }
        return _lineStrong;
      }),
    ),
    scrollbarTheme: ScrollbarThemeData(
      thumbColor: WidgetStateProperty.all(_lineStrong),
      radius: const Radius.circular(8),
      thickness: WidgetStateProperty.all(6),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: _surface,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(NagaRadius.modal),
        side: const BorderSide(color: _line),
      ),
    ),
    pageTransitionsTheme: const PageTransitionsTheme(
      builders: {
        TargetPlatform.linux: FadeUpwardsPageTransitionsBuilder(),
        TargetPlatform.windows: FadeUpwardsPageTransitionsBuilder(),
        TargetPlatform.macOS: FadeUpwardsPageTransitionsBuilder(),
        TargetPlatform.android: FadeUpwardsPageTransitionsBuilder(),
        TargetPlatform.iOS: FadeUpwardsPageTransitionsBuilder(),
      },
    ),
  );
}

class NagaNetworkApp extends StatelessWidget {
  const NagaNetworkApp({super.key, this.initialImportURL});

  final String? initialImportURL;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Naga Network',
      debugShowCheckedModeBanner: false,
      theme: nagaTheme(),
      home: Uri.base.queryParameters['design'] == 'lab'
          ? const DesignGalleryPage()
          : DashboardPage(initialImportURL: initialImportURL),
    );
  }
}

enum _DesignVariant { orbit, signal, monolith, route, pulse }

class _DesignVariantSpec {
  const _DesignVariantSpec({
    required this.variant,
    required this.number,
    required this.name,
    required this.subtitle,
    required this.accent,
    required this.tagline,
  });

  final _DesignVariant variant;
  final String number;
  final String name;
  final String subtitle;
  final Color accent;
  final String tagline;
}

const _designVariants = [
  _DesignVariantSpec(
    variant: _DesignVariant.orbit,
    number: '01',
    name: 'Orbit',
    subtitle: 'Маскот как живой центр подключения',
    accent: Color(0xFFF72F38),
    tagline: 'Твоя сеть под защитой',
  ),
  _DesignVariantSpec(
    variant: _DesignVariant.signal,
    number: '02',
    name: 'Signal',
    subtitle: 'Максимум ясности: статус, узел и трафик',
    accent: Color(0xFFB66CFF),
    tagline: 'Сигнал стабилен',
  ),
  _DesignVariantSpec(
    variant: _DesignVariant.monolith,
    number: '03',
    name: 'Monolith',
    subtitle: 'Большой спокойный power-screen для desktop',
    accent: Color(0xFFFF6B72),
    tagline: 'Один клик до сети',
  ),
  _DesignVariantSpec(
    variant: _DesignVariant.route,
    number: '04',
    name: 'Route Map',
    subtitle: 'Выбор узла сразу на главном экране',
    accent: Color(0xFF39D98A),
    tagline: 'Маршрут выбран',
  ),
  _DesignVariantSpec(
    variant: _DesignVariant.pulse,
    number: '05',
    name: 'Pulse',
    subtitle: 'Компактный command center для маленьких экранов',
    accent: Color(0xFFF4C95D),
    tagline: 'Всё работает тихо',
  ),
];

class DesignGalleryPage extends StatelessWidget {
  const DesignGalleryPage({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: _background,
      body: SafeArea(
        child: LayoutBuilder(
          builder: (context, constraints) {
            final compact = constraints.maxWidth < 760;
            return SingleChildScrollView(
              padding: EdgeInsets.fromLTRB(
                compact ? 18 : 42,
                compact ? 18 : 30,
                compact ? 18 : 42,
                42,
              ),
              child: Center(
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 1280),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      _GalleryHeader(compact: compact),
                      const SizedBox(height: 28),
                      Text(
                        'Пять направлений для Naga Network',
                        style: Theme.of(context).textTheme.headlineMedium
                            ?.copyWith(fontWeight: FontWeight.w800),
                      ),
                      const SizedBox(height: 8),
                      const SizedBox(
                        width: 650,
                        child: Text(
                          'Маскот становится частью сценария: встречает нового пользователя, показывает состояние сети и помогает выбрать маршрут. На каждой карточке — desktop и mobile версия одной идеи.',
                          style: TextStyle(color: _muted, height: 1.5),
                        ),
                      ),
                      const SizedBox(height: 28),
                      for (final spec in _designVariants) ...[
                        _DesignVariantCard(spec: spec),
                        const SizedBox(height: 18),
                      ],
                      const SizedBox(height: 8),
                      Card(
                        color: _surfaceRaised,
                        child: Padding(
                          padding: const EdgeInsets.all(18),
                          child: Row(
                            children: [
                              const Icon(
                                Icons.touch_app_rounded,
                                color: _accent,
                              ),
                              const SizedBox(width: 12),
                              const Expanded(
                                child: Text(
                                  'Выбери номер концепции — после этого перенесу выбранную композицию в рабочий dashboard и доведу состояния подключения.',
                                  style: TextStyle(color: _muted, height: 1.4),
                                ),
                              ),
                              if (!compact)
                                const Text(
                                  'NAGA UI LAB',
                                  style: TextStyle(
                                    color: _tertiary,
                                    fontSize: 11,
                                    letterSpacing: 1.4,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                            ],
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            );
          },
        ),
      ),
    );
  }
}

class _GalleryHeader extends StatelessWidget {
  const _GalleryHeader({required this.compact});

  final bool compact;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        IconButton(
          tooltip: 'Назад',
          onPressed: () {
            if (Navigator.of(context).canPop()) Navigator.of(context).pop();
          },
          icon: const Icon(Icons.arrow_back_rounded),
        ),
        const SizedBox(width: 4),
        SizedBox(width: 30, height: 30, child: Image.asset(_brandMarkAsset)),
        const SizedBox(width: 10),
        const Text(
          'NAGA UI LAB',
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w800,
            letterSpacing: 1.5,
          ),
        ),
        const Spacer(),
        if (!compact)
          const Text(
            'DESKTOP  /  MOBILE  /  2026',
            style: TextStyle(
              color: _tertiary,
              fontSize: 11,
              letterSpacing: 1.1,
            ),
          ),
      ],
    );
  }
}

class _DesignVariantCard extends StatelessWidget {
  const _DesignVariantCard({required this.spec});

  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) {
    return Card(
      color: _surface,
      clipBehavior: Clip.hardEdge,
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: LayoutBuilder(
          builder: (context, constraints) {
            final compact = constraints.maxWidth < 900;
            final desktop = _DesktopVariantPreview(spec: spec);
            final phone = _PhoneVariantPreview(spec: spec);
            return Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 8,
                        vertical: 5,
                      ),
                      decoration: BoxDecoration(
                        color: spec.accent.withValues(alpha: 0.14),
                        borderRadius: BorderRadius.circular(7),
                      ),
                      child: Text(
                        spec.number,
                        style: TextStyle(
                          color: spec.accent,
                          fontSize: 11,
                          fontWeight: FontWeight.w800,
                        ),
                      ),
                    ),
                    const SizedBox(width: 10),
                    Text(
                      spec.name,
                      style: const TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Text(
                        spec.subtitle,
                        style: const TextStyle(color: _muted),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    if (!compact)
                      Text(
                        spec.tagline.toUpperCase(),
                        style: TextStyle(
                          color: spec.accent,
                          fontSize: 10,
                          letterSpacing: 1,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                  ],
                ),
                const SizedBox(height: 16),
                if (compact) ...[
                  desktop,
                  const SizedBox(height: 14),
                  Center(child: phone),
                ] else
                  Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Expanded(child: desktop),
                      const SizedBox(width: 16),
                      phone,
                    ],
                  ),
              ],
            );
          },
        ),
      ),
    );
  }
}

class _DesktopVariantPreview extends StatelessWidget {
  const _DesktopVariantPreview({required this.spec});

  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final height = (constraints.maxWidth / 1.72).clamp(280.0, 390.0);
        return SizedBox(
          height: height,
          child: DecoratedBox(
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(14),
              border: Border.all(color: _lineStrong),
              gradient: LinearGradient(
                colors: [_surfaceRaised, _background],
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
              ),
            ),
            child: ClipRRect(
              borderRadius: BorderRadius.circular(14),
              child: Stack(
                children: [
                  Positioned.fill(
                    child: DecoratedBox(
                      decoration: BoxDecoration(
                        gradient: RadialGradient(
                          center: const Alignment(0.75, -0.6),
                          radius: 1.1,
                          colors: [
                            spec.accent.withValues(alpha: 0.12),
                            Colors.transparent,
                          ],
                        ),
                      ),
                    ),
                  ),
                  _MiniWindowChrome(spec: spec),
                  Positioned.fill(
                    top: 28,
                    child: FittedBox(
                      alignment: Alignment.topLeft,
                      fit: BoxFit.contain,
                      child: SizedBox(
                        width: 620,
                        height: 330,
                        child: Padding(
                          padding: const EdgeInsets.all(14),
                          child: _DesktopVariantBody(spec: spec),
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        );
      },
    );
  }
}

class _PhoneVariantPreview extends StatelessWidget {
  const _PhoneVariantPreview({required this.spec});

  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 224,
      height: 440,
      child: DecoratedBox(
        decoration: BoxDecoration(
          color: _background,
          borderRadius: BorderRadius.circular(25),
          border: Border.all(color: _lineStrong, width: 2),
          boxShadow: [
            BoxShadow(
              color: spec.accent.withValues(alpha: 0.12),
              blurRadius: 24,
            ),
          ],
        ),
        child: ClipRRect(
          borderRadius: BorderRadius.circular(23),
          child: Stack(
            children: [
              Positioned.fill(
                child: DecoratedBox(
                  decoration: BoxDecoration(
                    gradient: LinearGradient(
                      colors: [_surface, _background],
                      begin: Alignment.topCenter,
                      end: Alignment.bottomCenter,
                    ),
                  ),
                ),
              ),
              Positioned.fill(
                top: 22,
                child: FittedBox(
                  alignment: Alignment.topCenter,
                  fit: BoxFit.contain,
                  child: SizedBox(
                    width: 198,
                    height: 400,
                    child: Padding(
                      padding: const EdgeInsets.fromLTRB(0, 10, 0, 12),
                      child: _PhoneVariantBody(spec: spec),
                    ),
                  ),
                ),
              ),
              Positioned(
                top: 7,
                left: 78,
                width: 68,
                height: 4,
                child: DecoratedBox(
                  decoration: BoxDecoration(
                    color: _lineStrong,
                    borderRadius: BorderRadius.circular(4),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _MiniWindowChrome extends StatelessWidget {
  const _MiniWindowChrome({required this.spec});

  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 28,
      child: DecoratedBox(
        decoration: const BoxDecoration(
          color: _surfaceLow,
          border: Border(bottom: BorderSide(color: _line)),
        ),
        child: Row(
          children: [
            const SizedBox(width: 12),
            ...[spec.accent, _lineStrong, _lineStrong].map(
              (color) => Padding(
                padding: const EdgeInsets.only(right: 5),
                child: CircleAvatar(radius: 3, backgroundColor: color),
              ),
            ),
            const SizedBox(width: 10),
            const Text(
              'NAGA NETWORK',
              style: TextStyle(
                color: _tertiary,
                fontSize: 8,
                letterSpacing: 1,
                fontWeight: FontWeight.w700,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _DesktopVariantBody extends StatelessWidget {
  const _DesktopVariantBody({required this.spec});

  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) {
    return switch (spec.variant) {
      _DesignVariant.orbit => _OrbitDesktop(spec: spec),
      _DesignVariant.signal => _SignalDesktop(spec: spec),
      _DesignVariant.monolith => _MonolithDesktop(spec: spec),
      _DesignVariant.route => _RouteDesktop(spec: spec),
      _DesignVariant.pulse => _PulseDesktop(spec: spec),
    };
  }
}

class _PhoneVariantBody extends StatelessWidget {
  const _PhoneVariantBody({required this.spec});

  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) {
    return switch (spec.variant) {
      _DesignVariant.orbit => _OrbitPhone(spec: spec),
      _DesignVariant.signal => _SignalPhone(spec: spec),
      _DesignVariant.monolith => _MonolithPhone(spec: spec),
      _DesignVariant.route => _RoutePhone(spec: spec),
      _DesignVariant.pulse => _PulsePhone(spec: spec),
    };
  }
}

class _OrbitDesktop extends StatelessWidget {
  const _OrbitDesktop({required this.spec});

  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        const _MiniSidebar(),
        const SizedBox(width: 16),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _MiniKicker(text: 'GOOD EVENING, NAGA', color: spec.accent),
              const SizedBox(height: 5),
              const Text(
                'Подключение под контролем',
                style: TextStyle(fontSize: 16, fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 10),
              Expanded(
                child: Row(
                  children: [
                    Expanded(
                      flex: 6,
                      child: _MiniPanel(
                        color: _surface,
                        accent: spec.accent,
                        child: Column(
                          mainAxisAlignment: MainAxisAlignment.center,
                          children: [
                            _MiniPower(accent: spec.accent, size: 72),
                            const SizedBox(height: 8),
                            Text(
                              'VPN отключён',
                              style: TextStyle(
                                color: spec.accent,
                                fontSize: 11,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      flex: 7,
                      child: _MiniPanel(
                        color: _surface,
                        accent: spec.accent,
                        child: const _MiniRouteInfo(),
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 8),
              Row(
                children: [
                  Expanded(
                    child: _MiniMetric(
                      value: '12.45 GB',
                      label: 'TRAFFIC TODAY',
                      color: spec.accent,
                    ),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: _MiniMetric(
                      value: '26 DAYS',
                      label: 'SUBSCRIPTION',
                      color: spec.accent,
                    ),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: _MiniMetric(
                      value: 'WORKING',
                      label: 'PROFILE',
                      color: spec.accent,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              SizedBox(
                height: 54,
                child: _MiniPanel(
                  color: spec.accent.withValues(alpha: 0.08),
                  accent: spec.accent,
                  child: Stack(
                    clipBehavior: Clip.none,
                    children: [
                      const Align(
                        alignment: Alignment.centerLeft,
                        child: Text(
                          'Naga Network PRO · Больше серверов и скорости',
                          style: TextStyle(
                            fontSize: 9,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                      ),
                      _MiniMascot(
                        right: -4,
                        bottom: -68,
                        width: 122,
                        height: 188,
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _SignalDesktop extends StatelessWidget {
  const _SignalDesktop({required this.spec});

  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Expanded(
          flex: 7,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  _MiniKicker(text: 'LIVE STATUS', color: spec.accent),
                  const Spacer(),
                  _MiniPill(text: 'CONNECTED', color: spec.accent),
                ],
              ),
              const SizedBox(height: 10),
              const Text(
                'Сигнал стабилен',
                style: TextStyle(fontSize: 24, fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 5),
              const Text(
                'Трафик защищён через Helsinki · 18 ms',
                style: TextStyle(color: _muted, fontSize: 10),
              ),
              const SizedBox(height: 18),
              Expanded(
                child: Row(
                  children: [
                    Expanded(
                      child: _MiniMetric(
                        value: '24.8 GB',
                        label: 'DOWNLOAD',
                        color: spec.accent,
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: _MiniMetric(
                        value: '3.2 GB',
                        label: 'UPLOAD',
                        color: spec.accent,
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: _MiniMetric(
                        value: '04:28',
                        label: 'SESSION',
                        color: spec.accent,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
        Expanded(
          flex: 4,
          child: Stack(
            alignment: Alignment.bottomCenter,
            children: [
              _MiniPanel(
                color: spec.accent.withValues(alpha: 0.08),
                accent: spec.accent,
                child: const Align(
                  alignment: Alignment.topLeft,
                  child: Padding(
                    padding: EdgeInsets.all(10),
                    child: Text(
                      'NAGA / SIGNAL',
                      style: TextStyle(color: _muted, fontSize: 9),
                    ),
                  ),
                ),
              ),
              _MiniMascot(right: -8, bottom: -22, width: 130, height: 210),
            ],
          ),
        ),
      ],
    );
  }
}

class _MonolithDesktop extends StatelessWidget {
  const _MonolithDesktop({required this.spec});

  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        Positioned.fill(
          child: Column(
            children: [
              const _MiniKicker(text: 'NAGA NETWORK / OVERVIEW', color: _muted),
              const Spacer(),
              _MiniPower(accent: spec.accent, size: 118),
              const SizedBox(height: 9),
              Text(
                spec.tagline,
                style: TextStyle(
                  color: spec.accent,
                  fontSize: 12,
                  fontWeight: FontWeight.w700,
                ),
              ),
              const SizedBox(height: 5),
              const Text(
                'Профиль не выбран · добавь ссылку',
                style: TextStyle(color: _muted, fontSize: 10),
              ),
              const Spacer(),
              Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  _MiniPill(text: 'ADD PROFILE', color: spec.accent),
                  const SizedBox(width: 8),
                  const _MiniPill(text: 'SETTINGS', color: _muted),
                ],
              ),
            ],
          ),
        ),
        _MiniMascot(right: -18, bottom: -30, width: 118, height: 190),
      ],
    );
  }
}

class _RouteDesktop extends StatelessWidget {
  const _RouteDesktop({required this.spec});

  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SizedBox(
          width: 132,
          child: Stack(
            children: [
              Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _MiniKicker(text: 'YOUR ROUTE', color: spec.accent),
                  const SizedBox(height: 10),
                  const Text(
                    'Выбери узел',
                    style: TextStyle(fontSize: 18, fontWeight: FontWeight.w800),
                  ),
                  const SizedBox(height: 7),
                  const Text(
                    'Все варианты подписки в одном месте.',
                    style: TextStyle(color: _muted, fontSize: 10, height: 1.4),
                  ),
                ],
              ),
              _MiniMascot(right: -8, bottom: -26, width: 94, height: 150),
            ],
          ),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Column(
            children: [
              _MiniNode(
                name: 'Helsinki · 18 ms',
                type: 'FASTEST',
                color: spec.accent,
                selected: true,
              ),
              const SizedBox(height: 7),
              const _MiniNode(
                name: 'Amsterdam · 31 ms',
                type: 'STABLE',
                color: _muted,
              ),
              const SizedBox(height: 7),
              const _MiniNode(
                name: 'Stockholm · 42 ms',
                type: 'AVAILABLE',
                color: _muted,
              ),
              const Spacer(),
              Align(
                alignment: Alignment.centerRight,
                child: _MiniPill(text: 'CONNECT', color: spec.accent),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _PulseDesktop extends StatelessWidget {
  const _PulseDesktop({required this.spec});

  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            const _MiniKicker(text: 'NAGA / PULSE', color: _muted),
            const Spacer(),
            _MiniPill(text: 'OFFLINE', color: spec.accent),
          ],
        ),
        const SizedBox(height: 10),
        Expanded(
          child: Row(
            children: [
              Expanded(
                flex: 5,
                child: _MiniPanel(
                  color: spec.accent.withValues(alpha: 0.08),
                  accent: spec.accent,
                  child: Stack(
                    children: [
                      const Positioned(
                        left: 10,
                        top: 10,
                        child: Text(
                          'READY WHEN YOU ARE',
                          style: TextStyle(color: _muted, fontSize: 9),
                        ),
                      ),
                      Align(
                        alignment: Alignment.center,
                        child: _MiniPower(accent: spec.accent, size: 86),
                      ),
                      _MiniMascot(
                        right: -38,
                        bottom: -35,
                        width: 118,
                        height: 184,
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                flex: 6,
                child: Column(
                  children: [
                    const _MiniNode(
                      name: 'No profile selected',
                      type: 'IMPORT SUBSCRIPTION',
                      color: _muted,
                    ),
                    const SizedBox(height: 8),
                    Expanded(
                      child: Row(
                        children: [
                          Expanded(
                            child: _MiniMetric(
                              value: '—',
                              label: 'TRAFFIC',
                              color: spec.accent,
                            ),
                          ),
                          const SizedBox(width: 8),
                          Expanded(
                            child: _MiniMetric(
                              value: '—',
                              label: 'SUBSCRIPTION',
                              color: spec.accent,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _OrbitPhone extends StatelessWidget {
  const _OrbitPhone({required this.spec});
  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      const _MiniKicker(text: 'NAGA NETWORK', color: _muted),
      const SizedBox(height: 8),
      const Text(
        'Добро пожаловать',
        style: TextStyle(fontSize: 17, fontWeight: FontWeight.w800),
      ),
      const SizedBox(height: 4),
      const Text(
        'Твоя сеть под защитой',
        style: TextStyle(color: _muted, fontSize: 10),
      ),
      const SizedBox(height: 10),
      Expanded(
        child: _MiniPanel(
          color: _surfaceRaised,
          accent: spec.accent,
          child: Stack(
            children: [
              const Positioned(
                left: 10,
                top: 10,
                child: Text(
                  'READY TO CONNECT',
                  style: TextStyle(color: _muted, fontSize: 8),
                ),
              ),
              Align(
                alignment: Alignment.center,
                child: _MiniPower(accent: spec.accent, size: 72),
              ),
            ],
          ),
        ),
      ),
      const SizedBox(height: 10),
      SizedBox(
        height: 54,
        child: _MiniPanel(
          color: spec.accent.withValues(alpha: 0.08),
          accent: spec.accent,
          child: Stack(
            clipBehavior: Clip.none,
            children: [
              const Align(
                alignment: Alignment.centerLeft,
                child: Text(
                  'Импорт профиля\nпо HTTPS-ссылке',
                  style: TextStyle(fontSize: 9, fontWeight: FontWeight.w700),
                ),
              ),
              _MiniMascot(right: -4, bottom: -54, width: 90, height: 140),
            ],
          ),
        ),
      ),
      const SizedBox(height: 8),
      Row(
        children: [
          Expanded(
            child: _MiniMetric(
              value: '—',
              label: 'TRAFFIC TODAY',
              color: spec.accent,
            ),
          ),
          const SizedBox(width: 7),
          Expanded(
            child: _MiniMetric(
              value: '—',
              label: 'SUBSCRIPTION',
              color: spec.accent,
            ),
          ),
        ],
      ),
      const Spacer(),
      const _MiniBottomNav(active: 0),
    ],
  );
}

class _SignalPhone extends StatelessWidget {
  const _SignalPhone({required this.spec});
  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Row(
        children: [
          const _MiniKicker(text: 'LIVE', color: _muted),
          const Spacer(),
          _MiniPill(text: 'ON', color: spec.accent),
        ],
      ),
      const SizedBox(height: 12),
      const Text(
        'Сигнал стабилен',
        style: TextStyle(fontSize: 18, fontWeight: FontWeight.w800),
      ),
      const SizedBox(height: 4),
      const Text(
        'Helsinki · 18 ms',
        style: TextStyle(color: _muted, fontSize: 10),
      ),
      const SizedBox(height: 10),
      Expanded(
        child: Stack(
          children: [
            _MiniPanel(
              color: spec.accent.withValues(alpha: 0.08),
              accent: spec.accent,
              child: const SizedBox.expand(),
            ),
            _MiniMascot(right: -16, bottom: -30, width: 124, height: 196),
          ],
        ),
      ),
      const SizedBox(height: 10),
      Row(
        children: [
          Expanded(
            child: _MiniMetric(
              value: '24.8',
              label: 'GB DOWN',
              color: spec.accent,
            ),
          ),
          const SizedBox(width: 7),
          Expanded(
            child: _MiniMetric(
              value: '03:12',
              label: 'SESSION',
              color: spec.accent,
            ),
          ),
        ],
      ),
      const Spacer(),
      const _MiniBottomNav(active: 0),
    ],
  );
}

class _MonolithPhone extends StatelessWidget {
  const _MonolithPhone({required this.spec});
  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) => Stack(
    children: [
      Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const _MiniKicker(text: 'OVERVIEW', color: _muted),
          const Spacer(),
          Center(child: _MiniPower(accent: spec.accent, size: 116)),
          const SizedBox(height: 10),
          Center(
            child: Text(
              spec.tagline,
              style: TextStyle(
                color: spec.accent,
                fontWeight: FontWeight.w700,
                fontSize: 12,
              ),
            ),
          ),
          const SizedBox(height: 5),
          const Center(
            child: Text(
              'Добавь профиль для старта',
              style: TextStyle(color: _muted, fontSize: 10),
            ),
          ),
          const Spacer(),
          _MiniNode(
            name: 'Добавить профиль',
            type: 'HTTPS LINK',
            color: spec.accent,
          ),
          const SizedBox(height: 10),
          const _MiniBottomNav(active: 0),
        ],
      ),
      _MiniMascot(right: -30, bottom: 24, width: 110, height: 176),
    ],
  );
}

class _RoutePhone extends StatelessWidget {
  const _RoutePhone({required this.spec});
  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      const _MiniKicker(text: 'YOUR ROUTE', color: _muted),
      const SizedBox(height: 8),
      const Text(
        'Выбери узел',
        style: TextStyle(fontSize: 19, fontWeight: FontWeight.w800),
      ),
      const SizedBox(height: 4),
      const Text(
        'Быстрый маршрут для текущей сессии.',
        style: TextStyle(color: _muted, fontSize: 10),
      ),
      const SizedBox(height: 12),
      Expanded(
        child: Stack(
          children: [
            Column(
              children: [
                _MiniNode(
                  name: 'Helsinki',
                  type: '18 ms · FASTEST',
                  color: spec.accent,
                  selected: true,
                ),
                const SizedBox(height: 8),
                const _MiniNode(
                  name: 'Amsterdam',
                  type: '31 ms · STABLE',
                  color: _muted,
                ),
                const SizedBox(height: 8),
                const _MiniNode(
                  name: 'Stockholm',
                  type: '42 ms · AVAILABLE',
                  color: _muted,
                ),
              ],
            ),
            _MiniMascot(right: -22, bottom: -34, width: 112, height: 180),
          ],
        ),
      ),
      _MiniPill(text: 'CONNECT', color: spec.accent),
      const Spacer(),
      const _MiniBottomNav(active: 1),
    ],
  );
}

class _PulsePhone extends StatelessWidget {
  const _PulsePhone({required this.spec});
  final _DesignVariantSpec spec;

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Row(
        children: [
          const _MiniKicker(text: 'NAGA / PULSE', color: _muted),
          const Spacer(),
          _MiniPill(text: 'OFFLINE', color: spec.accent),
        ],
      ),
      const SizedBox(height: 14),
      Expanded(
        child: _MiniPanel(
          color: spec.accent.withValues(alpha: 0.08),
          accent: spec.accent,
          child: Stack(
            children: [
              const Positioned(
                left: 10,
                top: 10,
                child: Text(
                  'QUIET MODE',
                  style: TextStyle(color: _muted, fontSize: 8),
                ),
              ),
              Align(
                alignment: Alignment.center,
                child: _MiniPower(accent: spec.accent, size: 84),
              ),
              _MiniMascot(right: -26, bottom: -27, width: 110, height: 178),
            ],
          ),
        ),
      ),
      const SizedBox(height: 9),
      const _MiniNode(
        name: 'Профиль не выбран',
        type: 'ADD SUBSCRIPTION',
        color: _muted,
      ),
      const SizedBox(height: 9),
      Row(
        children: [
          Expanded(
            child: _MiniMetric(
              value: '—',
              label: 'TRAFFIC',
              color: spec.accent,
            ),
          ),
          const SizedBox(width: 7),
          Expanded(
            child: _MiniMetric(value: '—', label: 'TIME', color: spec.accent),
          ),
        ],
      ),
      const Spacer(),
      const _MiniBottomNav(active: 0),
    ],
  );
}

class _MiniSidebar extends StatelessWidget {
  const _MiniSidebar();

  @override
  Widget build(BuildContext context) => SizedBox(
    width: 58,
    child: Column(
      children: [
        Image.asset(_brandMarkAsset, width: 22, height: 22),
        const SizedBox(height: 18),
        for (final icon in [
          Icons.home_rounded,
          Icons.public_rounded,
          Icons.bar_chart_rounded,
          Icons.settings_outlined,
        ]) ...[
          Icon(
            icon,
            size: 14,
            color: icon == Icons.home_rounded ? _accent : _tertiary,
          ),
          const SizedBox(height: 16),
        ],
      ],
    ),
  );
}

class _MiniKicker extends StatelessWidget {
  const _MiniKicker({required this.text, required this.color});
  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => Text(
    text,
    style: TextStyle(
      color: color,
      fontSize: 8,
      letterSpacing: 1.1,
      fontWeight: FontWeight.w800,
    ),
  );
}

class _MiniPill extends StatelessWidget {
  const _MiniPill({required this.text, required this.color});
  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 5),
    decoration: BoxDecoration(
      color: color.withValues(alpha: 0.14),
      borderRadius: BorderRadius.circular(6),
    ),
    child: Text(
      text,
      style: TextStyle(
        color: color,
        fontSize: 8,
        fontWeight: FontWeight.w800,
        letterSpacing: 0.5,
      ),
    ),
  );
}

class _MiniPower extends StatelessWidget {
  const _MiniPower({required this.accent, required this.size});
  final Color accent;
  final double size;

  @override
  Widget build(BuildContext context) => Container(
    width: size,
    height: size,
    decoration: BoxDecoration(
      shape: BoxShape.circle,
      color: _surfaceElevated,
      border: Border.all(color: accent, width: 2),
      boxShadow: [
        BoxShadow(color: accent.withValues(alpha: 0.28), blurRadius: 18),
      ],
    ),
    child: Icon(
      Icons.power_settings_new_rounded,
      color: _text,
      size: size * 0.38,
    ),
  );
}

class _MiniPanel extends StatelessWidget {
  const _MiniPanel({
    required this.color,
    required this.accent,
    required this.child,
  });
  final Color color;
  final Color accent;
  final Widget child;

  @override
  Widget build(BuildContext context) => Container(
    decoration: BoxDecoration(
      color: color,
      borderRadius: BorderRadius.circular(10),
      border: Border.all(color: accent.withValues(alpha: 0.2)),
    ),
    child: Padding(padding: const EdgeInsets.all(10), child: child),
  );
}

class _MiniRouteInfo extends StatelessWidget {
  const _MiniRouteInfo();

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    mainAxisAlignment: MainAxisAlignment.center,
    children: [
      const _MiniKicker(text: 'CURRENT ROUTE', color: _muted),
      const SizedBox(height: 8),
      const Text(
        'Helsinki',
        style: TextStyle(fontSize: 18, fontWeight: FontWeight.w800),
      ),
      const SizedBox(height: 4),
      const Text(
        '18 ms · Rule / Auto',
        style: TextStyle(color: _muted, fontSize: 10),
      ),
      const SizedBox(height: 12),
      const _MiniPill(text: 'CHANGE NODE', color: _accent),
    ],
  );
}

class _MiniMetric extends StatelessWidget {
  const _MiniMetric({
    required this.value,
    required this.label,
    required this.color,
  });
  final String value;
  final String label;
  final Color color;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(10),
    decoration: BoxDecoration(
      color: _surface,
      borderRadius: BorderRadius.circular(9),
      border: Border.all(color: _line),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        Text(
          value,
          style: TextStyle(
            color: color,
            fontSize: 14,
            fontWeight: FontWeight.w800,
          ),
        ),
        const SizedBox(height: 4),
        Text(
          label,
          style: const TextStyle(
            color: _tertiary,
            fontSize: 7,
            letterSpacing: 0.8,
          ),
        ),
      ],
    ),
  );
}

class _MiniNode extends StatelessWidget {
  const _MiniNode({
    required this.name,
    required this.type,
    required this.color,
    this.selected = false,
  });
  final String name;
  final String type;
  final Color color;
  final bool selected;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
    decoration: BoxDecoration(
      color: selected ? color.withValues(alpha: 0.12) : _surface,
      borderRadius: BorderRadius.circular(8),
      border: Border.all(
        color: selected ? color.withValues(alpha: 0.55) : _line,
      ),
    ),
    child: Row(
      children: [
        Container(
          width: 7,
          height: 7,
          decoration: BoxDecoration(shape: BoxShape.circle, color: color),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            name,
            style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w700),
            overflow: TextOverflow.ellipsis,
          ),
        ),
        Flexible(
          child: Text(
            type,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            textAlign: TextAlign.right,
            style: TextStyle(
              color: color,
              fontSize: 7,
              fontWeight: FontWeight.w800,
              letterSpacing: 0.5,
            ),
          ),
        ),
      ],
    ),
  );
}

class _MiniBottomNav extends StatelessWidget {
  const _MiniBottomNav({required this.active});
  final int active;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(vertical: 8),
    decoration: const BoxDecoration(
      border: Border(top: BorderSide(color: _line)),
    ),
    child: Row(
      mainAxisAlignment: MainAxisAlignment.spaceAround,
      children: [
        for (var index = 0; index < 4; index++)
          Icon(
            [
              Icons.home_rounded,
              Icons.public_rounded,
              Icons.bar_chart_rounded,
              Icons.settings_outlined,
            ][index],
            color: index == active ? _accent : _tertiary,
            size: 15,
          ),
      ],
    ),
  );
}

class _MiniMascot extends StatelessWidget {
  const _MiniMascot({
    required this.right,
    required this.bottom,
    required this.width,
    required this.height,
  });
  final double right;
  final double bottom;
  final double width;
  final double height;

  @override
  Widget build(BuildContext context) => Positioned(
    right: right,
    bottom: bottom,
    width: width,
    height: height,
    child: IgnorePointer(
      child: Opacity(
        opacity: 0.9,
        child: Image.asset(
          _brandMascotCutoutAsset,
          fit: BoxFit.contain,
          alignment: Alignment.topCenter,
        ),
      ),
    ),
  );
}

enum ConnectionStatus { disconnected, connecting, connected, error }

class TrafficSample {
  const TrafficSample({
    required this.at,
    required this.upload,
    required this.download,
  });

  final DateTime at;
  final int upload;
  final int download;
}

class DashboardPage extends StatefulWidget {
  const DashboardPage({super.key, this.controlPlane, this.initialImportURL});

  final ControlPlaneClient? controlPlane;
  final String? initialImportURL;

  @override
  State<DashboardPage> createState() => _DashboardPageState();
}

class _DashboardPageState extends State<DashboardPage> {
  ConnectionStatus _status = ConnectionStatus.disconnected;
  String _profileName = 'Профиль не импортирован';
  String _profileUrl = '';
  String? _profileId;
  String _profileProvider = '';
  List<StoredProfileSummary> _profiles = const [];
  ProfilePreview? _profilePreview;
  ProfileNodes? _profileNodes;
  List<UnifiedNode> _unifiedNodes = const [];
  ConnectionPolicy? _connectionPolicy;
  RoutingPolicy? _routingPolicy;
  List<AppRoute> _apps = const [];
  List<DiscoveredApp> _discoveredApps = const [];
  RuntimeSnapshot? _runtime;
  List<TrafficSample> _trafficSamples = const [];
  Timer? _runtimePoller;
  Timer? _profileUpdater;
  bool _isImportingProfile = false;
  bool _runtimeRequestInFlight = false;
  int _runtimeMisses = 0;
  bool _autoUpdate = true;
  bool _autoAppUpdate = true;
  AppUpdatePrefs _appUpdatePrefs = const AppUpdatePrefs();
  String? _appUpdatePrefsPath;
  String? _trafficModeError;
  bool? _controlPlaneReachable;
  late final ControlPlaneClient _controlPlane;
  AppUpdater? _updater;
  String _uiVersion = embeddedAppVersion;
  String _controlVersion = '';
  bool _updateBusy = false;
  double? _updateProgress;
  String? _updateMessage;
  String? _updateError;
  UpdateCheckResult? _updateCheck;
  bool _olcrtcReady = true;

  bool get _hasProfile => _profileId != null || _profileUrl.isNotEmpty;

  @override
  void initState() {
    super.initState();
    _controlPlane = widget.controlPlane ?? ControlPlaneClient();
    _updater = AppUpdater(currentVersion: _uiVersion);
    _trayChannel.setMethodCallHandler(_handleTrayMethod);
    unawaited(_startDashboard());
    _runtimePoller = Timer.periodic(
      const Duration(seconds: 2),
      (_) => _refreshRuntime(),
    );
    unawaited(_syncTray());
  }

  Future<void> _startDashboard() async {
    if (nagaRunsOnAndroid) {
      final token = await AndroidVpn.instance.waitForControlToken();
      if (token != null) _controlPlane.updateToken(token);
      await AndroidVpn.instance.waitUntilHealthy(_controlPlane);
    }
    await Future.wait([
      _loadUiVersion(),
      _loadAppUpdatePrefs(),
      _bootstrap(),
    ]);
    if (!mounted) return;
    await _maybeAutoUpdateApp();
  }

  Future<void> _bootstrap() async {
    await _restoreProfiles();
    await _restoreWorkspaceSettings();
    final url = widget.initialImportURL;
    if (url == null || url.isEmpty || !mounted) {
      return;
    }
    await _importProfile(prefilledUrl: url, skipUrlDialog: true);
  }

  Future<void> _loadAppUpdatePrefs() async {
    if (runningUnderFlutterTest()) return;
    final path = await resolveAppUpdatePrefsPath();
    final prefs = loadAppUpdatePrefs(path: path);
    if (!mounted) return;
    setState(() {
      _appUpdatePrefsPath = path;
      _appUpdatePrefs = prefs;
      _autoAppUpdate = prefs.autoInstall;
    });
  }

  void _persistAppUpdatePrefs() {
    if (runningUnderFlutterTest()) return;
    saveAppUpdatePrefs(
      _appUpdatePrefs.copyWith(autoInstall: _autoAppUpdate),
      path: _appUpdatePrefsPath,
    );
  }

  @override
  void dispose() {
    _trayChannel.setMethodCallHandler(null);
    _runtimePoller?.cancel();
    _profileUpdater?.cancel();
    _updater?.close();
    super.dispose();
  }

  Future<void> _loadUiVersion() async {
    try {
      final info = await PackageInfo.fromPlatform();
      final version = info.version.trim();
      if (version.isEmpty || !mounted) return;
      _updater?.close();
      setState(() {
        _uiVersion = version;
        _updater = AppUpdater(currentVersion: version);
      });
    } on MissingPluginException {
      // Widget tests and hosts without the plugin keep the embedded version.
    } catch (_) {}
  }

  Future<void> _refreshControlVersion() async {
    try {
      final health = await _controlPlane.healthInfo();
      if (!mounted) return;
      setState(() {
        _controlVersion = health.version;
        _olcrtcReady = health.olcrtcReady;
      });
    } on ControlPlaneException {
      // Banner stays quiet while the control-plane is down.
    }
  }

  Future<void> _handleTrayMethod(MethodCall call) async {
    switch (call.method) {
      case 'toggleConnection':
        await _toggleConnection();
      case 'openServerPicker':
        if (mounted) await _showServerPicker();
      case 'quitApplication':
        await _quitFromTray();
      case 'installConfig':
        final raw = call.arguments is String ? call.arguments as String : '';
        final url = subscriptionFetchURL(raw);
        if (url != null && mounted) {
          await _importProfile(prefilledUrl: url, skipUrlDialog: true);
        }
    }
  }

  Future<void> _syncTray() async {
    final status = _controlPlaneReachable == false
        ? 'error'
        : switch (_status) {
            ConnectionStatus.connected => 'connected',
            ConnectionStatus.connecting => 'connecting',
            ConnectionStatus.error => 'error',
            ConnectionStatus.disconnected => 'disconnected',
          };
    try {
      await _trayChannel.invokeMethod<void>('updateStatus', {
        'status': status,
        'profile': _profileName,
        'server': _selectedNodeName,
      });
    } on MissingPluginException {
      // Web/mobile builds do not provide the desktop tray channel.
    } on PlatformException {
      // Keep the UI usable when the native tray is unavailable.
    }
  }

  Future<void> _invokeWindowChrome(String method) async {
    try {
      await _trayChannel.invokeMethod<void>(method);
    } on MissingPluginException {
      // Linux and non-Windows builds leave window chrome unimplemented.
    } on PlatformException {
      // Keep the UI usable if the native host is gone.
    }
  }

  Future<void> _quitFromTray() async {
    try {
      await _controlPlane.stopRuntime();
    } on ControlPlaneException {
      // Exit must remain available even if the runtime cannot be stopped.
    }
    if (nagaRunsOnAndroid) await AndroidVpn.instance.stop();
    try {
      await _trayChannel.invokeMethod<void>('terminate');
    } on MissingPluginException {
      // No native tray on web/mobile.
    } on PlatformException {
      // Keep the process alive if the native host is already gone.
    }
  }

  Future<void> _restoreProfiles() async {
    List<StoredProfileSummary> profiles = const [];
    RuntimeSnapshot? runtime;
    var reachable = false;
    try {
      profiles = await _controlPlane.listProfiles();
    } on ControlPlaneException {
      // The control-plane may not be running yet; the home banner explains how
      // to start the local service instead of showing a false empty onboarding.
    }
    try {
      runtime = await _controlPlane.runtimeStatus();
      reachable = true;
    } on ControlPlaneException {
      // A profiles response cannot establish the current VPN state.
    }
    unawaited(_refreshControlVersion());
    if (mounted) {
      setState(() => _controlPlaneReachable = reachable);
    }
    if (!mounted) return;
    if (profiles.isNotEmpty) {
      var selected = profiles.last;
      final activeID = runtime?.profileId;
      if (activeID != null) {
        for (final profile in profiles) {
          if (profile.id == activeID) {
            selected = profile;
            break;
          }
        }
      }
      setState(() {
        _profiles = profiles;
        _adoptProfile(selected);
      });
      await _loadNodes(selected.id);
      _scheduleProfileUpdate();
    }
    if (runtime != null && mounted) _applyRuntime(runtime);
  }

  Future<void> _restoreWorkspaceSettings() async {
    try {
      final policy = await _controlPlane.connectionPolicy();
      if (mounted) setState(() => _connectionPolicy = policy);
    } on ControlPlaneException {
      // Defaults are rendered while the control-plane is unavailable.
    }
    try {
      final routing = await _controlPlane.routingPolicy();
      if (mounted) {
        setState(() {
          _routingPolicy = routing;
          _apps = routing.apps;
        });
      }
    } on ControlPlaneException {
      // Defaults are rendered while the control-plane is unavailable.
    }
    try {
      final nodes = await _controlPlane.listUnifiedNodes();
      if (mounted) {
        setState(
          () => _unifiedNodes = nagaMergeProbedNodes(nodes, _unifiedNodes),
        );
      }
    } on ControlPlaneException {
      // The nodes page shows an actionable empty state.
    }
    await _refreshDiscoveredApps();
  }

  Future<List<DiscoveredApp>> _refreshDiscoveredApps() async {
    if (nagaRunsOnAndroid) {
      final discovered = await AndroidVpn.instance.listPackages();
      if (mounted) setState(() => _discoveredApps = discovered);
      return discovered;
    }
    try {
      final discovered = await _controlPlane.listDiscoveredApps();
      if (mounted) setState(() => _discoveredApps = discovered);
      return discovered;
    } on ControlPlaneException {
      if (mounted) setState(() => _discoveredApps = const []);
      return const [];
    }
  }

  Future<void> _reloadRoutingPolicy() async {
    try {
      final routing = await _controlPlane.routingPolicy();
      if (mounted) {
        setState(() {
          _routingPolicy = routing;
          _apps = routing.apps;
        });
      }
    } on ControlPlaneException {
      // Keep the last known policy if reload fails after a successful mutation.
    }
  }

  ConnectionPolicy get _effectiveConnectionPolicy =>
      _connectionPolicy ??
      const ConnectionPolicy(
        mode: 'auto',
        trafficMode: 'tun',
        preferredCountry: '',
        preferredProtocol: '',
        networkClass: 'unknown',
        tuicFallbackEnabled: true,
      );

  RoutingPolicy get _effectiveRoutingPolicy =>
      _routingPolicy ??
      const RoutingPolicy(mode: 'all_vpn', apps: []);

  Future<void> _saveConnectionPolicy(ConnectionPolicy value) async {
    try {
      final saved = await _controlPlane.saveConnectionPolicy(value);
      if (mounted) {
        setState(() {
          _connectionPolicy = saved;
          _trafficModeError = null;
        });
      }
    } on ControlPlaneException catch (error) {
      if (mounted) {
        setState(() => _trafficModeError = error.message);
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(error.message)));
      }
    }
  }

  void _selectTrafficMode(String trafficMode) {
    if (nagaRunsOnAndroid && trafficMode == 'system_proxy') {
      return;
    }
    final current = _effectiveConnectionPolicy;
    unawaited(
      _saveConnectionPolicy(
        ConnectionPolicy(
          mode: current.mode,
          trafficMode: trafficMode,
          preferredCountry: current.preferredCountry,
          preferredProtocol: current.preferredProtocol,
          networkClass: current.networkClass,
          tuicFallbackEnabled: current.tuicFallbackEnabled,
        ),
      ),
    );
  }

  Future<void> _saveRoutingPolicy(RoutingPolicy value) async {
    if ((value.mode == 'selected_vpn' || value.mode == 'selected_direct') &&
        !value.apps.any(
          (app) => app.enabled && app.packageOrProcessId.trim().isNotEmpty,
        )) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text(
              'Сначала добавь хотя бы одно приложение — иначе весь трафик уйдёт в обход VPN.',
            ),
          ),
        );
      }
      return;
    }
    try {
      final saved = await _controlPlane.saveRoutingPolicy(value);
      if (mounted) {
        setState(() {
          _routingPolicy = saved;
          _apps = saved.apps;
        });
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Маршрутизация сохранена.')),
        );
        unawaited(_rebuildAndroidVpnIfConnected());
      }
    } on ControlPlaneException catch (error) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(error.message)));
      }
    }
  }

  Future<void> _saveAppRoute(AppRoute app) async {
    try {
      await _controlPlane.saveApp(app);
      await _reloadRoutingPolicy();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Правило приложения сохранено.')),
        );
        unawaited(_rebuildAndroidVpnIfConnected());
      }
    } on ControlPlaneException catch (error) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(error.message)));
      }
    }
  }

  Future<void> _deleteAppRoute(AppRoute app) async {
    try {
      await _controlPlane.deleteApp(app.id);
      await _reloadRoutingPolicy();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Правило приложения удалено.')),
        );
        unawaited(_rebuildAndroidVpnIfConnected());
      }
    } on ControlPlaneException catch (error) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(error.message)));
      }
    }
  }

  Future<void> _selectUnifiedNode(UnifiedNode node) async {
    try {
      final crossesEngine =
          node.protocol == 'olcRTC' || _runtime?.engine == 'olcrtc';
      if (_isConnected && !crossesEngine) {
        final runtime = await _controlPlane.selectRuntime(
          node.runtimeTag,
          profileId: node.profileId,
        );
        if (mounted) _applyRuntime(runtime);
        await _loadNodes(node.profileId);
        if (mounted) unawaited(_syncTray());
      } else {
        await _controlPlane.selectMode(node.profileId, node.tag);
        await _loadNodes(node.profileId);
        if (_isConnected) {
          final runtime = await _controlPlane.runtimeStatus();
          if (mounted) _applyRuntime(runtime);
          if (mounted) unawaited(_syncTray());
        }
      }
      if (mounted) {
        await _restoreWorkspaceSettings();
        if (!mounted) return;
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('Узел выбран: ${node.tag}')));
      }
    } on ControlPlaneException catch (error) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(error.message)));
      }
    }
  }

  Future<void> _loadNodes(String profileId) async {
    try {
      final nodes = await _controlPlane.listNodes(profileId);
      if (!mounted) return;
      setState(() => _profileNodes = nodes);
      try {
        final unified = await _controlPlane.listUnifiedNodes();
        if (mounted) {
          setState(
            () => _unifiedNodes = nagaMergeProbedNodes(unified, _unifiedNodes),
          );
        }
      } on ControlPlaneException {
        // Keep the profile node list even if the aggregate endpoint is down.
      }
    } on ControlPlaneException {
      if (mounted) setState(() => _profileNodes = null);
    }
  }

  ProfilePreview _previewFromSummary(StoredProfileSummary summary) {
    return ProfilePreview(
      profileId: summary.id,
      profileTitle: summary.name,
      providerName: summary.providerName,
      engine: summary.engine,
      uploadBytes: summary.uploadBytes,
      downloadBytes: summary.downloadBytes,
      totalBytes: summary.totalBytes,
      unlimited: summary.unlimited,
      expireUtc: summary.expireUtc,
      updateIntervalSeconds: summary.updateIntervalSeconds,
      canConnect: summary.canConnect,
      olcrtcAvailable: summary.olcrtcAvailable,
    );
  }

  Future<void> _reloadProfiles() async {
    try {
      final profiles = await _controlPlane.listProfiles();
      if (mounted) setState(() => _profiles = profiles);
    } on ControlPlaneException {
      // The profile operation itself already succeeded; keep the current list
      // until the next refresh if the metadata request temporarily fails.
    }
  }

  void _adoptProfile(StoredProfileSummary selected) {
    _profileId = selected.id;
    _profileName = selected.name;
    _profileProvider = selected.providerName;
    _profilePreview = _previewFromSummary(selected);
  }

  Future<void> _selectMode(String mode) async {
    final profileId = _profileId;
    if (profileId == null) return;
    try {
      final nodes = await _controlPlane.selectMode(profileId, mode);
      if (!mounted) return;
      setState(() => _profileNodes = nodes);
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('Узел выбран: $mode.')));
    } on ControlPlaneException catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(error.message)));
    }
  }

  Future<void> _selectAutoNode() async {
    final current = _effectiveConnectionPolicy;
    if (current.mode != 'auto') {
      await _saveConnectionPolicy(
        ConnectionPolicy(
          mode: 'auto',
          trafficMode: current.trafficMode,
          preferredCountry: '',
          preferredProtocol: '',
          networkClass: current.networkClass,
          tuicFallbackEnabled: current.tuicFallbackEnabled,
        ),
      );
    }
    if (_isConnected) {
      try {
        final runtime = await _controlPlane.selectRuntimeAuto();
        if (mounted) {
          _applyRuntime(runtime);
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('Автовыбор из 3 наиболее подходящих серверов.'),
            ),
          );
        }
      } on ControlPlaneException catch (error) {
        if (mounted) {
          ScaffoldMessenger.of(context)
              .showSnackBar(SnackBar(content: Text(error.message)));
        }
      }
      return;
    }
    final autoTag = _findNodeTag(_profileNodes?.nodes ?? const [], 'auto');
    if (autoTag == null) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text(
              'Автовыбор сохранён: при подключении проверим 3 наиболее подходящих сервера.',
            ),
          ),
        );
      }
      return;
    }
    await _selectMode(autoTag);
  }

  Future<List<UnifiedNode>> _probeUnifiedNodes() async {
    try {
      final nodes = await _controlPlane.probeUnifiedNodes();
      RuntimeSnapshot? runtime;
      try {
        runtime = await _controlPlane.runtimeStatus();
      } on ControlPlaneException {
        runtime = null;
      }
      if (mounted) {
        setState(
          () => _unifiedNodes = nagaMergeProbedNodes(nodes, _unifiedNodes),
        );
        if (runtime != null) {
          _applyRuntime(runtime);
        }
      }
      return nodes;
    } on ControlPlaneException catch (error) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(error.message)));
      }
      rethrow;
    }
  }

  Future<void> _showServerPicker() async {
    if (_unifiedNodes.isEmpty) {
      if (mounted) setState(() => _selectedPage = 7);
      return;
    }
    final compact = MediaQuery.sizeOf(context).width < NagaBreakpoints.compact;
    Widget picker() {
      return Material(
        color: _surfaceRaised,
        borderRadius: const BorderRadius.all(Radius.circular(18)),
        child: SafeArea(
          top: false,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(20, 20, 20, 16),
            child: SingleChildScrollView(
              child: NagaNodesView(
                nodes: _unifiedNodes,
                connected: _isConnected,
                showClose: true,
                networkClass: _effectiveConnectionPolicy.networkClass,
                onSelect: (node) {
                  Navigator.of(context).pop();
                  unawaited(_selectUnifiedNode(node));
                },
                onSelectAuto: _hasProfile
                    ? () {
                        Navigator.of(context).pop();
                        unawaited(_selectAutoNode());
                      }
                    : null,
                onProbe: _hasProfile ? _probeUnifiedNodes : null,
              ),
            ),
          ),
        ),
      );
    }

    if (compact) {
      await showModalBottomSheet<void>(
        context: context,
        isScrollControlled: true,
        backgroundColor: Colors.transparent,
        builder: (context) =>
            FractionallySizedBox(heightFactor: 0.86, child: picker()),
      );
      return;
    }
    await showDialog<void>(
      context: context,
      builder: (context) => Dialog(
        backgroundColor: _surfaceRaised,
        child: SizedBox(width: 560, height: 620, child: picker()),
      ),
    );
  }

  Future<void> _refreshRuntime() async {
    if (_runtimeRequestInFlight || _isImportingProfile) return;
    _runtimeRequestInFlight = true;
    try {
      final runtime = await _controlPlane.runtimeStatus();
      if (!mounted) return;
      _runtimeMisses = 0;
      _controlPlaneReachable = true;
      _applyRuntime(runtime);
    } on ControlPlaneException {
      if (!mounted) return;
      _runtimeMisses++;
      if (_status == ConnectionStatus.connecting) {
        return;
      }
      if (_runtimeMisses >= 3) {
        setState(() => _controlPlaneReachable = false);
      }
      unawaited(_syncTray());
    } finally {
      _runtimeRequestInFlight = false;
    }
  }

  Future<bool> _disconnectForUpdate({bool silent = false}) async {
    if (!_isConnected && _status != ConnectionStatus.connecting) {
      return true;
    }
    if (!silent) {
      final confirmed = await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          backgroundColor: _surface,
          title: const Text('Отключить VPN?'),
          content: const Text(
            'Перед установкой обновления нужно отключить VPN.',
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(context).pop(false),
              child: const Text('Отмена'),
            ),
            FilledButton(
              onPressed: () => Navigator.of(context).pop(true),
              child: const Text('Отключить'),
            ),
          ],
        ),
      );
      if (confirmed != true) return false;
    }
    try {
      final runtime = await _controlPlane.stopRuntime();
      if (nagaRunsOnAndroid) await AndroidVpn.instance.stop();
      if (mounted) _applyRuntime(runtime);
      return true;
    } on ControlPlaneException catch (error) {
      if (nagaRunsOnAndroid) await AndroidVpn.instance.stop();
      if (mounted) {
        setState(() => _updateError = error.message);
      }
      return false;
    }
  }

  Future<bool> _waitForControlVersion(String expected) async {
    final want = Semver.tryParse(expected);
    final deadline = DateTime.now().add(const Duration(seconds: 25));
    while (DateTime.now().isBefore(deadline)) {
      try {
        final health = await _controlPlane.healthInfo();
        final got = Semver.tryParse(health.version);
        if (mounted) {
          setState(() => _controlVersion = health.version);
        }
        if (want != null && got != null && got >= want) {
          return true;
        }
      } on ControlPlaneException {
        // Keep polling until the restarted unit answers.
      }
      await Future<void>.delayed(const Duration(milliseconds: 400));
    }
    return false;
  }

  Future<void> _checkAppUpdate({bool quiet = false}) async {
    final updater = _updater;
    if (updater == null || _updateBusy) return;
    setState(() {
      _updateBusy = true;
      _updateError = null;
      _updateMessage = quiet ? null : 'Проверяем GitHub…';
      _updateProgress = null;
    });
    try {
      final result = await updater.checkLatest();
      if (!mounted) return;
      _appUpdatePrefs = _appUpdatePrefs.copyWith(
        lastCheckUtc: DateTime.now().toUtc(),
        lastLatestVersion: result.latestVersion,
      );
      _persistAppUpdatePrefs();
      setState(() {
        _updateCheck = result;
        _updateBusy = false;
        _updateMessage = result.newerAvailable
            ? result.missingAssets.isEmpty
                  ? 'Доступна ${result.latestVersion}'
                  : 'В релизе нет ${result.missingAssets.join(', ')}'
            : quiet
            ? null
            : 'Установлена актуальная версия';
      });
    } on AppUpdateException catch (error) {
      if (!mounted) return;
      setState(() {
        _updateBusy = false;
        _updateError = quiet ? null : error.message;
        _updateMessage = null;
      });
    }
  }

  Future<void> _installAppUpdate({bool auto = false}) async {
    final updater = _updater;
    final check = _updateCheck;
    final release = check?.release;
    if (updater == null || check == null || release == null || _updateBusy) {
      return;
    }
    if (!check.newerAvailable) return;
    if (check.missingAssets.isNotEmpty) {
      setState(() {
        _updateError =
            'В релизе нет ${check.missingAssets.join(', ')}. UI без runtime не обновляем.';
      });
      return;
    }
    setState(() {
      _updateBusy = true;
      _updateError = null;
      _updateMessage = auto
          ? 'Автообновление: скачиваем ${check.latestVersion}…'
          : 'Скачиваем ${check.latestVersion}…';
      _updateProgress = 0;
    });
    try {
      final files = await updater.downloadRequired(
        release,
        check.latestVersion,
        onProgress: (received, total) {
          if (!mounted || total <= 0) return;
          setState(() => _updateProgress = received / total);
        },
      );
      if (!mounted) return;
      setState(() {
        _updateMessage = auto
            ? 'Автообновление: устанавливаем…'
            : 'Устанавливаем…';
        _updateProgress = null;
      });
      await updater.applyDownloaded(
        files,
        version: check.latestVersion,
        disconnectIfNeeded: () => _disconnectForUpdate(silent: auto),
        waitForControlVersion: _waitForControlVersion,
      );
      if (!mounted) return;
      setState(() {
        _updateBusy = false;
        _updateMessage = 'Обновление запущено';
      });
    } on AppUpdateException catch (error) {
      if (!mounted) return;
      setState(() {
        _updateBusy = false;
        _updateError = error.message;
        _updateProgress = null;
      });
    }
  }

  Future<void> _maybeAutoUpdateApp() async {
    if (!_autoAppUpdate || runningUnderFlutterTest() || _updateBusy) {
      return;
    }
    final kind = _updater?.kind ?? detectInstallKind();
    if (!_appUpdatePrefs.dueForCheck(DateTime.now().toUtc())) {
      return;
    }
    await _checkAppUpdate(quiet: true);
    if (!mounted) return;
    final check = _updateCheck;
    if (check == null) return;
    if (!canAutoApplyUpdate(
      result: check,
      kind: kind,
      autoInstall: _autoAppUpdate,
    )) {
      return;
    }
    await _installAppUpdate(auto: true);
  }

  void _setAutoAppUpdate(bool enabled) {
    setState(() => _autoAppUpdate = enabled);
    _persistAppUpdatePrefs();
    if (!enabled) return;
    unawaited(() async {
      await _checkAppUpdate();
      if (!mounted) return;
      final check = _updateCheck;
      final kind = _updater?.kind ?? detectInstallKind();
      if (check == null ||
          !canAutoApplyUpdate(
            result: check,
            kind: kind,
            autoInstall: true,
          )) {
        return;
      }
      await _installAppUpdate(auto: true);
    }());
  }

  void _setAutoUpdate(bool enabled) {
    setState(() => _autoUpdate = enabled);
    _scheduleProfileUpdate();
  }

  void _scheduleProfileUpdate() {
    _profileUpdater?.cancel();
    _profileUpdater = null;
    if (!_autoUpdate || _profiles.isEmpty) return;
    var seconds = 86400;
    var hasUpdatable = false;
    for (final profile in _profiles) {
      if (!profile.hasSourceUrl) {
        continue;
      }
      hasUpdatable = true;
      final interval = profile.updateIntervalSeconds > 0
          ? profile.updateIntervalSeconds
          : 86400;
      if (interval < seconds) {
        seconds = interval;
      }
    }
    if (!hasUpdatable) {
      return;
    }
    if (_runtime?.engine == 'olcrtc' && seconds > 3600) {
      seconds = 3600;
    }
    _profileUpdater = Timer(Duration(seconds: seconds), () async {
      if (!mounted) return;
      final olcrtcActive = _runtime?.engine == 'olcrtc';
      if (_autoUpdate &&
          (olcrtcActive ||
              _status == ConnectionStatus.disconnected ||
              _status == ConnectionStatus.error) &&
          !_isImportingProfile) {
        await _refreshAllProfiles(silent: true);
      }
      if (mounted) _scheduleProfileUpdate();
    });
  }

  void _applyRuntime(RuntimeSnapshot runtime) {
    _runtime = runtime;
    _status = switch (runtime.status) {
      'connected' => ConnectionStatus.connected,
      'starting' || 'stopping' => ConnectionStatus.connecting,
      'failed' => ConnectionStatus.error,
      _ => ConnectionStatus.disconnected,
    };
    if (runtime.status != 'connected') {
      _trafficSamples = const [];
    } else if (runtime.trafficAvailable && runtime.lastTrafficUpdate != null) {
      final sample = TrafficSample(
        at: runtime.lastTrafficUpdate!.toLocal(),
        upload: runtime.uploadRateBytes,
        download: runtime.downloadRateBytes,
      );
      final samples = [..._trafficSamples, sample];
      _trafficSamples = samples.length > 90
          ? samples.sublist(samples.length - 90)
          : samples;
    }
    setState(() {});
    unawaited(_syncTray());
  }

  int _selectedPage = 0;

  bool get _isConnected => _status == ConnectionStatus.connected;

  Future<void> _toggleConnection() async {
    if (_controlPlaneReachable == false) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Нет связи с сервисом Naga. Состояние VPN неизвестно.'),
        ),
      );
      return;
    }
    if (_status == ConnectionStatus.connecting || _runtimeRequestInFlight) {
      return;
    }

    if (!_hasProfile || _profileId == null) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Сначала импортируй профиль.')),
      );
      return;
    }
    if (_isConnected) {
      setState(() => _status = ConnectionStatus.connecting);
      _runtimeRequestInFlight = true;
      try {
        final runtime = await _controlPlane.stopRuntime();
        if (nagaRunsOnAndroid) await AndroidVpn.instance.stop();
        if (mounted) _applyRuntime(runtime);
      } on ControlPlaneException catch (error) {
        if (nagaRunsOnAndroid) await AndroidVpn.instance.stop();
        if (!mounted) return;
        setState(() => _status = ConnectionStatus.error);
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(error.message)));
      } finally {
        _runtimeRequestInFlight = false;
      }
      return;
    }

    if (_selectedNodeIsOlc(_profileNodes?.nodes ?? const [])) {
      for (final profile in _profiles) {
        if (profile.id != _profileId || !profile.hasSourceUrl) continue;
        final imported = profile.importedAt;
        if (imported != null &&
            DateTime.now().difference(imported) > const Duration(hours: 1)) {
          await _refreshProfile(profile, silent: true);
        }
      }
    }

    setState(() => _status = ConnectionStatus.connecting);
    _runtimeRequestInFlight = true;
    try {
      if (!await _prepareAndroidTunnel()) {
        if (!mounted) return;
        setState(() => _status = ConnectionStatus.error);
        if (!_connectingOlc || _olcrtcReady) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(content: Text('Не удалось создать VPN-интерфейс.')),
          );
        }
        return;
      }
      final runtime = await _controlPlane.startRuntime(_profileId!);
      if (!mounted) return;
      setState(() {
        _status = switch (runtime.status) {
          'connected' => ConnectionStatus.connected,
          'starting' => ConnectionStatus.connecting,
          _ => ConnectionStatus.error,
        };
        _runtime = runtime;
      });
      if (runtime.status != 'connected') {
        if (nagaRunsOnAndroid) await AndroidVpn.instance.stop();
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              _friendlyRuntimeError(runtime.error) ??
                  'Не удалось подключиться. Попробуй ещё раз.',
            ),
          ),
        );
      }
    } on ControlPlaneException catch (error) {
      if (nagaRunsOnAndroid) await AndroidVpn.instance.stop();
      if (!mounted) return;
      setState(() => _status = ConnectionStatus.error);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            _friendlyRuntimeError(error.message) ?? error.message,
          ),
        ),
      );
    } finally {
      _runtimeRequestInFlight = false;
    }
  }

  bool get _connectingOlc =>
      _selectedNodeIsOlc(_profileNodes?.nodes ?? const []) ||
      _runtime?.engine == 'olcrtc' ||
      _profilePreview?.engine == 'olcrtc';

  List<String> get _androidSplitPackages => _effectiveRoutingPolicy.apps
      .where((app) => app.enabled && app.packageOrProcessId.trim().isNotEmpty)
      .map((app) => app.packageOrProcessId.trim())
      .toList(growable: false);

  Future<bool> _prepareAndroidTunnel() async {
    if (!nagaRunsOnAndroid) return true;
    if (_connectingOlc && !_olcrtcReady) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('olcRTC недоступен на Android.')),
        );
      }
      return false;
    }
    final prepared = await AndroidVpn.instance.prepare();
    if (!prepared) return false;
    return AndroidVpn.instance.start(
      engine: _connectingOlc ? 'olcrtc' : 'singbox',
      routing: _effectiveRoutingPolicy.mode,
      packages: _androidSplitPackages,
    );
  }

  Future<void> _rebuildAndroidVpnIfConnected() async {
    if (!nagaRunsOnAndroid || !_isConnected || _profileId == null) return;
    try {
      await _controlPlane.stopRuntime();
      await AndroidVpn.instance.stop();
      if (!await _prepareAndroidTunnel()) {
        if (mounted) setState(() => _status = ConnectionStatus.error);
        return;
      }
      final runtime = await _controlPlane.startRuntime(_profileId!);
      if (mounted) _applyRuntime(runtime);
      if (runtime.status != 'connected' && nagaRunsOnAndroid) {
        await AndroidVpn.instance.stop();
      }
    } on ControlPlaneException catch (error) {
      await AndroidVpn.instance.stop();
      if (mounted) {
        setState(() => _status = ConnectionStatus.error);
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(error.message)));
      }
    }
  }

  void _recordImportDiagnostic(String event, String message) {
    unawaited(_controlPlane.recordClientEvent(event: event, message: message));
  }

  Future<void> _importProfile({
    bool pasteFromClipboard = false,
    String? prefilledUrl,
    bool skipUrlDialog = false,
  }) async {
    if (_isImportingProfile) return;
    var initialUrl = prefilledUrl ?? _profileUrl;
    if (pasteFromClipboard) {
      try {
        final data = await Clipboard.getData('text/plain');
        initialUrl = data?.text?.trim() ?? '';
      } on PlatformException {
        if (!mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Буфер обмена недоступен. Вставьте ссылку вручную.'),
          ),
        );
      }
    }
    if (!mounted) return;
    NagaImportRequest? source;
    if (skipUrlDialog) {
      source = NagaImportRequest.url(initialUrl);
    } else {
      final compact =
          MediaQuery.sizeOf(context).width < NagaBreakpoints.compact;
      source = compact
          ? await showModalBottomSheet<NagaImportRequest>(
              context: context,
              isScrollControlled: true,
              backgroundColor: Colors.transparent,
              builder: (context) => Padding(
                padding: EdgeInsets.only(
                  bottom: MediaQuery.viewInsetsOf(context).bottom,
                ),
                child: _ImportProfilePanel(
                  initialUrl: initialUrl,
                  onDiagnostic: _recordImportDiagnostic,
                ),
              ),
            )
          : await showDialog<NagaImportRequest>(
              context: context,
              builder: (context) => Dialog(
                backgroundColor: _surfaceRaised,
                child: SizedBox(
                  width: 520,
                  child: _ImportProfilePanel(
                    initialUrl: initialUrl,
                    onDiagnostic: _recordImportDiagnostic,
                  ),
                ),
              ),
            );
    }
    if (!mounted || source == null) return;
    var config = source.config?.trim();
    var url = source.url?.trim() ?? '';
    if ((config == null || config.isEmpty) &&
        url.toLowerCase().startsWith('olcrtc://')) {
      config = url;
      url = '';
    }
    if (config == null || config.isEmpty) {
      final resolved = subscriptionFetchURL(url);
      if (resolved == null) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text(
              'Нужна HTTPS-ссылка профиля, nagavpn://install-config или файл конфига.',
            ),
          ),
        );
        return;
      }
      url = resolved;
    }
    setState(() => _isImportingProfile = true);
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(
          config != null && config.isNotEmpty
              ? 'Проверяем файл…'
              : 'Проверяем ссылку…',
        ),
      ),
    );
    try {
      SubscriptionDownload? downloaded;
      var supplied = config ?? '';
      var useServerFetch = false;
      final throughTunnel = supplied.isEmpty && _isConnected;
      if (throughTunnel) {
        useServerFetch = true;
      } else if (supplied.isEmpty) {
        try {
          downloaded = await downloadSubscription(url);
          supplied = downloaded.body;
        } on SubscriptionFetchException catch (error) {
          unawaited(
            _controlPlane.recordClientEvent(
              event: 'subscription_download_fallback',
              level: 'warn',
              message:
                  'Загрузка подписки в приложении не удалась (${error.message}); '
                  'пробуем через сервис.',
            ),
          );
          useServerFetch = true;
        }
      }
      final preview = useServerFetch
          ? await _controlPlane.preview(url)
          : await _controlPlane.previewConfig(
              config: supplied,
              name: source.fileName,
              downloaded: downloaded,
            );
      if (!mounted) return;
      final confirmed = await _confirmProfilePreview(preview);
      if (confirmed != true || !mounted) return;
      if (!throughTunnel) {
        await _controlPlane.stopRuntime();
        if (nagaRunsOnAndroid) await AndroidVpn.instance.stop();
      }
      final saved = useServerFetch
          ? await _controlPlane.importProfile(url)
          : await _controlPlane.importConfig(
              config: supplied,
              name: source.fileName,
              downloaded: downloaded,
            );
      if (!mounted) return;
      setState(() {
        _profileUrl = useServerFetch ? url : (downloaded?.url ?? '');
        _profileId = saved.profileId;
        _profileName = saved.profileTitle;
        _profileProvider = saved.providerName;
        _profilePreview = saved;
        _runtime = null;
        _status = ConnectionStatus.disconnected;
      });
      final profileId = saved.profileId;
      if (profileId == null || profileId.isEmpty) {
        throw const ControlPlaneException(
          'Control-plane не вернул идентификатор профиля.',
        );
      }
      await _loadNodes(profileId);
      _scheduleProfileUpdate();
      await _reloadProfiles();
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Профиль сохранён: ${saved.engine}.')),
      );
    } on ControlPlaneException catch (error) {
      if (!mounted) return;
      unawaited(
        _controlPlane.recordClientEvent(
          event: 'import_failed',
          message: 'Импорт профиля прерван: ${error.message}',
        ),
      );
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(_friendlyImportError(error.message))),
      );
    } on SubscriptionFetchException catch (error) {
      if (!mounted) return;
      unawaited(
        _controlPlane.recordClientEvent(
          event: 'subscription_download_failed',
          message:
              'Загрузка подписки в приложении не удалась: ${error.message}',
        ),
      );
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(_friendlyImportError(error.message))),
      );
    } finally {
      if (mounted) setState(() => _isImportingProfile = false);
    }
  }

  Future<bool?> _confirmProfilePreview(ProfilePreview preview) {
    final limit = preview.unlimited
        ? 'Безлимитный трафик'
        : _formatBytes(preview.totalBytes);
    final expiry = preview.expireUtc == null
        ? 'Без срока действия'
        : _formatDate(preview.expireUtc!);
    return showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        scrollable: true,
        title: const Text('Профиль найден'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              preview.profileTitle,
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 14),
            Text('Провайдер: ${preview.providerName}'),
            Text('Движок: ${preview.engine}'),
            Text('Лимит: $limit'),
            Text('Действует до: $expiry'),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Отмена'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Сохранить профиль'),
          ),
        ],
      ),
    );
  }

  Future<void> _refreshAllProfiles({bool silent = false}) async {
    if (_isImportingProfile) return;
    setState(() => _isImportingProfile = true);
    try {
      for (final profile in List<StoredProfileSummary>.from(_profiles)) {
        if (!mounted) return;
        if (!profile.hasSourceUrl) {
          continue;
        }
        await _refreshProfileUnlocked(profile, silent: silent);
      }
    } finally {
      if (mounted) setState(() => _isImportingProfile = false);
    }
  }

  Future<void> _refreshProfile(
    StoredProfileSummary profile, {
    bool silent = false,
  }) async {
    if (_isImportingProfile) return;
    setState(() => _isImportingProfile = true);
    try {
      await _refreshProfileUnlocked(profile, silent: silent);
    } finally {
      if (mounted) setState(() => _isImportingProfile = false);
    }
  }

  Future<void> _refreshProfileUnlocked(
    StoredProfileSummary profile, {
    required bool silent,
  }) async {
    if (!silent) {
      ScaffoldMessenger.of(context)
          .showSnackBar(const SnackBar(content: Text('Обновляем профиль…')));
    }
    try {
      final ProfilePreview preview;
      if (profile.hasSourceUrl) {
        final share = await _controlPlane.shareProfile(profile.id);
        final downloaded = await downloadSubscription(share.url);
        preview = await _controlPlane.refreshWithConfig(profile.id, downloaded);
      } else {
        preview = await _controlPlane.refreshProfile(profile.id);
      }
      if (!mounted) return;
      setState(() {
        if (profile.id == _profileId) {
          _profileName = preview.profileTitle;
          _profileProvider = preview.providerName;
          _profilePreview = preview;
          if (!silent) {
            _runtime = null;
            _status = ConnectionStatus.disconnected;
          }
        }
      });
      if (profile.id == _profileId) {
        await _loadNodes(profile.id);
      } else {
        try {
          final unified = await _controlPlane.listUnifiedNodes();
          if (mounted) {
            setState(
              () =>
                  _unifiedNodes = nagaMergeProbedNodes(unified, _unifiedNodes),
            );
          }
        } on ControlPlaneException {
          // Keep the current unified list if the aggregate endpoint is down.
        }
      }
      _scheduleProfileUpdate();
      await _reloadProfiles();
      if (mounted && !silent) {
        ScaffoldMessenger.of(context)
            .showSnackBar(const SnackBar(content: Text('Профиль обновлён.')));
      }
    } on ControlPlaneException catch (error) {
      if (!mounted || silent) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(_friendlyImportError(error.message))),
      );
    } on SubscriptionFetchException catch (error) {
      if (!mounted || silent) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(_friendlyImportError(error.message))),
      );
    }
  }

  Future<void> _shareProfile(StoredProfileSummary profile) async {
    try {
      final share = await _controlPlane.shareProfile(profile.id);
      if (!mounted) return;
      await showDialog<void>(
        context: context,
        builder: (context) => NagaShareProfileDialog(
          profileName: share.name.isEmpty ? profile.name : share.name,
          url: share.url,
        ),
      );
    } on ControlPlaneException catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(error.message)));
    }
  }

  Future<void> _deleteProfile(StoredProfileSummary profile) async {
    if (_isImportingProfile) return;
    final runtimeStatus = _runtime?.status;
    if (runtimeBlocksProfileDelete(runtimeStatus)) {
      ScaffoldMessenger.of(context)
          .showSnackBar(const SnackBar(content: Text('Сначала отключи VPN.')));
      return;
    }
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Удалить профиль?'),
        content: Text(
          'Профиль «${profile.name}» будет удалён с этого устройства. Подписка у VPN-сервиса не изменится.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Отмена'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Удалить'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    setState(() => _isImportingProfile = true);
    try {
      await _controlPlane.deleteProfile(profile.id);
      if (!mounted) return;
      final remaining = _profiles
          .where((item) => item.id != profile.id)
          .toList(growable: false);
      setState(() {
        _profiles = remaining;
        if (remaining.isEmpty) {
          _profileUrl = '';
          _profileId = null;
          _profileName = 'Профиль не импортирован';
          _profileProvider = '';
          _profilePreview = null;
          _profileNodes = null;
          _unifiedNodes = const [];
        } else if (_profileId == profile.id || _profileId == null) {
          _adoptProfile(remaining.last);
        }
        _runtime = null;
        _status = ConnectionStatus.disconnected;
      });
      if (remaining.isNotEmpty && _profileId != null) {
        await _loadNodes(_profileId!);
      }
      _profileUpdater?.cancel();
      _profileUpdater = null;
      _scheduleProfileUpdate();
      ScaffoldMessenger.of(context)
          .showSnackBar(const SnackBar(content: Text('Профиль удалён.')));
    } on ControlPlaneException catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(_friendlyImportError(error.message))),
      );
    } finally {
      if (mounted) setState(() => _isImportingProfile = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: LayoutBuilder(
          builder: (context, constraints) {
            final compact = constraints.maxWidth < NagaBreakpoints.compact;
            final expanded =
                constraints.maxWidth >= NagaBreakpoints.medium &&
                MediaQuery.textScalerOf(context).scale(14) <= 20;
            final content = Expanded(
              child: SingleChildScrollView(
                key: ValueKey('section-scroll-$_selectedPage'),
                physics: const AlwaysScrollableScrollPhysics(
                  parent: ClampingScrollPhysics(),
                ),
                padding: EdgeInsets.symmetric(
                  horizontal: compact
                      ? NagaSpacing.md
                      : expanded
                      ? NagaSpacing.xl
                      : NagaSpacing.lg,
                  vertical: compact ? 24 : 36,
                ),
                child: Center(
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 1080),
                    child: AnimatedSwitcher(
                      duration: MediaQuery.disableAnimationsOf(context)
                          ? Duration.zero
                          : NagaMotion.normal,
                      switchInCurve: NagaMotion.standard,
                      switchOutCurve: Curves.easeInCubic,
                      layoutBuilder: (currentChild, previousChildren) {
                        return Stack(
                          clipBehavior: Clip.none,
                          alignment: Alignment.topCenter,
                          children: [
                            ...previousChildren,
                            if (currentChild != null) currentChild,
                          ],
                        );
                      },
                      child: KeyedSubtree(
                        key: ValueKey(_selectedPage),
                        child: switch (_selectedPage) {
                          0 => NagaHomeView(
                            status: _status,
                            profileName: _profileName,
                            profileProvider: _profileProvider,
                            selectedNode: _selectedNodeName,
                            runtime: _runtime,
                            trafficSamples: _trafficSamples,
                            preview: _profilePreview,
                            hasProfile: _hasProfile,
                            isImporting: _isImportingProfile,
                            onImport: _importProfile,
                            onPaste: () =>
                                _importProfile(pasteFromClipboard: true),
                            onOpenServerPicker: _hasProfile
                                ? _showServerPicker
                                : _importProfile,
                            onOpenDiagnostics: () =>
                                setState(() => _selectedPage = 6),
                            onToggle: _toggleConnection,
                            trafficMode: _runtime?.engine == 'olcrtc'
                                ? 'olcrtc_tun'
                                : _effectiveConnectionPolicy.trafficMode,
                            onSelectTrafficMode: _selectTrafficMode,
                            trafficModeError: _trafficModeError,
                            controlPlaneReachable: _controlPlaneReachable,
                            nodes: _unifiedNodes,
                            networkClass:
                                _effectiveConnectionPolicy.networkClass,
                          ),
                          1 => _ProfilesPage(
                            profileName: _profileName,
                            profiles: _profiles,
                            hasProfile: _hasProfile,
                            onImport: _importProfile,
                            onRefresh: _refreshProfile,
                            onShare: _shareProfile,
                            onDelete: _deleteProfile,
                            preview: _profilePreview,
                            autoUpdate: _autoUpdate,
                            onAutoUpdateChanged: _setAutoUpdate,
                          ),
                          2 => _StatisticsPage(
                            runtime: _controlPlaneReachable == false
                                ? null
                                : _runtime,
                            trafficSamples: _trafficSamples,
                          ),
                          3 => _SettingsPage(
                            autoUpdate: _autoUpdate,
                            onAutoUpdateChanged: _setAutoUpdate,
                            autoAppUpdate: _autoAppUpdate,
                            onAutoAppUpdateChanged: _setAutoAppUpdate,
                            onOpenDiagnostics: () =>
                                setState(() => _selectedPage = 6),
                            onOpenHelp: () => setState(() => _selectedPage = 5),
                            onOpenSplitTunnel: () {
                              unawaited(_refreshDiscoveredApps());
                              setState(() => _selectedPage = 8);
                            },
                            onOpenServerChoice: () =>
                                setState(() => _selectedPage = 9),
                            trafficMode: _effectiveConnectionPolicy.trafficMode,
                            onSelectTrafficMode: _selectTrafficMode,
                            trafficModeError: _trafficModeError,
                            uiVersion: _uiVersion,
                            controlVersion: _controlVersion,
                            updateBusy: _updateBusy,
                            updateProgress: _updateProgress,
                            updateMessage: _updateMessage,
                            updateError: _updateError,
                            updateCheck: _updateCheck,
                            onCheckUpdate: _checkAppUpdate,
                            onInstallUpdate: _installAppUpdate,
                          ),
                          6 => _DiagnosticsPage(controlPlane: _controlPlane),
                          7 => _NodesPage(
                            nodes: _unifiedNodes,
                            connected:
                                _controlPlaneReachable != false && _isConnected,
                            onSelect: _selectUnifiedNode,
                            onSelectAuto: _hasProfile ? _selectAutoNode : null,
                            onProbe: _hasProfile ? _probeUnifiedNodes : null,
                            networkClass:
                                _effectiveConnectionPolicy.networkClass,
                          ),
                          8 => _RoutingPage(
                            policy: _effectiveRoutingPolicy,
                            apps: _apps,
                            discovered: _discoveredApps,
                            onSaveRouting: _saveRoutingPolicy,
                            onSaveApp: _saveAppRoute,
                            onDeleteApp: _deleteAppRoute,
                            onRefreshDiscovered: _refreshDiscoveredApps,
                          ),
                          9 => _ServerChoicePage(
                            connectionPolicy: _effectiveConnectionPolicy,
                            onSaveConnection: _saveConnectionPolicy,
                          ),
                          _ => const _HelpPage(),
                        },
                      ),
                    ),
                  ),
                ),
              ),
            );

            final Widget shell;
            if (compact) {
              shell = Column(
                children: [
                  content,
                  _BottomNavigationBar(
                    selectedIndex: _selectedPage,
                    onSelect: (index) => setState(() => _selectedPage = index),
                  ),
                ],
              );
            } else {
              shell = Row(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  NagaNavigationRail(
                    selectedIndex: switch (_selectedPage) {
                      8 || 9 => 3,
                      _ => _selectedPage,
                    },
                    onSelect: (index) => setState(() => _selectedPage = index),
                    compact: !expanded,
                    activeProfile: _railProfileLabel,
                  ),
                  content,
                ],
              );
            }

            if (nagaUsesWindowsChrome) {
              return Column(
                children: [
                  NagaTitleBar(
                    onMinimize: () =>
                        unawaited(_invokeWindowChrome('minimize')),
                    onMaximize: () =>
                        unawaited(_invokeWindowChrome('maximize')),
                    onClose: () => unawaited(_invokeWindowChrome('close')),
                    onDrag: () => unawaited(_invokeWindowChrome('startDrag')),
                  ),
                  Expanded(child: shell),
                ],
              );
            }
            return shell;
          },
        ),
      ),
    );
  }

  String get _selectedNodeName {
    return _findSelectedTag(_profileNodes?.nodes ?? const <ProfileNode>[]) ??
        '';
  }

  String? get _railProfileLabel {
    if (_profiles.isEmpty) {
      return _hasProfile ? _profileName : null;
    }
    if (_profiles.length == 1) {
      return _profiles.first.name;
    }
    return 'Naga Network';
  }
}

bool _selectedNodeIsOlc(List<ProfileNode> nodes) {
  for (final node in nodes) {
    if (_selectedNodeIsOlc(node.children)) return true;
    if (node.selected && node.protocol == 'olcRTC') return true;
  }
  return false;
}

String? _findSelectedTag(List<ProfileNode> nodes) {
  for (final node in nodes) {
    final nested = _findSelectedTag(node.children);
    if (nested != null) return nested;
    if (node.selected) return node.tag;
  }
  return null;
}

String? _findNodeTag(List<ProfileNode> nodes, String query) {
  for (final node in nodes) {
    if (node.tag.toLowerCase() == query) return node.tag;
    final nested = _findNodeTag(node.children, query);
    if (nested != null) return nested;
  }
  return null;
}

class NagaImportRequest {
  const NagaImportRequest.url(this.url) : config = null, fileName = null;

  const NagaImportRequest.file({required this.config, this.fileName})
    : url = null;

  final String? url;
  final String? config;
  final String? fileName;
}

class _ImportProfilePanel extends StatefulWidget {
  const _ImportProfilePanel({required this.initialUrl, this.onDiagnostic});
  final String initialUrl;
  final void Function(String event, String message)? onDiagnostic;

  @override
  State<_ImportProfilePanel> createState() => _ImportProfilePanelState();
}

class _ImportProfilePanelState extends State<_ImportProfilePanel> {
  late final controller = TextEditingController(text: widget.initialUrl);

  @override
  void dispose() {
    controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Material(
      color: _surfaceRaised,
      borderRadius: const BorderRadius.all(Radius.circular(18)),
      child: SafeArea(
        top: false,
        child: SingleChildScrollView(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(24, 20, 24, 24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Center(
                  child: Container(
                    width: 40,
                    height: 4,
                    decoration: BoxDecoration(
                      color: _lineStrong,
                      borderRadius: BorderRadius.circular(4),
                    ),
                  ),
                ),
                const SizedBox(height: 20),
                const Text(
                  'Добавить VPN',
                  style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 6),
                const Text(
                  'Вставьте HTTPS-ссылку профиля, deep-link nagavpn://install-config или выберите файл конфига.',
                  style: TextStyle(color: _muted, height: 1.4),
                ),
                const SizedBox(height: 18),
                TextField(
                  controller: controller,
                  autofocus: true,
                  keyboardType: TextInputType.url,
                  decoration: const InputDecoration(
                    labelText: 'Ссылка на профиль',
                    hintText: 'https://… или nagavpn://install-config?url=…',
                    border: OutlineInputBorder(),
                  ),
                ),
                const SizedBox(height: 8),
                TextButton.icon(
                  onPressed: () async {
                    ClipboardData? data;
                    try {
                      data = await Clipboard.getData('text/plain');
                    } on PlatformException {
                      if (!context.mounted) return;
                      ScaffoldMessenger.maybeOf(context)?.showSnackBar(
                        const SnackBar(
                          content: Text(
                            'Буфер обмена недоступен. Вставьте ссылку вручную.',
                          ),
                        ),
                      );
                      return;
                    }
                    if (!context.mounted) return;
                    final value = data?.text?.trim();
                    if (value == null || value.isEmpty) return;
                    controller.value = TextEditingValue(
                      text: value,
                      selection: TextSelection.collapsed(offset: value.length),
                    );
                  },
                  icon: const Icon(Icons.content_paste_rounded, size: 17),
                  label: const Text('Вставить из буфера'),
                ),
                TextButton.icon(
                  onPressed: () async {
                    try {
                      final picked = await pickNagaConfigFile();
                      if (!context.mounted) return;
                      if (picked == null) {
                        widget.onDiagnostic?.call(
                          'file_dialog_no_selection',
                          'Окно выбора файла закрылось без файла.',
                        );
                        ScaffoldMessenger.maybeOf(context)?.showSnackBar(
                          const SnackBar(
                            content: Text(
                              'Файл не выбран. Окно должно открыться поверх Naga.',
                            ),
                          ),
                        );
                        return;
                      }
                      Navigator.of(context).pop(
                        NagaImportRequest.file(
                          config: picked.contents,
                          fileName: picked.fileName,
                        ),
                      );
                    } on FormatException {
                      if (!context.mounted) return;
                      widget.onDiagnostic?.call(
                        'file_read_failed',
                        'Файл конфига не прочитан: слишком большой или неизвестная кодировка.',
                      );
                      ScaffoldMessenger.maybeOf(context)?.showSnackBar(
                        const SnackBar(
                          content: Text(
                            'Файл слишком большой или в неизвестной кодировке.',
                          ),
                        ),
                      );
                    } catch (_) {
                      if (!context.mounted) return;
                      widget.onDiagnostic?.call(
                        'file_dialog_failed',
                        'Не удалось открыть окно выбора файла конфига.',
                      );
                      ScaffoldMessenger.maybeOf(context)?.showSnackBar(
                        const SnackBar(
                          content: Text('Не удалось открыть файл конфига.'),
                        ),
                      );
                    }
                  },
                  icon: const Icon(Icons.folder_open_rounded, size: 17),
                  label: const Text('Выбрать файл'),
                ),
                const SizedBox(height: 10),
                SizedBox(
                  width: double.infinity,
                  child: FilledButton(
                    onPressed: () {
                      final url = controller.text.trim();
                      if (url.isEmpty) {
                        return;
                      }
                      Navigator.of(context).pop(NagaImportRequest.url(url));
                    },
                    child: const Text('Продолжить'),
                  ),
                ),
                Center(
                  child: TextButton(
                    onPressed: () => Navigator.of(context).pop(),
                    child: const Text('Отмена'),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class NagaNodeGroup {
  const NagaNodeGroup({required this.country, required this.nodes});

  final String country;
  final List<UnifiedNode> nodes;
}

List<NagaNodeGroup> groupUnifiedNodes(List<UnifiedNode> nodes) {
  final grouped = <String, List<UnifiedNode>>{};
  for (final node in nodes) {
    final country = node.country.trim().isEmpty
        ? 'Другие'
        : node.country.trim();
    grouped.putIfAbsent(country, () => []).add(node);
  }
  for (final list in grouped.values) {
    list.sort((left, right) {
      final protocol = left.protocol.compareTo(right.protocol);
      if (protocol != 0) {
        return protocol;
      }
      return left.tag.compareTo(right.tag);
    });
  }
  final countries = grouped.keys.toList()..sort();
  return [
    for (final country in countries)
      NagaNodeGroup(country: country, nodes: grouped[country]!),
  ];
}

const int kAutoCandidateLimit = 3;

int nagaProtocolPriority(String protocol, {String network = 'unknown'}) {
  final normalized = protocol.trim().toLowerCase();
  if (normalized == 'tuic') {
    return 2;
  }
  if (network == 'cellular' && normalized == 'vless') {
    return 0;
  }
  return 1;
}

List<UnifiedNode> recommendedUnifiedNodes(
  List<UnifiedNode> nodes, {
  int limit = kAutoCandidateLimit,
  String network = 'unknown',
}) {
  final ranked = [...nodes]
    ..sort((left, right) {
      final priority = nagaProtocolPriority(
        left.protocol,
        network: network,
      ).compareTo(nagaProtocolPriority(right.protocol, network: network));
      if (priority != 0) {
        return priority;
      }
      final leftLatency = left.latencyMs ?? 0;
      final rightLatency = right.latencyMs ?? 0;
      if (leftLatency <= 0 && rightLatency <= 0) {
        return left.tag.compareTo(right.tag);
      }
      if (leftLatency <= 0) {
        return 1;
      }
      if (rightLatency <= 0) {
        return -1;
      }
      final latency = leftLatency.compareTo(rightLatency);
      if (latency != 0) {
        return latency;
      }
      return left.tag.compareTo(right.tag);
    });
  if (ranked.length <= limit) {
    return ranked;
  }
  return ranked.take(limit).toList(growable: false);
}

String? nagaVisibleFailover(String? message, String displayedNode) {
  final text = message?.trim() ?? '';
  final node = displayedNode.trim();
  if (text.isEmpty || node.isEmpty || node.toLowerCase() == 'auto') {
    return null;
  }
  if (!text.contains(node)) {
    return null;
  }
  return text;
}

String nagaNodePingLabel(UnifiedNode node) {
  final latency = node.latencyMs;
  if (latency != null && latency > 0) {
    return '$latency мс';
  }
  if (node.probeStatus == 'failed') {
    return 'нет ответа';
  }
  return '—';
}

final _nagaFlagPrefix = RegExp(
  r'^[\u{1F1E6}-\u{1F1FF}]{2}',
  unicode: true,
);

String nagaNodeFlag(String tag) {
  final match = _nagaFlagPrefix.firstMatch(tag.trim());
  return match?.group(0) ?? '';
}

String nagaNodeTitle(UnifiedNode node) {
  var title = node.tag.trim().replaceFirst(
    RegExp(r'^[\u{1F1E6}-\u{1F1FF}]{2}\s*', unicode: true),
    '',
  );
  final protocol = node.protocol.trim();
  if (protocol.isNotEmpty) {
    final lower = title.toLowerCase();
    final proto = protocol.toLowerCase();
    if (lower == proto) {
      return protocol;
    }
    if (lower.endsWith(proto)) {
      title = title.substring(0, title.length - protocol.length).trim();
      title = title.replaceFirst(RegExp(r'[\s·\-–/]+$'), '');
    }
  }
  if (title.isEmpty) {
    return protocol.isNotEmpty ? protocol : 'Без названия';
  }
  return title;
}

String nagaNodeProtocolLabel(UnifiedNode node) {
  final protocol = node.protocol.trim();
  if (protocol.isEmpty) {
    return '';
  }
  if (nagaNodeTitle(node).toLowerCase() == protocol.toLowerCase()) {
    return '';
  }
  return protocol;
}

int? nagaHomeLatencyMs({
  int? runtimeLatencyMs,
  required String selectedNode,
  String? activeNode,
  required List<UnifiedNode> nodes,
  String networkClass = 'unknown',
}) {
  if (runtimeLatencyMs != null && runtimeLatencyMs > 0) {
    return runtimeLatencyMs;
  }
  final names = <String>{
    if (selectedNode.isNotEmpty) selectedNode,
    if (activeNode != null && activeNode.isNotEmpty) activeNode,
  };
  for (final node in nodes) {
    if (names.contains(node.tag) || names.contains(node.runtimeTag)) {
      final latency = node.latencyMs;
      if (latency != null && latency > 0) {
        return latency;
      }
    }
  }
  final automatic =
      selectedNode.isEmpty || selectedNode.toLowerCase() == 'auto';
  if (!automatic) {
    return null;
  }
  for (final node in recommendedUnifiedNodes(nodes, network: networkClass)) {
    final latency = node.latencyMs;
    if (latency != null && latency > 0) {
      return latency;
    }
  }
  return null;
}

UnifiedNode nagaPreferProbedNode(UnifiedNode incoming, UnifiedNode? probed) {
  if (probed == null) {
    return incoming;
  }
  if (incoming.probeStatus == 'healthy' || incoming.probeStatus == 'failed') {
    return incoming;
  }
  if (probed.probeStatus == 'healthy' || probed.probeStatus == 'failed') {
    return UnifiedNode(
      id: incoming.id,
      profileId: incoming.profileId,
      tag: incoming.tag,
      runtimeTag: incoming.runtimeTag,
      country: incoming.country,
      protocol: incoming.protocol,
      latencyMs: probed.latencyMs,
      probeStatus: probed.probeStatus,
    );
  }
  return incoming;
}

List<UnifiedNode> nagaMergeProbedNodes(
  List<UnifiedNode> incoming,
  List<UnifiedNode> probed,
) {
  if (probed.isEmpty) {
    return incoming;
  }
  final byId = {for (final node in probed) node.id: node};
  return [
    for (final node in incoming) nagaPreferProbedNode(node, byId[node.id]),
  ];
}

class NagaNodesView extends StatefulWidget {
  const NagaNodesView({
    super.key,
    required this.nodes,
    required this.connected,
    required this.onSelect,
    this.onSelectAuto,
    this.onProbe,
    this.showClose = false,
    this.networkClass = 'unknown',
  });

  final List<UnifiedNode> nodes;
  final bool connected;
  final ValueChanged<UnifiedNode> onSelect;
  final VoidCallback? onSelectAuto;
  final Future<List<UnifiedNode>> Function()? onProbe;
  final bool showClose;
  final String networkClass;

  @override
  State<NagaNodesView> createState() => _NagaNodesViewState();
}

class _NagaNodesViewState extends State<NagaNodesView> {
  String _query = '';
  bool _probing = false;
  List<UnifiedNode>? _probed;

  List<UnifiedNode> get _nodes => _probed ?? widget.nodes;

  @override
  void didUpdateWidget(covariant NagaNodesView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (_probed == null) {
      return;
    }
    _probed = nagaMergeProbedNodes(widget.nodes, _probed!);
  }

  Future<void> _probe() async {
    final onProbe = widget.onProbe;
    if (onProbe == null || _probing) {
      return;
    }
    setState(() => _probing = true);
    try {
      final nodes = await onProbe();
      if (mounted) {
        setState(() => _probed = nodes);
      }
    } catch (_) {
      // The caller shows a user-facing error; keep the last known list.
    } finally {
      if (mounted) {
        setState(() => _probing = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final query = _query.trim().toLowerCase();
    final visible = _nodes
        .where((node) {
          if (query.isEmpty) {
            return true;
          }
          return node.tag.toLowerCase().contains(query) ||
              node.country.toLowerCase().contains(query) ||
              node.protocol.toLowerCase().contains(query);
        })
        .toList(growable: false);
    final recommended = query.isEmpty
        ? recommendedUnifiedNodes(visible, network: widget.networkClass)
        : const <UnifiedNode>[];
    final recommendedIds = {for (final node in recommended) node.id};
    final groupedNodes = query.isEmpty
        ? visible
              .where((node) => !recommendedIds.contains(node.id))
              .toList(growable: false)
        : visible;
    final groups = groupUnifiedNodes(groupedNodes);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (widget.showClose)
          Row(
            children: [
              const Expanded(
                child: Text(
                  'Серверы',
                  style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
                ),
              ),
              IconButton(
                onPressed: () => Navigator.of(context).pop(),
                tooltip: 'Закрыть',
                icon: const Icon(Icons.close_rounded),
              ),
            ],
          ),
        if (widget.onSelectAuto != null) ...[
          Card(
            child: ListTile(
              leading: const _SoftIcon(Icons.bolt_rounded, accent: true),
              title: const Text('Автоматический выбор'),
              subtitle: const Text(
                'Проверит 3 наиболее подходящих сервера и включит лучший.',
                style: TextStyle(color: _muted),
              ),
              trailing: const Icon(Icons.chevron_right_rounded),
              onTap: widget.onSelectAuto,
            ),
          ),
          const SizedBox(height: 12),
        ],
        if (widget.nodes.isNotEmpty) ...[
          TextField(
            onChanged: (value) => setState(() => _query = value),
            decoration: const InputDecoration(
              prefixIcon: Icon(Icons.search_rounded),
              hintText: 'Страна, имя или протокол',
              border: OutlineInputBorder(),
            ),
          ),
          if (widget.onProbe != null) ...[
            const SizedBox(height: 12),
            Align(
              alignment: Alignment.centerRight,
              child: OutlinedButton.icon(
                onPressed: _probing ? null : _probe,
                icon: _probing
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.speed_rounded),
                label: Text(_probing ? 'Проверяем…' : 'Проверить'),
              ),
            ),
          ],
          const SizedBox(height: 16),
        ],
        if (widget.nodes.isEmpty)
          const Card(
            child: ListTile(
              leading: Icon(Icons.public_off_rounded, color: _muted),
              title: Text('Серверы пока не загружены'),
              subtitle: Text(
                'Импортируй профиль, чтобы увидеть доступные страны и протоколы.',
                style: TextStyle(color: _muted),
              ),
            ),
          )
        else if (visible.isEmpty)
          const Card(
            child: ListTile(
              leading: Icon(Icons.search_off_rounded, color: _muted),
              title: Text('Ничего не найдено'),
              subtitle: Text(
                'Измени запрос поиска.',
                style: TextStyle(color: _muted),
              ),
            ),
          )
        else ...[
          if (recommended.isNotEmpty)
            _nodeGroupCard('Рекомендуемые', recommended),
          for (final group in groups)
            _nodeGroupCard(group.country, group.nodes),
        ],
      ],
    );
  }

  Widget _nodeGroupCard(String title, List<UnifiedNode> nodes) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Card(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 14, 16, 4),
              child: Text(
                title,
                style: const TextStyle(
                  fontWeight: FontWeight.w700,
                  color: _muted,
                ),
              ),
            ),
            for (var i = 0; i < nodes.length; i++) ...[
              _nodeTile(nodes[i]),
              if (i != nodes.length - 1) const Divider(height: 1, color: _line),
            ],
          ],
        ),
      ),
    );
  }

  Widget _nodeTile(UnifiedNode node) {
    final flag = nagaNodeFlag(node.tag);
    final title = nagaNodeTitle(node);
    final protocol = nagaNodeProtocolLabel(node);
    final ping = nagaNodePingLabel(node);
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 10, 16, 10),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          _nodeLeading(flag),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  title,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontWeight: FontWeight.w600,
                    height: 1.25,
                  ),
                ),
                if (protocol.isNotEmpty) ...[
                  const SizedBox(height: 2),
                  Text(
                    protocol,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      color: _muted,
                      fontSize: 13,
                      height: 1.2,
                    ),
                  ),
                ],
              ],
            ),
          ),
          const SizedBox(width: 12),
          SizedBox(
            width: 56,
            child: _probing
                ? const Center(
                    child: SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    ),
                  )
                : Text(
                    ping,
                    maxLines: 1,
                    textAlign: TextAlign.right,
                    style: const TextStyle(
                      color: _muted,
                      fontSize: 13,
                      fontFeatures: [FontFeature.tabularFigures()],
                    ),
                  ),
          ),
          const SizedBox(width: 12),
          SizedBox(
            width: 148,
            height: 40,
            child: OutlinedButton(
              onPressed: () => widget.onSelect(node),
              style: OutlinedButton.styleFrom(
                padding: const EdgeInsets.symmetric(horizontal: 10),
                visualDensity: VisualDensity.compact,
              ),
              child: Text(
                widget.connected ? 'Выбрать' : 'Основной',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _nodeLeading(String flag) {
    if (flag.isEmpty) {
      return const _SoftIcon(Icons.public_rounded);
    }
    return Container(
      width: 40,
      height: 40,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: _surface,
        borderRadius: BorderRadius.circular(NagaRadius.control),
      ),
      child: Text(flag, style: const TextStyle(fontSize: 20, height: 1)),
    );
  }
}

class NagaNavigationRail extends StatelessWidget {
  const NagaNavigationRail({
    required this.selectedIndex,
    required this.onSelect,
    this.compact = false,
    this.activeProfile,
  });
  final int selectedIndex;
  final ValueChanged<int> onSelect;
  final bool compact;
  final String? activeProfile;

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final scrolling =
          constraints.maxHeight < 740 ||
          MediaQuery.textScalerOf(context).scale(14) > 18;
      final content = Container(
        width: compact ? 72 : 224,
        decoration: const BoxDecoration(
          color: _surfaceLow,
          border: Border(right: BorderSide(color: _line)),
        ),
        padding: EdgeInsets.fromLTRB(
          compact ? 8 : 16,
          32,
          compact ? 8 : 16,
          20,
        ),
        child: Column(
          crossAxisAlignment: compact
              ? CrossAxisAlignment.center
              : CrossAxisAlignment.start,
          children: [
            Padding(
              padding: EdgeInsets.symmetric(horizontal: compact ? 0 : 8),
              child: Row(
                mainAxisAlignment: compact
                    ? MainAxisAlignment.center
                    : MainAxisAlignment.start,
                children: [
                  Image.asset(
                    _brandMarkAsset,
                    width: 32,
                    height: 32,
                    fit: BoxFit.contain,
                    filterQuality: FilterQuality.medium,
                  ),
                  if (!compact) ...[
                    const SizedBox(width: 12),
                    const Text(
                      'NAGA\nNETWORK',
                      style: TextStyle(
                        color: _text,
                        fontSize: 13,
                        height: 1.1,
                        fontWeight: FontWeight.w700,
                        letterSpacing: 1,
                      ),
                    ),
                  ],
                ],
              ),
            ),
            const SizedBox(height: 36),
            for (final (index, icon, label) in [
              (0, Icons.home_outlined, 'Главная'),
              (1, Icons.person_outline_rounded, 'Профили'),
              (7, Icons.public_rounded, 'Серверы'),
              (2, Icons.bar_chart_rounded, 'Статистика'),
              (6, Icons.health_and_safety_outlined, 'Диагностика'),
              (3, Icons.settings_outlined, 'Настройки'),
            ])
              _SidebarItem(
                icon: icon,
                label: label,
                active: selectedIndex == index,
                compact: compact,
                onPressed: () => onSelect(index),
              ),
            if (!scrolling) const Spacer() else const SizedBox(height: 24),
            if (!compact && activeProfile != null) ...[
              Material(
                color: Colors.transparent,
                child: ListTile(
                  contentPadding: const EdgeInsets.symmetric(horizontal: 10),
                  leading: const _SoftIcon(
                    Icons.person_outline_rounded,
                    size: 32,
                  ),
                  title: const Text(
                    'Ваш профиль',
                    style: TextStyle(fontSize: 11, color: _muted),
                  ),
                  subtitle: Text(
                    activeProfile!,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(fontSize: 12, color: _text),
                  ),
                  onTap: () => onSelect(1),
                ),
              ),
              const SizedBox(height: 16),
            ],
            if (!compact)
              Material(
                color: Colors.transparent,
                borderRadius: BorderRadius.circular(NagaRadius.card),
                clipBehavior: Clip.antiAlias,
                child: _NagaBackdrop(
                  imageKey: const Key('naga-sidebar-mascot'),
                  child: InkWell(
                    onTap: () => onSelect(5),
                    child: const Padding(
                      padding: EdgeInsets.all(20),
                      child: SizedBox(
                        width: double.infinity,
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              'Помощь',
                              style: TextStyle(
                                color: _text,
                                fontSize: 13,
                                fontWeight: FontWeight.w500,
                              ),
                            ),
                            SizedBox(height: 8),
                            Text(
                              'Инструкции и ответы',
                              style: TextStyle(
                                color: _muted,
                                fontSize: 11,
                                height: 1.5,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ),
                ),
              ),
            if (compact) ...[
              const SizedBox(height: 8),
              _SidebarItem(
                icon: Icons.help_outline_rounded,
                label: 'Помощь',
                active: selectedIndex == 5,
                compact: true,
                onPressed: () => onSelect(5),
              ),
            ],
          ],
        ),
      );
      return scrolling ? SingleChildScrollView(child: content) : content;
    },
  );
}

class _SidebarItem extends StatelessWidget {
  const _SidebarItem({
    required this.icon,
    required this.label,
    required this.onPressed,
    this.active = false,
    this.compact = false,
  });

  final IconData icon;
  final String label;
  final bool active;
  final bool compact;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    final overlay = WidgetStateProperty.resolveWith<Color?>((states) {
      if (states.contains(WidgetState.pressed)) {
        return _accent.withValues(alpha: 0.16);
      }
      if (states.contains(WidgetState.hovered) && !active) {
        return _surfaceRaised;
      }
      return null;
    });
    if (compact) {
      return Padding(
        padding: const EdgeInsets.only(bottom: 6),
        child: SizedBox(
          width: 56,
          height: 48,
          child: IconButton(
            tooltip: label,
            onPressed: onPressed,
            style: IconButton.styleFrom(
              foregroundColor: active ? _accent : _muted,
              backgroundColor: active ? _accent.withValues(alpha: 0.10) : null,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(NagaRadius.small),
              ),
            ).copyWith(overlayColor: overlay),
            icon: Icon(icon, size: 20),
          ),
        ),
      );
    }
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: AnimatedContainer(
        duration: NagaMotion.fast,
        curve: NagaMotion.standard,
        constraints: const BoxConstraints(minHeight: 48),
        width: double.infinity,
        decoration: BoxDecoration(
          color: active ? _accent.withValues(alpha: 0.10) : Colors.transparent,
          borderRadius: BorderRadius.circular(NagaRadius.small),
        ),
        child: TextButton.icon(
          onPressed: onPressed,
          style: TextButton.styleFrom(
            alignment: Alignment.centerLeft,
            padding: const EdgeInsets.symmetric(horizontal: 14),
            foregroundColor: active ? _text : _muted,
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(NagaRadius.small),
            ),
          ).copyWith(overlayColor: overlay),
          icon: Icon(icon, color: active ? _accent : _muted, size: 20),
          label: Text(
            label,
            style: TextStyle(
              color: active ? _text : _muted,
              fontWeight: active ? FontWeight.w600 : FontWeight.w500,
            ),
          ),
        ),
      ),
    );
  }
}

class _BottomNavigationBar extends StatelessWidget {
  const _BottomNavigationBar({
    required this.selectedIndex,
    required this.onSelect,
  });

  final int selectedIndex;
  final ValueChanged<int> onSelect;

  @override
  Widget build(BuildContext context) {
    final bottomIndex = switch (selectedIndex) {
      1 => 1,
      7 => 2,
      2 => 3,
      3 || 5 || 6 || 8 || 9 => 4,
      _ => 0,
    };
    return NavigationBar(
      selectedIndex: bottomIndex,
      onDestinationSelected: (index) => onSelect(const [0, 1, 7, 2, 3][index]),
      height: 76,
      labelBehavior: MediaQuery.textScalerOf(context).scale(12) > 18
          ? NavigationDestinationLabelBehavior.alwaysHide
          : NavigationDestinationLabelBehavior.alwaysShow,
      destinations: const [
        NavigationDestination(
          icon: Icon(Icons.home_outlined),
          selectedIcon: Icon(Icons.home_rounded),
          label: 'Главная',
        ),
        NavigationDestination(
          icon: Icon(Icons.person_outline_rounded),
          selectedIcon: Icon(Icons.person_rounded),
          label: 'Профили',
        ),
        NavigationDestination(
          icon: Icon(Icons.public_outlined),
          selectedIcon: Icon(Icons.public_rounded),
          label: 'Серверы',
        ),
        NavigationDestination(
          icon: Icon(Icons.bar_chart_outlined),
          selectedIcon: Icon(Icons.bar_chart_rounded),
          label: 'Статистика',
        ),
        NavigationDestination(
          icon: Icon(Icons.settings_outlined),
          selectedIcon: Icon(Icons.settings_rounded),
          label: 'Настройки',
        ),
      ],
    );
  }
}

class _TrafficModeSelector extends StatelessWidget {
  const _TrafficModeSelector({
    required this.value,
    required this.onChanged,
    this.errorMessage,
  });

  final String value;
  final ValueChanged<String> onChanged;
  final String? errorMessage;

  @override
  Widget build(BuildContext context) {
    if (nagaRunsOnAndroid) {
      return const Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Режим трафика',
            style: TextStyle(fontWeight: FontWeight.w600),
          ),
          SizedBox(height: 8),
          Text(
            'Трафик устройства направляется по правилам маршрутизации.',
            style: TextStyle(color: _muted, fontSize: 12),
          ),
        ],
      );
    }
    final selected = value == 'system_proxy' ? 'system_proxy' : 'tun';
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text(
          'Режим трафика',
          style: TextStyle(fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 8),
        SizedBox(
          width: double.infinity,
          child: SegmentedButton<String>(
            direction: MediaQuery.textScalerOf(context).scale(14) > 20
                ? Axis.vertical
                : Axis.horizontal,
            segments: const [
              ButtonSegment<String>(
                value: 'tun',
                icon: Icon(Icons.shield_outlined),
                label: Text('TUN'),
              ),
              ButtonSegment<String>(
                value: 'system_proxy',
                icon: Icon(Icons.swap_horiz_rounded),
                label: Text('Системный прокси'),
              ),
            ],
            selected: {selected},
            onSelectionChanged: (values) {
              if (values.isEmpty) return;
              onChanged(values.first);
            },
          ),
        ),
        const SizedBox(height: 5),
        Text(
          selected == 'tun'
              ? 'Трафик устройства направляется по правилам маршрутизации.'
              : 'Работает для приложений, поддерживающих системный прокси.',
          style: const TextStyle(color: _muted, fontSize: 12),
        ),
        if (selected == 'system_proxy') ...[
          const SizedBox(height: 3),
          Row(
            children: [
              const Text(
                '127.0.0.1:2080',
                style: TextStyle(color: _tertiary, fontSize: 12),
              ),
              IconButton(
                onPressed: () async {
                  await Clipboard.setData(
                    const ClipboardData(text: '127.0.0.1:2080'),
                  );
                  if (!context.mounted) return;
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('Адрес прокси скопирован.')),
                  );
                },
                tooltip: 'Скопировать адрес прокси',
                visualDensity: VisualDensity.compact,
                icon: const Icon(Icons.copy_rounded, size: 16),
              ),
            ],
          ),
        ],
        if (errorMessage != null && errorMessage!.isNotEmpty) ...[
          const SizedBox(height: 8),
          Text(
            selected == 'system_proxy'
                ? 'Не удалось включить системный прокси: $errorMessage'
                : errorMessage!,
            style: const TextStyle(color: _error, fontSize: 12),
          ),
        ],
      ],
    );
  }
}

String _formatRate(int value) {
  if (value <= 0) return '0 Б/с';
  return '${_formatBytes(value)}/с';
}

class NagaShareProfileDialog extends StatelessWidget {
  const NagaShareProfileDialog({
    super.key,
    required this.profileName,
    required this.url,
  });

  final String profileName;
  final String url;

  Future<void> _copy(BuildContext context) async {
    try {
      await Clipboard.setData(ClipboardData(text: url));
    } on PlatformException {
      if (context.mounted) {
        ScaffoldMessenger.maybeOf(context)?.showSnackBar(
          const SnackBar(
            content: Text('Не удалось скопировать ссылку. Попробуйте ещё раз.'),
          ),
        );
      }
      return;
    }
    if (!context.mounted) return;
    ScaffoldMessenger.maybeOf(context)
        ?.showSnackBar(const SnackBar(content: Text('Ссылка скопирована')));
  }

  @override
  Widget build(BuildContext context) {
    return Dialog(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 360),
        child: SingleChildScrollView(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(24, 20, 24, 16),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Text(
                  'Скопировать профиль',
                  textAlign: TextAlign.center,
                  style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 12),
                Text(
                  profileName,
                  textAlign: TextAlign.center,
                  style: const TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                const SizedBox(height: 16),
                DecoratedBox(
                  decoration: BoxDecoration(
                    color: Colors.white,
                    borderRadius: BorderRadius.circular(NagaRadius.card),
                  ),
                  child: Padding(
                    padding: const EdgeInsets.all(12),
                    child: SizedBox(
                      width: 180,
                      height: 180,
                      child: QrImageView(
                        data: url,
                        size: 180,
                        backgroundColor: Colors.white,
                      ),
                    ),
                  ),
                ),
                const SizedBox(height: 16),
                const Text(
                  'Отсканируй QR на телефоне, чтобы добавить ту же подписку.',
                  textAlign: TextAlign.center,
                  style: TextStyle(height: 1.45),
                ),
                const SizedBox(height: 8),
                const Text(
                  'Ссылка — секрет, как пароль. Не отправляй её в чат.',
                  textAlign: TextAlign.center,
                  style: TextStyle(color: _muted, height: 1.45),
                ),
                const SizedBox(height: 12),
                SelectableText(
                  url,
                  textAlign: TextAlign.center,
                  style: const TextStyle(
                    fontSize: 12,
                    color: _muted,
                    height: 1.4,
                  ),
                ),
                const SizedBox(height: 16),
                OverflowBar(
                  alignment: MainAxisAlignment.end,
                  overflowAlignment: OverflowBarAlignment.end,
                  spacing: 8,
                  overflowSpacing: 8,
                  children: [
                    TextButton(
                      onPressed: () => Navigator.of(context).pop(),
                      child: const Text('Закрыть'),
                    ),
                    FilledButton.icon(
                      onPressed: () => _copy(context),
                      icon: const Icon(Icons.copy_rounded),
                      label: const Text('Скопировать'),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _ProfilesPage extends StatelessWidget {
  const _ProfilesPage({
    required this.profileName,
    required this.profiles,
    required this.hasProfile,
    required this.onImport,
    required this.onRefresh,
    required this.onShare,
    required this.onDelete,
    required this.preview,
    required this.autoUpdate,
    required this.onAutoUpdateChanged,
  });

  final String profileName;
  final List<StoredProfileSummary> profiles;
  final bool hasProfile;
  final VoidCallback onImport;
  final ValueChanged<StoredProfileSummary> onRefresh;
  final ValueChanged<StoredProfileSummary> onShare;
  final ValueChanged<StoredProfileSummary> onDelete;
  final ProfilePreview? preview;
  final bool autoUpdate;
  final ValueChanged<bool> onAutoUpdateChanged;

  @override
  Widget build(BuildContext context) {
    return _SectionPage(
      eyebrow: 'NAGA NETWORK',
      title: 'Профили',
      subtitle: 'Ваши подписки и доступ к VPN.',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: double.infinity,
            child: Card(
              color: _surfaceLow,
              child: Padding(
                padding: const EdgeInsets.all(22),
                child: LayoutBuilder(
                  builder: (context, constraints) {
                    final compact = constraints.maxWidth < 560;
                    const title = 'Импорт профиля';
                    final copy = Expanded(
                      child: Text(
                        '$title\nДобавьте ссылку на подписку или файл конфига.',
                        style: const TextStyle(height: 1.45),
                      ),
                    );
                    final action = FilledButton(
                      onPressed: onImport,
                      child: const Text('Импорт по ссылке'),
                    );
                    if (compact) {
                      return Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Icon(
                            Icons.add_link_rounded,
                            color: _accent,
                            size: 28,
                          ),
                          const SizedBox(height: 14),
                          Text(
                            title,
                            style: const TextStyle(
                              color: _text,
                              fontSize: 18,
                              fontWeight: FontWeight.w500,
                            ),
                          ),
                          const SizedBox(height: 5),
                          const Text(
                            'Добавьте ссылку на подписку или файл конфига.',
                            style: TextStyle(color: _muted, height: 1.45),
                          ),
                          const SizedBox(height: 18),
                          SizedBox(width: double.infinity, child: action),
                        ],
                      );
                    }
                    return Row(
                      children: [
                        const Icon(
                          Icons.add_link_rounded,
                          color: _accent,
                          size: 28,
                        ),
                        const SizedBox(width: 16),
                        copy,
                        action,
                      ],
                    );
                  },
                ),
              ),
            ),
          ),
          const SizedBox(height: 24),
          if (hasProfile && preview != null && profiles.length <= 1) ...[
            _SubscriptionSummaryCard(hasProfile: hasProfile, preview: preview),
            const SizedBox(height: 12),
          ],
          if (hasProfile) ...[
            Card(
              child: SwitchListTile.adaptive(
                value: autoUpdate,
                onChanged: onAutoUpdateChanged,
                activeThumbColor: _text,
                secondary: const Icon(Icons.sync_rounded, color: _accent),
                title: const Text('Автоматически обновлять профили'),
                subtitle: Text(
                  autoUpdate
                      ? 'Все профили обновляются по расписанию подписки.'
                      : 'Обновлять вручную',
                  style: const TextStyle(color: _muted),
                ),
              ),
            ),
            const SizedBox(height: 24),
          ],
          Text('Мои профили', style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 12),
          if (!hasProfile && profiles.isEmpty)
            const Card(
              child: _NagaBackdrop(
                imageKey: Key('naga-empty-profiles-backdrop'),
                child: Padding(
                  padding: EdgeInsets.all(24),
                  child: SizedBox(
                    width: double.infinity,
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        _SoftIcon(Icons.add_link_rounded),
                        SizedBox(height: 20),
                        Text(
                          'Здесь пока пусто',
                          style: TextStyle(
                            color: _text,
                            fontSize: 22,
                            fontWeight: FontWeight.w500,
                          ),
                        ),
                        SizedBox(height: 10),
                        Text(
                          'Добавьте VPN-профиль по ссылке или файлу конфига.',
                          style: TextStyle(
                            color: _muted,
                            fontSize: 14,
                            height: 1.5,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            )
          else ...[
            for (final profile in profiles)
              Card(
                child: ListTile(
                  contentPadding: const EdgeInsets.symmetric(
                    horizontal: 18,
                    vertical: 8,
                  ),
                  leading: const Icon(Icons.public_rounded, color: _muted),
                  title: Text(profile.name),
                  subtitle: Text(
                    [
                      _storedProfileSubtitle(profile),
                      nagaProfileUpdatedLabel(
                        profile.updatedAt,
                        profile.importedAt,
                      ),
                    ].where((line) => line.isNotEmpty).join('\n'),
                    style: const TextStyle(color: _muted),
                  ),
                  trailing: PopupMenuButton<String>(
                    iconColor: _muted,
                    onSelected: (value) {
                      switch (value) {
                        case 'share':
                          onShare(profile);
                        case 'refresh':
                          onRefresh(profile);
                        case 'delete':
                          onDelete(profile);
                      }
                    },
                    itemBuilder: (context) => [
                      if (profile.hasSourceUrl)
                        const PopupMenuItem(
                          value: 'share',
                          child: Text('Скопировать'),
                        ),
                      if (profile.hasSourceUrl)
                        const PopupMenuItem(
                          value: 'refresh',
                          child: Text('Обновить профиль'),
                        ),
                      const PopupMenuItem(
                        value: 'delete',
                        child: Text('Удалить профиль'),
                      ),
                    ],
                  ),
                ),
              ),
            if (profiles.isEmpty)
              Card(
                child: ListTile(
                  leading: const Icon(Icons.public_rounded, color: _accent),
                  title: Text(profileName),
                  subtitle: const Text(
                    'Сохранённый профиль',
                    style: TextStyle(color: _muted),
                  ),
                ),
              ),
          ],
        ],
      ),
    );
  }

  String _storedProfileSubtitle(StoredProfileSummary profile) {
    final engine = profile.providerName.isEmpty
        ? profile.engine
        : '${profile.providerName} · ${profile.engine}';
    final expire = profile.expireUtc;
    if (expire == null) {
      return engine;
    }
    return '$engine · до ${_formatDate(expire)}';
  }
}

class _SubscriptionSummaryCard extends StatelessWidget {
  const _SubscriptionSummaryCard({
    required this.hasProfile,
    required this.preview,
  });

  final bool hasProfile;
  final ProfilePreview? preview;

  @override
  Widget build(BuildContext context) {
    final expired =
        preview?.expireUtc != null &&
        !DateTime.now().isBefore(preview!.expireUtc!);
    final status = !hasProfile
        ? 'Нет профиля'
        : preview == null
        ? 'Нет данных'
        : expired
        ? 'Истёк'
        : !preview!.canConnect
        ? 'Недоступна'
        : 'Активна';
    final statusColor = expired || preview?.canConnect == false
        ? const Color(0xFFFFB347)
        : !hasProfile || preview == null
        ? _muted
        : _success;
    final usedBytes =
        (preview?.uploadBytes ?? 0) + (preview?.downloadBytes ?? 0);
    final totalBytes = preview?.totalBytes ?? 0;
    final progress = totalBytes > 0
        ? (usedBytes / totalBytes).clamp(0.0, 1.0)
        : null;

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    preview?.profileTitle ?? 'Профиль',
                    style: TextStyle(
                      color: _text,
                      fontSize: 20,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 9,
                    vertical: 5,
                  ),
                  decoration: BoxDecoration(
                    color: statusColor.withValues(alpha: 0.16),
                    borderRadius: BorderRadius.circular(6),
                  ),
                  child: Text(
                    status,
                    style: TextStyle(
                      color: statusColor,
                      fontSize: 11,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 20),
            Text(
              !hasProfile
                  ? 'Профиль не добавлен'
                  : preview == null
                  ? 'Сведения о подписке недоступны'
                  : preview!.unlimited
                  ? 'Безлимитный профиль'
                  : 'Лимит ${_formatBytes(totalBytes)}',
              style: const TextStyle(
                color: _text,
                fontSize: 17,
                fontWeight: FontWeight.w500,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              hasProfile
                  ? (preview?.expireUtc == null
                        ? 'Дата окончания не указана'
                        : expired
                        ? 'Срок истёк ${_formatDate(preview!.expireUtc!)}'
                        : 'до ${_formatDate(preview!.expireUtc!)}')
                  : '—',
              style: const TextStyle(color: _muted, fontSize: 13, height: 1.5),
            ),
            if (progress != null) ...[
              const SizedBox(height: 20),
              ClipRRect(
                borderRadius: BorderRadius.circular(4),
                child: LinearProgressIndicator(
                  value: progress,
                  minHeight: 6,
                  backgroundColor: _line,
                  color: _accent,
                ),
              ),
              const SizedBox(height: 8),
              Text(
                '${_formatBytes(usedBytes)} использовано',
                style: const TextStyle(color: _tertiary, fontSize: 12),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _DashboardHeading extends StatelessWidget {
  const _DashboardHeading();

  @override
  Widget build(BuildContext context) {
    final mobile = MediaQuery.sizeOf(context).width < NagaBreakpoints.compact;
    return Row(
      children: [
        if (mobile) ...[
          Image.asset(_brandMarkAsset, width: 28, height: 28),
          const SizedBox(width: 10),
        ],
        Expanded(
          child: Padding(
            padding: const EdgeInsets.only(left: 2, top: 4, bottom: 4),
            child: Text(
              mobile ? 'NAGA NETWORK' : 'Главная',
              overflow: TextOverflow.visible,
              style: TextStyle(
                color: _text,
                fontSize: mobile ? 13 : 30,
                height: 1.25,
                fontWeight: FontWeight.w500,
                letterSpacing: mobile ? 1.4 : -0.2,
              ),
            ),
          ),
        ),
      ],
    );
  }
}

class _StatisticsPage extends StatelessWidget {
  const _StatisticsPage({required this.runtime, required this.trafficSamples});

  final RuntimeSnapshot? runtime;
  final List<TrafficSample> trafficSamples;

  @override
  Widget build(BuildContext context) {
    return _SectionPage(
      eyebrow: 'NAGA NETWORK',
      title: 'Статистика',
      subtitle: 'Трафик обновляется во время активного подключения.',
      child: Column(
        children: [
          LayoutBuilder(
            builder: (context, constraints) {
              final width = constraints.maxWidth < 700
                  ? constraints.maxWidth
                  : (constraints.maxWidth - 32) / 3;
              return Wrap(
                spacing: 16,
                runSpacing: 16,
                children: [
                  SizedBox(
                    width: width,
                    child: _MetricTile(
                      label: 'Загрузка',
                      value: _formatBytes(runtime?.downloadBytes ?? 0),
                      icon: Icons.download_rounded,
                      detail: runtime?.trafficAvailable == true
                          ? _formatRate(runtime!.downloadRateBytes)
                          : 'Нет данных',
                    ),
                  ),
                  SizedBox(
                    width: width,
                    child: _MetricTile(
                      label: 'Отдача',
                      value: _formatBytes(runtime?.uploadBytes ?? 0),
                      icon: Icons.upload_rounded,
                      detail: runtime?.trafficAvailable == true
                          ? _formatRate(runtime!.uploadRateBytes)
                          : 'Нет данных',
                    ),
                  ),
                  SizedBox(
                    width: width,
                    child: _MetricTile(
                      label: 'Сессия',
                      value: _formatDuration(runtime?.sessionStartedAt),
                      icon: Icons.timer_outlined,
                      detail: runtime?.status != 'connected'
                          ? 'Не подключено'
                          : runtime?.sessionStartedAt == null
                          ? 'Время сессии недоступно'
                          : 'Время работы VPN',
                    ),
                  ),
                ],
              );
            },
          ),
          const SizedBox(height: 16),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(20),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'Скорость трафика',
                    style: TextStyle(fontWeight: FontWeight.w700),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    runtime?.trafficAvailable == true
                        ? 'Загрузка ${_formatRate(runtime!.downloadRateBytes)} · Отправка ${_formatRate(runtime!.uploadRateBytes)}'
                        : 'Ожидаем данные о трафике',
                    style: const TextStyle(color: _muted),
                  ),
                  if (runtime?.activeNode?.isNotEmpty == true) ...[
                    const SizedBox(height: 6),
                    Text(
                      '${runtime!.activeCountry?.isNotEmpty == true ? runtime!.activeCountry : 'Страна не указана'} · ${runtime!.activeProtocol?.isNotEmpty == true ? runtime!.activeProtocol : 'Протокол не указан'}',
                      style: const TextStyle(color: _muted, fontSize: 12),
                    ),
                  ],
                  if (runtime?.status == 'connected' &&
                      runtime?.failoverMessage?.isNotEmpty == true) ...[
                    const SizedBox(height: 6),
                    Text(
                      runtime!.failoverMessage!,
                      style: const TextStyle(color: _warning, fontSize: 12),
                    ),
                  ],
                  const SizedBox(height: 16),
                  SizedBox(
                    height: 170,
                    width: double.infinity,
                    child: _TrafficChart(samples: trafficSamples),
                  ),
                  if (runtime?.lastTrafficUpdate != null) ...[
                    const SizedBox(height: 8),
                    Text(
                      'Обновлено ${_formatTime(runtime!.lastTrafficUpdate!)}',
                      style: const TextStyle(color: _tertiary, fontSize: 12),
                    ),
                  ],
                ],
              ),
            ),
          ),
          if (_friendlyRuntimeError(runtime?.error) != null) ...[
            const SizedBox(height: 16),
            Card(
              child: Padding(
                padding: const EdgeInsets.all(20),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Icon(Icons.info_outline_rounded, color: _warning),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Text(
                        _friendlyRuntimeError(runtime?.error)!,
                        style: const TextStyle(color: _muted, height: 1.5),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _TrafficChart extends StatelessWidget {
  const _TrafficChart({required this.samples});

  final List<TrafficSample> samples;

  @override
  Widget build(BuildContext context) {
    if (samples.length < 2) {
      return const Center(
        child: Text(
          'График появится после первых данных',
          style: TextStyle(color: _muted),
        ),
      );
    }
    return CustomPaint(painter: _TrafficChartPainter(samples));
  }
}

class _TrafficChartPainter extends CustomPainter {
  const _TrafficChartPainter(this.samples);

  final List<TrafficSample> samples;

  @override
  void paint(Canvas canvas, Size size) {
    const inset = 8.0;
    final chart = Rect.fromLTWH(
      inset,
      inset,
      size.width - inset * 2,
      size.height - inset * 2,
    );
    final grid = Paint()
      ..color = _line.withValues(alpha: 0.7)
      ..strokeWidth = 1;
    for (var index = 0; index < 4; index++) {
      final y = chart.top + chart.height * index / 3;
      canvas.drawLine(Offset(chart.left, y), Offset(chart.right, y), grid);
    }

    final maximum = samples.fold<int>(
      1,
      (max, sample) =>
          [max, sample.upload, sample.download].reduce((a, b) => a > b ? a : b),
    );
    Path lineFor(int Function(TrafficSample) value) {
      final path = Path();
      for (var index = 0; index < samples.length; index++) {
        final x = chart.left + chart.width * index / (samples.length - 1);
        final y = chart.bottom - chart.height * value(samples[index]) / maximum;
        if (index == 0) {
          path.moveTo(x, y);
        } else {
          path.lineTo(x, y);
        }
      }
      return path;
    }

    final download = Paint()
      ..color = _accent
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2.5
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;
    final upload = Paint()
      ..color = _success
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;
    canvas.drawPath(lineFor((sample) => sample.download), download);
    canvas.drawPath(lineFor((sample) => sample.upload), upload);
  }

  @override
  bool shouldRepaint(covariant _TrafficChartPainter oldDelegate) {
    return oldDelegate.samples != samples;
  }
}

String _formatBytes(int value) {
  if (value < 1024) return '$value Б';
  if (value < 1024 * 1024) return '${(value / 1024).toStringAsFixed(1)} КБ';
  if (value < 1024 * 1024 * 1024) {
    return '${(value / (1024 * 1024)).toStringAsFixed(1)} МБ';
  }
  return '${(value / (1024 * 1024 * 1024)).toStringAsFixed(2)} ГБ';
}

String _formatDuration(DateTime? startedAt) {
  if (startedAt == null) return '—';
  final elapsed = DateTime.now().difference(startedAt.toLocal());
  final hours = elapsed.inHours;
  final minutes = elapsed.inMinutes.remainder(60).toString().padLeft(2, '0');
  final seconds = elapsed.inSeconds.remainder(60).toString().padLeft(2, '0');
  return hours > 0 ? '$hours ч $minutes мин' : '$minutes:$seconds';
}

String _formatTime(DateTime value) {
  final local = value.toLocal();
  return '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')}:${local.second.toString().padLeft(2, '0')}';
}

String nagaProfileUpdatedLabel(DateTime? updatedAt, DateTime? importedAt) {
  final moment = updatedAt ?? importedAt;
  if (moment == null) {
    return '';
  }
  final local = moment.toLocal();
  final clock =
      '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')}';
  return 'Обновлён ${_formatDate(moment)}, $clock';
}

String _formatDate(DateTime value) {
  final local = value.toLocal();
  final day = local.day.toString().padLeft(2, '0');
  final month = local.month.toString().padLeft(2, '0');
  return '$day.$month.${local.year}';
}

String? _friendlyRuntimeError(String? error) {
  if (error == null || error.trim().isEmpty) return null;
  final normalized = error
      .replaceAll(RegExp(r'\x1b\[[0-9;]*m'), '')
      .toLowerCase();
  if (normalized.contains('ни один vpn-узел') ||
      normalized.contains('не прошёл проверку доступности') ||
      normalized.contains('не прошел проверку доступности')) {
    return 'Серверы сейчас недоступны. Проверь интернет без VPN и повтори.';
  }
  if (normalized.contains('hev-socks5-tunnel binary') ||
      normalized.contains('libhevfd')) {
    return 'Не удалось запустить VPN-туннель на этом устройстве.';
  }
  if (normalized.contains('olcrtc binary') ||
      normalized.contains('olcrtc mode is not available') ||
      normalized.contains('olcrtc недоступ')) {
    return 'olcRTC недоступен на этом устройстве.';
  }
  if (normalized.contains('запуск отменён') ||
      normalized.contains('права администратора не выданы') ||
      normalized.contains('uac')) {
    return 'Запуск VPN отменён: нужны права администратора. Подтверди запрос UAC и повтори.';
  }
  if (normalized.contains('already exists') ||
      normalized.contains('element not found') ||
      normalized.contains('cannot create a file') ||
      normalized.contains('старый tun ещё занят')) {
    return 'Не удалось создать сетевой адаптер VPN: старый TUN ещё занят. Закрой другие VPN и повтори, при необходимости перезагрузи Windows.';
  }
  if (normalized.contains('tunsetiff') ||
      normalized.contains('operation not permitted')) {
    return 'Linux не разрешил создать VPN-интерфейс. Проверь системные разрешения Naga.';
  }
  if (normalized.contains('configure tun') ||
      normalized.contains('create adapter')) {
    return 'Не удалось создать TUN-адаптер Windows. Подтверди запрос UAC и повтори подключение.';
  }
  if (normalized.contains('system dns') ||
      normalized.contains('resolvectl') ||
      normalized.contains('resolve1')) {
    return 'Не удалось переключить системный DNS на Naga. Перезапусти сервис Naga и попробуй снова.';
  }
  if (normalized.contains('legacy dns')) {
    return 'Профиль использует устаревшую настройку DNS. Обнови профиль или выбери другой узел.';
  }
  if (normalized.contains('empty direct outbound') ||
      normalized.contains('naga-dns')) {
    return 'Не удалось запустить VPN: ошибка DNS. Обнови приложение и повтори.';
  }
  if (normalized.contains('не удалось запустить vpn-ядро') ||
      normalized.contains('panic:') ||
      normalized.contains('nil pointer')) {
    return 'Не удалось запустить VPN на этом устройстве. Обнови приложение и повтори.';
  }
  if (normalized.contains('dns') ||
      normalized.contains('detour') ||
      normalized.contains('outbound')) {
    return 'Профиль содержит несовместимый DNS-маршрут. Обнови профиль или выбери другой узел.';
  }
  if (normalized.contains('tun')) {
    return 'Системе не удалось создать VPN-интерфейс. Проверь разрешения клиента.';
  }
  if (normalized.contains('permission') ||
      normalized.contains('access denied')) {
    return 'Не хватает системных разрешений для запуска VPN.';
  }
  if (normalized.contains('file exists') ||
      (normalized.contains('set routes') && normalized.contains('route'))) {
    return 'Другой VPN оставил маршруты в системе. Полностью выйди из другого VPN-клиента и попробуй снова.';
  }
  return 'Проверь профиль и попробуй подключиться ещё раз.';
}

String _friendlyImportError(String message) {
  final normalized = message.toLowerCase();
  if (normalized.contains('control-plane') ||
      normalized.contains('связаться')) {
    return 'Не удалось связаться с Naga. Проверь, что локальный сервис запущен.';
  }
  if (normalized.contains('must use https') ||
      normalized.contains('url is required') ||
      normalized.contains('url or config')) {
    return 'Нужна HTTPS-ссылка или файл VPN-профиля.';
  }
  if (normalized.contains('нет ссылки подписки')) {
    return 'Этот профиль добавлен из файла и не обновляется по ссылке.';
  }
  if (normalized.contains('host is not allowed')) {
    return 'Адрес подписки указывает на локальную сеть и не может быть загружен.';
  }
  if (normalized.contains('returned http 4')) {
    return 'Сервис подписки отклонил ссылку. Проверь её или запроси новую.';
  }
  if (normalized.contains('tls certificate') ||
      normalized.contains('x509') ||
      normalized.contains('certificate')) {
    return 'Не удалось проверить HTTPS-сертификат сайта подписки. Обнови Windows или добавь профиль из файла.';
  }
  if (normalized.contains('dns lookup') ||
      normalized.contains('no such host')) {
    return 'Не удалось найти сервер подписки. Проверь интернет и DNS.';
  }
  if (normalized.contains('timed out') || normalized.contains('timeout')) {
    return 'Домен подписки недоступен напрямую. Подключитесь профилем из файла и повторите добавление по ссылке.';
  }
  if (normalized.contains('endpoint blocked') ||
      normalized.contains('connection reset') ||
      normalized.contains('connectex')) {
    return 'Сеть, файрвол или антивирус разорвали соединение с сервером подписки. Добавь профиль из файла JSON.';
  }
  if (normalized.contains('returned http 5') ||
      normalized.contains('endpoint unavailable')) {
    return 'Сервис подписки временно недоступен. Попробуй добавить профиль из файла.';
  }
  if (normalized.contains('profile-title') ||
      normalized.contains('profile-update-interval') ||
      normalized.contains('subscription-userinfo') ||
      normalized.contains('response is invalid')) {
    return 'Сервис вернул неподдерживаемые данные профиля.';
  }
  if (normalized.contains('profile') || normalized.contains('config')) {
    return 'Не удалось распознать VPN-профиль. Проверь ссылку или файл и попробуй снова.';
  }
  return 'Не удалось добавить профиль. Попробуй ещё раз.';
}

class _DiagnosticsPage extends StatefulWidget {
  const _DiagnosticsPage({required this.controlPlane});

  final ControlPlaneClient controlPlane;

  @override
  State<_DiagnosticsPage> createState() => _DiagnosticsPageState();
}

class _DiagnosticsPageState extends State<_DiagnosticsPage> {
  ControlPlaneClient get _controlPlane => widget.controlPlane;
  bool _loading = true;
  bool _statusInFlight = false;
  bool? _serviceHealthy;
  RuntimeSnapshot? _runtime;
  List<DiagnosticLogEntry> _logs = const [];
  String? _checkError;
  Timer? _statusPoller;

  @override
  void initState() {
    super.initState();
    _check();
    _statusPoller = Timer.periodic(const Duration(seconds: 2), (_) {
      unawaited(_refreshStatus());
    });
  }

  @override
  void dispose() {
    _statusPoller?.cancel();
    super.dispose();
  }

  Future<void> _refreshStatus() async {
    if (_statusInFlight || _loading) return;
    _statusInFlight = true;
    try {
      final healthy = await _controlPlane.health();
      final runtime = await _controlPlane.runtimeStatus();
      if (!mounted) return;
      setState(() {
        _serviceHealthy = healthy;
        _runtime = runtime;
        _checkError = null;
      });
    } on ControlPlaneException catch (error) {
      if (!mounted) return;
      setState(() {
        _serviceHealthy = false;
        _runtime = null;
        _checkError = error.message;
      });
    } finally {
      _statusInFlight = false;
    }
  }

  Future<void> _check() async {
    if (mounted) {
      setState(() {
        _loading = true;
        _checkError = null;
      });
    }
    try {
      final healthy = await _controlPlane.health();
      final runtime = await _controlPlane.runtimeStatus();
      var logs = const <DiagnosticLogEntry>[];
      try {
        logs = await _controlPlane.diagnosticLogs();
      } on ControlPlaneException {
        // Runtime diagnostics remain useful while an older control-plane is
        // being upgraded or the persistent log is temporarily unavailable.
      }
      if (!mounted) return;
      setState(() {
        _serviceHealthy = healthy;
        _runtime = runtime;
        _logs = logs;
        _loading = false;
      });
    } on ControlPlaneException catch (error) {
      if (!mounted) return;
      setState(() {
        _serviceHealthy = false;
        _runtime = null;
        _checkError = error.message;
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final runtime = _runtime;
    final diagnosticsText = diagnosticsCopyText(
      serviceHealthy: _serviceHealthy,
      runtime: runtime,
      checkError: _checkError,
      logs: _logs,
    );

    return _SectionPage(
      eyebrow: 'NAGA NETWORK',
      title: 'Диагностика',
      subtitle: 'Проверяем состояние Naga и текущего соединения.',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.all(20),
              child: _loading
                  ? const Center(child: CircularProgressIndicator())
                  : DiagnosticsStatusCard(
                      serviceHealthy: _serviceHealthy,
                      runtime: runtime,
                      checkError: _checkError,
                    ),
            ),
          ),
          const SizedBox(height: 16),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(20),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      const Icon(Icons.receipt_long_outlined, color: _accent),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Text(
                          'Журнал событий',
                          style: Theme.of(context).textTheme.titleMedium
                              ?.copyWith(fontWeight: FontWeight.w700),
                        ),
                      ),
                      Text(
                        '${_logs.length} записей',
                        style: const TextStyle(color: _muted, fontSize: 12),
                      ),
                    ],
                  ),
                  const SizedBox(height: 8),
                  const Text(
                    'Сохраняется после отключения VPN и перезапуска приложения.',
                    style: TextStyle(color: _muted),
                  ),
                  const SizedBox(height: 14),
                  if (_logs.isEmpty)
                    const Text(
                      'Записей пока нет.',
                      style: TextStyle(color: _muted),
                    )
                  else
                    for (final entry in _logs.reversed.take(40)) ...[
                      Container(
                        width: double.infinity,
                        padding: const EdgeInsets.symmetric(vertical: 9),
                        decoration: const BoxDecoration(
                          border: Border(bottom: BorderSide(color: _line)),
                        ),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Row(
                              children: [
                                Icon(
                                  entry.level == 'error'
                                      ? Icons.error_outline_rounded
                                      : entry.level == 'warn'
                                      ? Icons.warning_amber_rounded
                                      : Icons.info_outline_rounded,
                                  size: 16,
                                  color: entry.level == 'error'
                                      ? _error
                                      : entry.level == 'warn'
                                      ? _warning
                                      : _accent,
                                ),
                                const SizedBox(width: 8),
                                Expanded(
                                  child: Text(
                                    '${entry.component} · ${entry.event}',
                                    style: const TextStyle(
                                      fontWeight: FontWeight.w700,
                                      fontSize: 12,
                                    ),
                                  ),
                                ),
                                Text(
                                  _formatDiagnosticTime(entry.timestamp),
                                  style: const TextStyle(
                                    color: _muted,
                                    fontSize: 11,
                                  ),
                                ),
                              ],
                            ),
                            if (entry.message.isNotEmpty) ...[
                              const SizedBox(height: 5),
                              Text(
                                entry.message,
                                style: const TextStyle(
                                  color: _muted,
                                  fontSize: 12,
                                ),
                              ),
                            ],
                            if (entry.fields.isNotEmpty) ...[
                              const SizedBox(height: 4),
                              Text(
                                entry.fields.entries
                                    .map((item) => '${item.key}=${item.value}')
                                    .join(' · '),
                                style: const TextStyle(
                                  color: _tertiary,
                                  fontSize: 10,
                                ),
                              ),
                            ],
                          ],
                        ),
                      ),
                    ],
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),
          Row(
            children: [
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: _loading ? null : _check,
                  icon: const Icon(Icons.refresh_rounded),
                  label: const Text('Повторить проверку'),
                ),
              ),
              const SizedBox(width: 12),
              IconButton(
                tooltip: 'Скопировать диагностику',
                onPressed: _loading
                    ? null
                    : () async {
                        await Clipboard.setData(
                          ClipboardData(text: diagnosticsText),
                        );
                        if (!mounted) return;
                        ScaffoldMessenger.of(context).showSnackBar(
                          const SnackBar(
                            content: Text('Диагностика скопирована.'),
                          ),
                        );
                      },
                icon: const Icon(Icons.copy_rounded),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

String diagnosticsCopyText({
  required bool? serviceHealthy,
  required RuntimeSnapshot? runtime,
  required String? checkError,
  required List<DiagnosticLogEntry> logs,
}) {
  final runtimeLabel = switch (runtime?.status) {
    'connected' => 'Подключено',
    'starting' || 'stopping' => 'Подключение выполняется',
    'failed' => 'Требует внимания',
    _ => 'Отключено',
  };
  return [
    'Локальный сервис: ${serviceHealthy == true ? 'работает' : 'недоступен'}',
    'VPN: $runtimeLabel',
    'Профиль: ${runtime?.profileId == null ? 'не подключён' : 'загружен'}',
    if (runtime?.activeNode?.isNotEmpty == true) 'Узел: ${runtime!.activeNode}',
    if (runtime?.activeCountry?.isNotEmpty == true ||
        runtime?.activeProtocol?.isNotEmpty == true)
      'Страна/протокол: ${runtime!.activeCountry ?? '—'} · ${runtime.activeProtocol ?? '—'}',
    if (runtime?.activeNodeStatus?.isNotEmpty == true)
      'Статус узла: ${runtime!.activeNodeStatus}',
    if (runtime?.failoverMessage?.isNotEmpty == true)
      'Переключение: ${runtime!.failoverMessage}',
    if (runtime?.error != null) 'Ошибка: ${runtime!.error}',
    if (checkError != null) 'Проверка: $checkError',
    if (logs.isNotEmpty) '',
    if (logs.isNotEmpty) 'Журнал событий:',
    ...logs.map(_formatDiagnosticLogEntry),
  ].join('\n');
}

class DiagnosticsStatusCard extends StatelessWidget {
  const DiagnosticsStatusCard({
    super.key,
    required this.serviceHealthy,
    required this.runtime,
    this.checkError,
  });

  final bool? serviceHealthy;
  final RuntimeSnapshot? runtime;
  final String? checkError;

  @override
  Widget build(BuildContext context) {
    final connected = checkError == null && runtime?.status == 'connected';
    final runtimeLabel = checkError != null
        ? 'Статус неизвестен'
        : switch (runtime?.status) {
            'connected' => 'Подключено',
            'starting' || 'stopping' => 'Подключение выполняется',
            'failed' => 'Требует внимания',
            _ => 'Отключено',
          };
    final node = runtime?.activeNode?.trim() ?? '';
    final country = runtime?.activeCountry?.trim() ?? '';
    final protocol = runtime?.activeProtocol?.trim() ?? '';
    final nodeStatus = runtime?.activeNodeStatus?.trim() ?? '';
    final failover = runtime?.failoverMessage?.trim() ?? '';
    return Column(
      children: [
        _DiagnosticLine(
          icon: Icons.hub_outlined,
          title: 'Локальный сервис',
          detail: serviceHealthy == true ? 'Работает' : 'Недоступен',
          ok: serviceHealthy == true,
        ),
        const Divider(height: 24, color: _line),
        _DiagnosticLine(
          icon: Icons.shield_outlined,
          title: 'VPN-соединение',
          detail: runtimeLabel,
          ok: connected,
        ),
        const Divider(height: 24, color: _line),
        _DiagnosticLine(
          icon: Icons.person_outline_rounded,
          title: 'Профиль',
          detail: runtime?.profileId == null ? 'Не подключён' : 'Загружен',
          ok: runtime?.profileId != null,
        ),
        if (node.isNotEmpty) ...[
          const Divider(height: 24, color: _line),
          _DiagnosticLine(
            icon: Icons.dns_outlined,
            title: 'Активный узел',
            detail: node,
            ok: connected,
          ),
        ],
        if (country.isNotEmpty || protocol.isNotEmpty) ...[
          const Divider(height: 24, color: _line),
          _DiagnosticLine(
            icon: Icons.public_outlined,
            title: 'Страна и протокол',
            detail: [
              if (country.isNotEmpty) country else 'Страна не указана',
              if (protocol.isNotEmpty) protocol else 'Протокол не указан',
            ].join(' · '),
            ok: connected,
          ),
        ],
        if (nodeStatus.isNotEmpty) ...[
          const Divider(height: 24, color: _line),
          _DiagnosticLine(
            icon: Icons.monitor_heart_outlined,
            title: 'Статус узла',
            detail: nodeStatus,
            ok: nodeStatus == 'healthy',
          ),
        ],
        if (failover.isNotEmpty) ...[
          const Divider(height: 24, color: _line),
          _DiagnosticLine(
            icon: Icons.swap_horiz_rounded,
            title: 'Переключение',
            detail: failover,
            ok: false,
          ),
        ],
        if (checkError != null || runtime?.error != null) ...[
          const Divider(height: 24, color: _line),
          Align(
            alignment: Alignment.centerLeft,
            child: Text(
              _friendlyRuntimeError(runtime?.error) ??
                  'Сервис недоступен. Проверь запуск Naga.',
              style: const TextStyle(color: _muted),
            ),
          ),
        ],
      ],
    );
  }
}

String _formatDiagnosticTime(DateTime? value) {
  if (value == null) return '—';
  final local = value.toLocal();
  String two(int number) => number.toString().padLeft(2, '0');
  return '${two(local.hour)}:${two(local.minute)}:${two(local.second)}';
}

String _formatDiagnosticLogEntry(DiagnosticLogEntry entry) {
  final fields = entry.fields.entries
      .map((item) => '${item.key}=${item.value}')
      .join(' ');
  return [
    _formatDiagnosticTime(entry.timestamp),
    entry.level.toUpperCase(),
    '${entry.component}/${entry.event}',
    entry.message,
    if (fields.isNotEmpty) fields,
  ].where((part) => part.isNotEmpty).join(' | ');
}

class _DiagnosticLine extends StatelessWidget {
  const _DiagnosticLine({
    required this.icon,
    required this.title,
    required this.detail,
    required this.ok,
  });

  final IconData icon;
  final String title;
  final String detail;
  final bool ok;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(icon, color: ok ? _success : _muted),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(title, style: const TextStyle(fontWeight: FontWeight.w600)),
              const SizedBox(height: 4),
              Text(detail, style: TextStyle(color: ok ? _success : _muted)),
            ],
          ),
        ),
      ],
    );
  }
}

class _NodesPage extends StatelessWidget {
  const _NodesPage({
    required this.nodes,
    required this.connected,
    required this.onSelect,
    this.onSelectAuto,
    this.onProbe,
    this.networkClass = 'unknown',
  });

  final List<UnifiedNode> nodes;
  final bool connected;
  final ValueChanged<UnifiedNode> onSelect;
  final VoidCallback? onSelectAuto;
  final Future<List<UnifiedNode>> Function()? onProbe;
  final String networkClass;

  @override
  Widget build(BuildContext context) {
    return _SectionPage(
      eyebrow: 'NAGA NETWORK',
      title: 'Серверы',
      subtitle: connected
          ? 'Выберите сервер или доверьте выбор Naga.'
          : 'Общий пул узлов из сохранённых профилей.',
      child: NagaNodesView(
        nodes: nodes,
        connected: connected,
        onSelect: onSelect,
        onSelectAuto: onSelectAuto,
        onProbe: onProbe,
        networkClass: networkClass,
      ),
    );
  }
}

class _RoutingPage extends StatelessWidget {
  const _RoutingPage({
    required this.policy,
    required this.apps,
    required this.discovered,
    required this.onSaveRouting,
    required this.onSaveApp,
    required this.onDeleteApp,
    required this.onRefreshDiscovered,
  });

  final RoutingPolicy policy;
  final List<AppRoute> apps;
  final List<DiscoveredApp> discovered;
  final ValueChanged<RoutingPolicy> onSaveRouting;
  final ValueChanged<AppRoute> onSaveApp;
  final ValueChanged<AppRoute> onDeleteApp;
  final Future<List<DiscoveredApp>> Function() onRefreshDiscovered;

  static const _routingLabels = {
    'all_vpn': 'Весь трафик через VPN',
    'selected_vpn': 'Только выбранные приложения через VPN',
    'selected_direct': 'Только выбранные приложения напрямую',
    'all_direct': 'Весь трафик напрямую',
  };

  @override
  Widget build(BuildContext context) {
    return _SectionPage(
      eyebrow: 'NAGA NETWORK',
      title: 'Какие приложения через VPN',
      subtitle: 'Выберите, какие приложения направлять через VPN и какие встроенные правила применять.',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.all(18),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'Режим трафика',
                    style: TextStyle(fontWeight: FontWeight.w700),
                  ),
                  const SizedBox(height: 10),
                  DropdownButtonFormField<String>(
                    initialValue: policy.mode,
                    isExpanded: true,
                    itemHeight: null,
                    decoration: const InputDecoration(
                      border: OutlineInputBorder(),
                      prefixIcon: Icon(Icons.alt_route_rounded),
                    ),
                    items: _routingLabels.entries
                        .map(
                          (entry) => DropdownMenuItem(
                            value: entry.key,
                            child: Text(entry.value),
                          ),
                        )
                        .toList(),
                    onChanged: (value) {
                      if (value == null) return;
                      onSaveRouting(policy.copyWith(mode: value, apps: apps));
                    },
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(18),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'Правила',
                    style: TextStyle(fontWeight: FontWeight.w700),
                  ),
                  SwitchListTile.adaptive(
                    contentPadding: EdgeInsets.zero,
                    value: policy.builtinPrivate,
                    activeThumbColor: _text,
                    onChanged: (value) => onSaveRouting(
                      policy.copyWith(builtinPrivate: value, apps: apps),
                    ),
                    title: const Text('Локальная сеть напрямую'),
                    subtitle: const Text(
                      'Домашняя сеть, loopback и link-local не уходят в VPN.',
                      style: TextStyle(color: _muted),
                    ),
                  ),
                  SwitchListTile.adaptive(
                    contentPadding: EdgeInsets.zero,
                    value: policy.builtinRU,
                    activeThumbColor: _text,
                    onChanged: (value) => onSaveRouting(
                      policy.copyWith(builtinRU: value, apps: apps),
                    ),
                    title: const Text('Российские домены напрямую'),
                    subtitle: const Text(
                      'Встроенные зоны .ru, .su и .рф подключаются напрямую.',
                      style: TextStyle(color: _muted),
                    ),
                  ),
                  SwitchListTile.adaptive(
                    contentPadding: EdgeInsets.zero,
                    value: policy.providerRules,
                    activeThumbColor: _text,
                    onChanged: (value) => onSaveRouting(
                      policy.copyWith(providerRules: value, apps: apps),
                    ),
                    title: const Text('Использовать правила VPN-провайдера'),
                    subtitle: const Text(
                      'Списки из подписки: сайты вроде зарубежных доменов сервисов тоже могут идти напрямую.',
                      style: TextStyle(color: _muted),
                    ),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),
          _ApplicationsPage(
            apps: apps,
            discovered: discovered,
            defaultRoute: policy.mode == 'selected_direct' ? 'direct' : 'vpn',
            onSave: onSaveApp,
            onDelete: onDeleteApp,
            onRefreshDiscovered: onRefreshDiscovered,
          ),
        ],
      ),
    );
  }
}

class _ServerChoicePage extends StatelessWidget {
  const _ServerChoicePage({
    required this.connectionPolicy,
    required this.onSaveConnection,
  });

  final ConnectionPolicy connectionPolicy;
  final ValueChanged<ConnectionPolicy> onSaveConnection;

  static const _connectionLabels = {
    'auto': 'Автоматически',
    'manual_fallback': 'Ручной выбор с запасным сервером',
    'manual_strict': 'Только выбранный сервер',
  };

  @override
  Widget build(BuildContext context) {
    return _SectionPage(
      eyebrow: 'NAGA NETWORK',
      title: 'Как выбирать сервер',
      subtitle: 'Авто, запасной вариант или только выбранная страна.',
      child: Card(
        child: Padding(
          padding: const EdgeInsets.all(18),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              DropdownButtonFormField<String>(
                initialValue: connectionPolicy.mode,
                isExpanded: true,
                itemHeight: null,
                decoration: const InputDecoration(
                  border: OutlineInputBorder(),
                  prefixIcon: Icon(Icons.bolt_rounded),
                ),
                items: _connectionLabels.entries
                    .map(
                      (entry) => DropdownMenuItem(
                        value: entry.key,
                        child: Text(entry.value),
                      ),
                    )
                    .toList(),
                onChanged: (value) {
                  if (value == null) return;
                  onSaveConnection(
                    ConnectionPolicy(
                      mode: value,
                      trafficMode: connectionPolicy.trafficMode,
                      preferredCountry: connectionPolicy.preferredCountry,
                      preferredProtocol: connectionPolicy.preferredProtocol,
                      networkClass: connectionPolicy.networkClass,
                      tuicFallbackEnabled: connectionPolicy.tuicFallbackEnabled,
                    ),
                  );
                },
              ),
              const SizedBox(height: 12),
              Text(
                'Текущая сеть: ${connectionPolicy.networkClass}',
                style: const TextStyle(color: _muted),
              ),
              const SizedBox(height: 4),
              const Text(
                'На сотовой сети сначала пробуем VLESS. TUIC — последний запасной вариант.',
                style: TextStyle(color: _muted),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _ApplicationsPage extends StatelessWidget {
  const _ApplicationsPage({
    required this.apps,
    required this.discovered,
    required this.defaultRoute,
    required this.onSave,
    required this.onDelete,
    required this.onRefreshDiscovered,
  });

  final List<AppRoute> apps;
  final List<DiscoveredApp> discovered;
  final String defaultRoute;
  final ValueChanged<AppRoute> onSave;
  final ValueChanged<AppRoute> onDelete;
  final Future<List<DiscoveredApp>> Function() onRefreshDiscovered;

  @override
  Widget build(BuildContext context) {
    final emptyHint = nagaUsesWindowsChrome
        ? 'Добавь имя exe, например Telegram.exe, или полный путь к программе.'
        : nagaRunsOnAndroid
        ? 'Выбери приложение из списка установленных пакетов.'
        : 'Добавь имя процесса, например telegram-desktop или firefox.';
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Align(
          alignment: Alignment.centerRight,
          child: FilledButton.icon(
            onPressed: () async {
              final refreshed = await onRefreshDiscovered();
              if (!context.mounted) return;
              final app = await _showAddAppDialog(
                context,
                refreshed,
                defaultRoute,
              );
              if (app != null) onSave(app);
            },
            icon: const Icon(Icons.add_rounded),
            label: const Text('Добавить приложение'),
          ),
        ),
        const SizedBox(height: 16),
        if (apps.isEmpty)
          Card(
            child: ListTile(
              leading: const Icon(Icons.apps_rounded, color: _accent),
              title: const Text('Список приложений пуст'),
              subtitle: Text(emptyHint, style: const TextStyle(color: _muted)),
            ),
          )
        else
          Card(
            child: Column(
              children: [
                for (var index = 0; index < apps.length; index++) ...[
                  ListTile(
                    leading: Icon(
                      apps[index].route == 'vpn'
                          ? Icons.shield_rounded
                          : Icons.public_rounded,
                      color: apps[index].route == 'vpn' ? _accent : _muted,
                    ),
                    trailing: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        PopupMenuButton<String>(
                          tooltip: 'Маршрут',
                          onSelected: (value) {
                            if (value == 'delete') {
                              unawaited(_confirmDelete(context, apps[index]));
                              return;
                            }
                            onSave(
                              AppRoute(
                                id: apps[index].id,
                                displayName: apps[index].displayName,
                                platform: apps[index].platform,
                                packageOrProcessId:
                                    apps[index].packageOrProcessId,
                                route: value,
                                enabled: apps[index].enabled,
                              ),
                            );
                          },
                          itemBuilder: (context) => const [
                            PopupMenuItem(value: 'vpn', child: Text('VPN')),
                            PopupMenuItem(
                              value: 'direct',
                              child: Text('DIRECT'),
                            ),
                            PopupMenuDivider(),
                            PopupMenuItem(
                              value: 'delete',
                              child: Text('Удалить'),
                            ),
                          ],
                        ),
                        Switch.adaptive(
                          value: apps[index].enabled,
                          activeThumbColor: _text,
                          onChanged: (enabled) => onSave(
                            AppRoute(
                              id: apps[index].id,
                              displayName: apps[index].displayName,
                              platform: apps[index].platform,
                              packageOrProcessId:
                                  apps[index].packageOrProcessId,
                              route: apps[index].route,
                              enabled: enabled,
                            ),
                          ),
                        ),
                      ],
                    ),
                    title: Text(apps[index].displayName),
                    subtitle: Text(
                      '${apps[index].packageOrProcessId} · ${apps[index].route == 'vpn' ? 'VPN' : 'DIRECT'}',
                      style: const TextStyle(color: _muted),
                    ),
                  ),
                  if (index != apps.length - 1)
                    const Divider(height: 1, color: _line),
                ],
              ],
            ),
          ),
        const SizedBox(height: 12),
        const Text(
          'После изменения правил может потребоваться переподключение VPN.',
          style: TextStyle(color: _muted),
        ),
      ],
    );
  }

  Future<void> _confirmDelete(BuildContext context, AppRoute app) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (context) {
        return AlertDialog(
          title: const Text('Удалить правило?'),
          content: Text('«${app.displayName}» будет убран из списка.'),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(context).pop(false),
              child: const Text('Отмена'),
            ),
            FilledButton(
              onPressed: () => Navigator.of(context).pop(true),
              child: const Text('Удалить'),
            ),
          ],
        );
      },
    );
    if (ok == true) onDelete(app);
  }

  Future<AppRoute?> _showAddAppDialog(
    BuildContext context,
    List<DiscoveredApp> discoveredApps,
    String route,
  ) async {
    var name = '';
    var process = '';
    var query = '';
    final processHint = nagaUsesWindowsChrome
        ? r'Telegram.exe или C:\Program Files\...\app.exe'
        : nagaRunsOnAndroid
        ? 'eu.example.app'
        : 'firefox или /usr/bin/firefox';
    final result = await showDialog<AppRoute>(
      context: context,
      builder: (context) {
        return StatefulBuilder(
          builder: (context, setDialogState) {
            final matches = discoveredApps
                .where((app) {
                  final haystack =
                      '${app.name} ${app.process} ${app.processPath}'
                          .toLowerCase();
                  return query.trim().isEmpty ||
                      haystack.contains(query.trim().toLowerCase());
                })
                .toList(growable: false);
            final emptyMatchesText = discoveredApps.isEmpty
                ? (nagaUsesWindowsChrome
                      ? 'Список запущенных процессов пуст или недоступен — введи имя exe вручную.'
                      : nagaRunsOnAndroid
                      ? 'Список приложений пуст или нет QUERY_ALL_PACKAGES — введи имя пакета вручную.'
                      : 'Нет подходящих процессов — введи имя вручную.')
                : 'Нет совпадений — введи имя вручную.';
            return AlertDialog(
              title: const Text('Добавить приложение'),
              content: SizedBox(
                width: 420,
                child: SingleChildScrollView(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      TextField(
                        onChanged: (value) =>
                            setDialogState(() => query = value),
                        decoration: const InputDecoration(
                          prefixIcon: Icon(Icons.search_rounded),
                          labelText: 'Поиск среди запущенных',
                        ),
                      ),
                      const SizedBox(height: 8),
                      ConstrainedBox(
                        constraints: const BoxConstraints(maxHeight: 180),
                        child: matches.isEmpty
                            ? Text(
                                emptyMatchesText,
                                style: const TextStyle(color: _muted),
                              )
                            : ListView.builder(
                                shrinkWrap: true,
                                itemCount: matches.length,
                                itemBuilder: (context, index) {
                                  final app = matches[index];
                                  return ListTile(
                                    dense: true,
                                    title: Text(app.name),
                                    subtitle: Text(
                                      app.processPath.isNotEmpty
                                          ? app.processPath
                                          : app.process,
                                      maxLines: 1,
                                      overflow: TextOverflow.ellipsis,
                                    ),
                                    onTap: () {
                                      Navigator.of(context).pop(
                                        AppRoute(
                                          id: app.ruleId,
                                          displayName: app.name,
                                          platform: nagaAppPlatform,
                                          packageOrProcessId: app.ruleProcessId,
                                          route: route,
                                          enabled: true,
                                        ),
                                      );
                                    },
                                  );
                                },
                              ),
                      ),
                      const SizedBox(height: 12),
                      TextField(
                        onChanged: (value) => name = value,
                        decoration: const InputDecoration(
                          labelText: 'Название',
                        ),
                      ),
                      const SizedBox(height: 12),
                      TextField(
                        onChanged: (value) => process = value,
                        decoration: InputDecoration(
                          labelText: 'Имя процесса или путь',
                          hintText: processHint,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
              actions: [
                TextButton(
                  onPressed: () => Navigator.of(context).pop(),
                  child: const Text('Отмена'),
                ),
                FilledButton(
                  onPressed: () {
                    final id = process.trim();
                    if (id.isEmpty) return;
                    Navigator.of(context).pop(
                      AppRoute(
                        id: id.contains('\\') ? processBaseName(id) : id,
                        displayName: name.trim().isEmpty
                            ? processBaseName(id)
                            : name.trim(),
                        platform: nagaAppPlatform,
                        packageOrProcessId: id,
                        route: route,
                        enabled: true,
                      ),
                    );
                  },
                  child: const Text('Добавить'),
                ),
              ],
            );
          },
        );
      },
    );
    return result;
  }
}

String processBaseName(String path) {
  final normalized = path.replaceAll('\\', '/');
  final parts = normalized.split('/');
  final base = parts.isEmpty ? path : parts.last;
  return base.isEmpty ? path : base;
}

class _SettingsPage extends StatefulWidget {
  const _SettingsPage({
    required this.autoUpdate,
    required this.onAutoUpdateChanged,
    required this.autoAppUpdate,
    required this.onAutoAppUpdateChanged,
    required this.onOpenDiagnostics,
    required this.onOpenHelp,
    required this.onOpenSplitTunnel,
    required this.onOpenServerChoice,
    required this.trafficMode,
    required this.onSelectTrafficMode,
    this.trafficModeError,
    required this.uiVersion,
    required this.controlVersion,
    required this.updateBusy,
    this.updateProgress,
    this.updateMessage,
    this.updateError,
    this.updateCheck,
    required this.onCheckUpdate,
    required this.onInstallUpdate,
  });

  final bool autoUpdate;
  final ValueChanged<bool> onAutoUpdateChanged;
  final bool autoAppUpdate;
  final ValueChanged<bool> onAutoAppUpdateChanged;
  final VoidCallback onOpenDiagnostics;
  final VoidCallback onOpenHelp;
  final VoidCallback onOpenSplitTunnel;
  final VoidCallback onOpenServerChoice;
  final String trafficMode;
  final ValueChanged<String> onSelectTrafficMode;
  final String? trafficModeError;
  final String uiVersion;
  final String controlVersion;
  final bool updateBusy;
  final double? updateProgress;
  final String? updateMessage;
  final String? updateError;
  final UpdateCheckResult? updateCheck;
  final VoidCallback onCheckUpdate;
  final VoidCallback onInstallUpdate;

  @override
  State<_SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<_SettingsPage> {
  @override
  Widget build(BuildContext context) {
    final connection = _SettingsGroup(
      title: 'Подключение',
      children: [
        Padding(
          padding: const EdgeInsets.all(20),
          child: _TrafficModeSelector(
            value: widget.trafficMode,
            onChanged: widget.onSelectTrafficMode,
            errorMessage: widget.trafficModeError,
          ),
        ),
        _SettingsRow(
          icon: Icons.apps_rounded,
          title: 'Какие приложения через VPN',
          detail: 'Все приложения или только выбранные',
          trailing: Icons.chevron_right_rounded,
          onTap: widget.onOpenSplitTunnel,
        ),
        _SettingsRow(
          icon: Icons.bolt_rounded,
          title: 'Как выбирать сервер',
          detail: 'Автоматически или вручную',
          trailing: Icons.chevron_right_rounded,
          onTap: widget.onOpenServerChoice,
        ),
        _SettingsRow(
          icon: Icons.health_and_safety_outlined,
          title: 'Диагностика',
          detail: 'Проверить соединение',
          trailing: Icons.chevron_right_rounded,
          onTap: widget.onOpenDiagnostics,
        ),
      ],
    );
    final application = _SettingsGroup(
      title: 'Приложение',
      children: [
        SwitchListTile.adaptive(
          contentPadding: const EdgeInsets.symmetric(
            horizontal: 20,
            vertical: 10,
          ),
          value: widget.autoUpdate,
          onChanged: widget.onAutoUpdateChanged,
          activeThumbColor: _text,
          title: const Text(
            'Обновление профиля',
            style: TextStyle(fontWeight: FontWeight.w600),
          ),
          subtitle: Text(
            widget.autoUpdate ? 'По расписанию подписки' : 'Вручную',
            style: const TextStyle(color: _muted),
          ),
        ),
        SwitchListTile.adaptive(
          contentPadding: const EdgeInsets.symmetric(
            horizontal: 20,
            vertical: 10,
          ),
          value: widget.autoAppUpdate,
          onChanged: widget.onAutoAppUpdateChanged,
          activeThumbColor: _text,
          title: const Text(
            'Автообновление приложения',
            style: TextStyle(fontWeight: FontWeight.w600),
          ),
          subtitle: Text(
            widget.autoAppUpdate
                ? 'Проверка при запуске и установка'
                : 'Только по кнопке',
            style: const TextStyle(color: _muted),
          ),
        ),
        _SettingsRow(
          icon: Icons.help_outline_rounded,
          title: 'Помощь',
          detail: 'Ответы и подсказки',
          trailing: Icons.chevron_right_rounded,
          onTap: widget.onOpenHelp,
        ),
        _SettingsRow(
          icon: Icons.info_outline_rounded,
          title: 'О приложении',
          detail: 'Naga Network ${widget.uiVersion}',
          trailing: Icons.chevron_right_rounded,
          onTap: () => showAboutDialog(
            context: context,
            applicationName: 'Naga Network',
            applicationVersion: widget.uiVersion,
            applicationLegalese: 'Свободный прокси-клиент для Naga Network.',
          ),
        ),
        _AppUpdateTile(
          uiVersion: widget.uiVersion,
          controlVersion: widget.controlVersion,
          busy: widget.updateBusy,
          progress: widget.updateProgress,
          message: widget.updateMessage,
          error: widget.updateError,
          check: widget.updateCheck,
          onCheck: widget.updateBusy ? null : widget.onCheckUpdate,
          onInstall: widget.updateBusy ? null : widget.onInstallUpdate,
        ),
      ],
    );
    return _SectionPage(
      eyebrow: 'NAGA NETWORK',
      title: 'Настройки',
      subtitle: 'Всё необходимое для вашего подключения.',
      child: LayoutBuilder(
        builder: (context, constraints) {
          if (constraints.maxWidth >= 840 &&
              MediaQuery.textScalerOf(context).scale(14) <= 18) {
            return Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Expanded(flex: 6, child: connection),
                const SizedBox(width: 20),
                Expanded(flex: 5, child: application),
              ],
            );
          }
          return Column(
            children: [connection, const SizedBox(height: 24), application],
          );
        },
      ),
    );
  }
}

class _AppUpdateTile extends StatelessWidget {
  const _AppUpdateTile({
    required this.uiVersion,
    required this.controlVersion,
    required this.busy,
    this.progress,
    this.message,
    this.error,
    this.check,
    this.onCheck,
    this.onInstall,
  });

  final String uiVersion;
  final String controlVersion;
  final bool busy;
  final double? progress;
  final String? message;
  final String? error;
  final UpdateCheckResult? check;
  final VoidCallback? onCheck;
  final VoidCallback? onInstall;

  @override
  Widget build(BuildContext context) {
    final kind = detectInstallKind();
    final behind = controlPlaneBehind(uiVersion, controlVersion);
    final canInstall = check?.newerAvailable == true &&
        (check?.missingAssets.isEmpty ?? false) &&
        kind != UpdateInstallKind.linuxDev;
    String detail = 'Текущая версия $uiVersion';
    if (behind) {
      detail =
          'Приложение $uiVersion, control-plane $controlVersion — обновите runtime';
    } else if (kind == UpdateInstallKind.linuxDev) {
      detail = 'Проверка доступна. Установка — только из AppImage.';
    }
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            detail,
            style: TextStyle(
              color: behind ? _warning : _muted,
              height: 1.4,
            ),
          ),
          if (message != null) ...[
            const SizedBox(height: 8),
            Text(message!, style: const TextStyle(color: _text)),
          ],
          if (error != null) ...[
            const SizedBox(height: 8),
            Text(error!, style: const TextStyle(color: _error, height: 1.4)),
          ],
          if (progress != null) ...[
            const SizedBox(height: 12),
            LinearProgressIndicator(
              value: progress,
              color: _accent,
              backgroundColor: _line,
            ),
          ],
          const SizedBox(height: 12),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              OutlinedButton(
                onPressed: onCheck,
                child: Text(busy ? 'Проверяем…' : 'Проверить обновление'),
              ),
              if (canInstall)
                FilledButton(
                  onPressed: onInstall,
                  child: Text('Скачать и установить ${check!.latestVersion}'),
                ),
            ],
          ),
        ],
      ),
    );
  }
}

class _HelpPage extends StatelessWidget {
  const _HelpPage();

  @override
  Widget build(BuildContext context) {
    return _SectionPage(
      eyebrow: 'NAGA NETWORK',
      title: 'Помощь',
      subtitle: 'Быстрые ответы и подсказки по подключению.',
      child: Column(
        children: [
          const Card(
            child: _NagaBackdrop(
              imageKey: Key('naga-help-backdrop'),
              child: Padding(
                padding: EdgeInsets.all(24),
                child: SizedBox(
                  width: double.infinity,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'VPN не работает?',
                        style: TextStyle(
                          color: _text,
                          fontSize: 22,
                          fontWeight: FontWeight.w500,
                        ),
                      ),
                      SizedBox(height: 12),
                      Text(
                        'Проверьте интернет, профиль и срок подписки.',
                        style: TextStyle(color: _muted, height: 1.5),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
          const SizedBox(height: 16),
          Card(
            child: Column(
              children: const [
                ExpansionTile(
                  leading: Icon(Icons.link_rounded, color: _accent),
                  title: Text('Как добавить профиль?'),
                  children: [
                    Padding(
                      padding: EdgeInsets.fromLTRB(56, 0, 20, 16),
                      child: Text(
                        'Открой «Профили», нажми «Импорт по ссылке» и вставь HTTPS-ссылку подписки или выбери файл конфига (.json, .conf). Уже добавленный профиль по ссылке можно «Скопировать».',
                        style: TextStyle(color: _muted, height: 1.45),
                      ),
                    ),
                  ],
                ),
                Divider(height: 1, color: _line),
                ExpansionTile(
                  leading: Icon(Icons.shield_outlined, color: _accent),
                  title: Text('Почему VPN не подключается?'),
                  children: [
                    Padding(
                      padding: EdgeInsets.fromLTRB(56, 0, 20, 16),
                      child: Text(
                        'Проверь интернет, обнови профиль и попробуй другой сервер. Если ошибка повторяется, открой диагностику.',
                        style: TextStyle(color: _muted, height: 1.45),
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _SectionPage extends StatelessWidget {
  const _SectionPage({
    required this.eyebrow,
    required this.title,
    required this.subtitle,
    required this.child,
  });

  final String eyebrow;
  final String title;
  final String subtitle;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _SectionHeader(title: title, subtitle: subtitle),
        const SizedBox(height: 28),
        child,
      ],
    );
  }
}

class _MetricTile extends StatelessWidget {
  const _MetricTile({
    required this.label,
    required this.value,
    required this.icon,
    this.detail,
  });

  final String label;
  final String value;
  final IconData icon;
  final String? detail;

  @override
  Widget build(BuildContext context) => _HomeMetricCard(
    icon: icon,
    label: label,
    value: value,
    detail: detail ?? '',
  );
}

class _SettingsRow extends StatelessWidget {
  const _SettingsRow({
    required this.icon,
    required this.title,
    required this.detail,
    required this.trailing,
    this.onTap,
  });

  final IconData icon;
  final String title;
  final String detail;
  final IconData trailing;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return ListTile(
      contentPadding: const EdgeInsets.symmetric(horizontal: 20, vertical: 10),
      leading: _SoftIcon(icon, size: 36),
      title: Text(title, style: const TextStyle(fontWeight: FontWeight.w600)),
      subtitle: Text(detail, style: const TextStyle(color: _muted)),
      trailing: Icon(trailing, color: _muted),
      onTap: onTap,
    );
  }
}

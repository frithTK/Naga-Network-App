part of '../main.dart';

/// Брендовый фон: не участвует в раскладке, фокусе и обработке нажатий.
class _NagaBackdrop extends StatelessWidget {
  const _NagaBackdrop({required this.child, required this.imageKey, this.mood});
  final Widget child;
  final Key imageKey;

  /// Когда задано, карточка показывает позу маскота из брендпака вместо
  /// приглушённого ростового портрета.
  final NagaMascotMood? mood;

  @override
  Widget build(BuildContext context) => ClipRRect(
    borderRadius: BorderRadius.circular(NagaRadius.card),
    child: ColoredBox(
      color: _surfaceLow,
      child: Stack(
        children: [
          Positioned.fill(
            child: ExcludeSemantics(
              child: IgnorePointer(
                child: mood == null ? _portrait() : _stateMascot(context),
              ),
            ),
          ),
          child,
        ],
      ),
    ),
  );

  Widget _portrait() => LayoutBuilder(
    builder: (context, constraints) {
      final height = (constraints.maxHeight * 1.6).clamp(240.0, 800.0);
      final width = height * 2 / 3;
      return Stack(
        children: [
          Positioned(
            right: -width * 0.12,
            top: -height * 0.1,
            width: width,
            height: height,
            child: ShaderMask(
              blendMode: BlendMode.dstIn,
              shaderCallback: (bounds) => const LinearGradient(
                begin: Alignment.topCenter,
                end: Alignment.bottomCenter,
                colors: [
                  Colors.transparent,
                  Colors.white,
                  Colors.white,
                  Colors.transparent,
                ],
                stops: [0, 0.15, 0.65, 1],
              ).createShader(bounds),
              child: Image.asset(
                _brandMascotCutoutAsset,
                key: imageKey,
                opacity: const AlwaysStoppedAnimation(0.16),
                fit: BoxFit.contain,
                filterQuality: FilterQuality.medium,
              ),
            ),
          ),
          Positioned.fill(
            child: DecoratedBox(
              decoration: BoxDecoration(
                gradient: LinearGradient(
                  colors: [
                    _surfaceLow.withValues(alpha: 0.85),
                    _surfaceLow.withValues(alpha: 0.1),
                  ],
                  stops: const [0, 0.75],
                ),
              ),
            ),
          ),
        ],
      );
    },
  );

  Widget _stateMascot(BuildContext context) => LayoutBuilder(
    key: imageKey,
    builder: (context, constraints) {
      // Поза живёт в углу карточки и не должна наезжать на центральный
      // столбец с кнопкой и подписями, поэтому ограничена и по ширине.
      final side = math
          .min(constraints.maxWidth * 0.30, constraints.maxHeight * 0.44)
          .clamp(72.0, 160.0);
      return Stack(
        children: [
          Positioned(
            right: 0,
            bottom: 0,
            width: side,
            height: side,
            child: AnimatedSwitcher(
              duration: MediaQuery.disableAnimationsOf(context)
                  ? Duration.zero
                  : NagaMotion.slow,
              switchInCurve: NagaMotion.standard,
              child: Image.asset(
                _brandMascotStateAsset(mood!),
                key: ValueKey(mood),
                opacity: const AlwaysStoppedAnimation(0.9),
                fit: BoxFit.contain,
                filterQuality: FilterQuality.medium,
              ),
            ),
          ),
        ],
      );
    },
  );
}

/// Общая иерархия заголовков разделов; бренд остаётся в оболочке приложения.
class _SectionHeader extends StatelessWidget {
  const _SectionHeader({required this.title, required this.subtitle});
  final String title, subtitle;

  @override
  Widget build(BuildContext context) {
    final compact = MediaQuery.sizeOf(context).width < NagaBreakpoints.compact;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: TextStyle(
            color: _text,
            fontSize: compact ? 28 : 32,
            height: 1.2,
            fontWeight: FontWeight.w500,
            letterSpacing: -0.8,
          ),
        ),
        const SizedBox(height: 10),
        Text(
          subtitle,
          style: const TextStyle(color: _muted, fontSize: 14, height: 1.5),
        ),
      ],
    );
  }
}

class _SoftIcon extends StatelessWidget {
  const _SoftIcon(this.icon, {this.accent = false, this.size = 40});
  final IconData icon;
  final bool accent;
  final double size;

  @override
  Widget build(BuildContext context) => Container(
    width: size,
    height: size,
    decoration: BoxDecoration(
      color: accent ? _accent.withValues(alpha: 0.08) : _surface,
      borderRadius: BorderRadius.circular(NagaRadius.control),
    ),
    child: Icon(icon, color: accent ? _accent : _muted, size: 20),
  );
}

class _SettingsGroup extends StatelessWidget {
  const _SettingsGroup({required this.title, required this.children});
  final String title;
  final List<Widget> children;

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      Padding(
        padding: const EdgeInsets.fromLTRB(4, 0, 4, 12),
        child: Text(
          title,
          style: const TextStyle(
            color: _muted,
            fontSize: 13,
            fontWeight: FontWeight.w500,
          ),
        ),
      ),
      Card(
        child: Column(
          children: [
            for (var i = 0; i < children.length; i++) ...[
              if (i > 0) const Divider(height: 1, indent: 20, endIndent: 20),
              children[i],
            ],
          ],
        ),
      ),
    ],
  );
}

import 'package:flutter/material.dart';

import '../../app/services.dart';
import '../l10n/localization.dart';
import '../theme/tokens.dart';

/// One bar in a [BarChart].
class Bar {
  const Bar(this.label, this.value, {this.color = Tokens.primary, this.caption});
  final String label;
  final double value;
  final Color color;
  final String? caption;
}

/// A vertical bar chart. Every chart in the app is this widget, and it always
/// carries a [NarrationControl] — the narration for [kind] cannot be left out.
class BarChart extends StatelessWidget {
  const BarChart({super.key, required this.kind, required this.title, required this.bars, this.height = 160});
  final String kind;
  final String title;
  final List<Bar> bars;
  final double height;

  @override
  Widget build(BuildContext context) {
    final max = bars.fold<double>(0, (m, b) => b.value.abs() > m ? b.value.abs() : m);
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(Tokens.spaceMd),
        child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Text(title, style: Tokens.title),
          const SizedBox(height: Tokens.spaceMd),
          SizedBox(
            height: height,
            child: Row(crossAxisAlignment: CrossAxisAlignment.end, children: [
              for (final b in bars)
                Expanded(
                  child: Padding(
                    padding: const EdgeInsets.symmetric(horizontal: Tokens.spaceXs),
                    child: Column(mainAxisAlignment: MainAxisAlignment.end, children: [
                      if (b.caption != null) Text(b.caption!, style: Tokens.caption, maxLines: 1, overflow: TextOverflow.fade),
                      Container(
                        height: max == 0 ? 0 : (height - 48) * b.value.abs() / max,
                        decoration: BoxDecoration(
                          color: b.value < 0 ? Tokens.error : b.color,
                          borderRadius: const BorderRadius.vertical(top: Radius.circular(Tokens.radiusSm)),
                        ),
                      ),
                      const SizedBox(height: Tokens.spaceXs),
                      Text(b.label, style: Tokens.caption, maxLines: 1, overflow: TextOverflow.ellipsis),
                    ]),
                  ),
                ),
            ]),
          ),
          NarrationControl(kind: kind),
        ]),
      ),
    );
  }
}

/// Plays the pre-recorded narration for a chart kind and shows its text.
class NarrationControl extends StatelessWidget {
  const NarrationControl({super.key, required this.kind});
  final String kind;

  String get textKey => 'narration.chart.$kind';

  @override
  Widget build(BuildContext context) {
    final narration = AppScope.of(context).narration;
    return ListenableBuilder(
      listenable: narration,
      builder: (context, _) {
        final playing = narration.playing == textKey;
        return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          TextButton.icon(
            key: Key('narrate-$kind'),
            onPressed: () => narration.toggle(textKey),
            icon: Icon(playing ? Icons.stop_circle_outlined : Icons.volume_up_outlined),
            label: Text(context.t(playing ? 'advisor.narration.stop' : 'advisor.narration.play')),
          ),
          Text(context.t(textKey), style: Tokens.caption),
          if (narration.unavailable.contains(textKey))
            Text(context.t('common.audio_unavailable'), style: Tokens.caption.copyWith(color: Tokens.textSecondary)),
        ]);
      },
    );
  }
}

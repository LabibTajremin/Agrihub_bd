import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../app/services.dart';
import '../../core/l10n/localization.dart';
import '../../core/theme/tokens.dart';
import 'voice_controller.dart';

/// Where an understood intent leads, and the button label for it.
const intentRoutes = {
  'weather': ('/home/weather', 'home.weather.title'),
  'disease': ('/doctor', 'home.quickscan.title'),
  'crop': ('/advisor', 'home.advisor.title'),
};

/// 4.1–4.4 Voice assistant: idle, listening, understanding, response,
/// not understood and help.
class VoiceScreen extends StatelessWidget {
  const VoiceScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final services = AppScope.of(context);
    final voice = services.voice;
    return ListenableBuilder(
      listenable: voice,
      builder: (context, _) => Scaffold(
        appBar: AppBar(
          title: Text(context.t('voice.title')),
          actions: [IconButton(key: const Key('voice-help'), onPressed: voice.help, icon: const Icon(Icons.help_outline))],
        ),
        body: Padding(
          padding: const EdgeInsets.all(Tokens.spaceXl),
          child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            Expanded(child: SingleChildScrollView(child: _body(context, voice))),
            _MicButton(voice: voice),
          ]),
        ),
      ),
    );
  }

  Widget _body(BuildContext context, VoiceController voice) {
    final a = voice.answer;
    return switch (voice.state) {
      VoiceState.idle => _Message(key: const Key('voice-idle'), text: context.t('voice.idle')),
      VoiceState.listening => _Message(key: const Key('voice-listening'), text: context.t('voice.listening')),
      VoiceState.understanding => Column(key: const Key('voice-understanding'), children: [
          const CircularProgressIndicator(),
          const SizedBox(height: Tokens.spaceLg),
          Text(context.t('voice.understanding'), style: Tokens.title),
        ]),
      VoiceState.response => Column(key: const Key('voice-response'), crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Text(context.t(a!.answerKey, a.params), style: Tokens.headline),
          _AnswerAudio(answerKey: a.answerKey),
          if (intentRoutes[a.intent] case (final route, final label))
            FilledButton(onPressed: () => context.go(route), child: Text(context.t(label))),
          _Suggestions(keys: a.followUps, voice: voice),
        ]),
      VoiceState.notUnderstood => Column(key: const Key('voice-not-understood'), children: [
          const Icon(Icons.hearing_disabled, size: Tokens.spaceXxxl, color: Tokens.warning),
          Text(voice.error == null ? context.t('voice.not_understood') : context.tError(voice.error!),
              style: Tokens.title, textAlign: TextAlign.center),
          _Suggestions(keys: suggestions, voice: voice),
        ]),
      VoiceState.help => Column(key: const Key('voice-help-state'), crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Text(context.t('voice.help.title'), style: Tokens.headline),
          _Suggestions(keys: suggestions, voice: voice),
        ]),
    };
  }
}

class _Message extends StatelessWidget {
  const _Message({super.key, required this.text});
  final String text;

  @override
  Widget build(BuildContext context) =>
      Padding(padding: const EdgeInsets.only(top: Tokens.spaceXxl), child: Text(text, style: Tokens.title, textAlign: TextAlign.center));
}

class _Suggestions extends StatelessWidget {
  const _Suggestions({required this.keys, required this.voice});
  final List<String> keys;
  final VoiceController voice;

  @override
  Widget build(BuildContext context) => Wrap(spacing: Tokens.spaceSm, runSpacing: Tokens.spaceSm, children: [
        for (final k in keys)
          ActionChip(
            label: Text(context.t(k)),
            onPressed: () => voice.ask(Utterance(transcript: context.t(k))),
          ),
      ]);
}

/// Plays the pre-recorded clip of the answer, when one exists.
class _AnswerAudio extends StatelessWidget {
  const _AnswerAudio({required this.answerKey});
  final String answerKey;

  @override
  Widget build(BuildContext context) {
    final narration = AppScope.of(context).narration;
    return ListenableBuilder(
      listenable: narration,
      builder: (context, _) => Align(
        alignment: AlignmentDirectional.centerStart,
        child: TextButton.icon(
          key: const Key('voice-play'),
          onPressed: () => narration.toggle(answerKey),
          icon: Icon(narration.playing == answerKey ? Icons.stop_circle_outlined : Icons.volume_up_outlined),
          label: Text(context.t(narration.unavailable.contains(answerKey)
              ? 'common.audio_unavailable'
              : narration.playing == answerKey
                  ? 'advisor.narration.stop'
                  : 'advisor.narration.play')),
        ),
      ),
    );
  }
}

class _MicButton extends StatelessWidget {
  const _MicButton({required this.voice});
  final VoiceController voice;

  @override
  Widget build(BuildContext context) {
    final listening = voice.state == VoiceState.listening;
    return Column(children: [
      SizedBox(
        width: Tokens.spaceXxxl * 2,
        height: Tokens.spaceXxxl * 2,
        child: FloatingActionButton(
          key: const Key('voice-mic'),
          heroTag: null,
          backgroundColor: listening ? Tokens.error : Tokens.primary,
          onPressed: switch (voice.state) {
            VoiceState.listening => voice.stopListening,
            VoiceState.understanding => null,
            _ => voice.talk,
          },
          child: Icon(listening ? Icons.stop : Icons.mic, size: Tokens.spaceXxl, color: Tokens.textOnPrimary),
        ),
      ),
      const SizedBox(height: Tokens.spaceSm),
      Text(context.t('voice.tap_to_talk'), style: Tokens.label),
    ]);
  }
}

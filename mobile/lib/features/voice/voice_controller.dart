import 'package:flutter/foundation.dart';

import '../../core/network/api_client.dart';

/// The voice screen's states (Figma 4.1–4.4).
enum VoiceState { idle, listening, understanding, response, notUnderstood, help }

/// What the farmer said: text, or a recording uploaded as media.
class Utterance {
  const Utterance({this.transcript = '', this.audioMediaId});
  final String transcript;
  final String? audioMediaId;
}

/// Captures one utterance. OPEN SLOT (§14): recording and speech recognition
/// are chosen later; the shipped [NoVoiceInput] captures nothing, so the
/// screen falls back to the suggested questions.
abstract interface class VoiceInput {
  Future<Utterance?> listen();
  Future<void> cancel();
}

class NoVoiceInput implements VoiceInput {
  const NoVoiceInput();
  @override
  Future<Utterance?> listen() async => null;
  @override
  Future<void> cancel() async {}
}

/// The assistant's answer: a dictionary key plus follow-up suggestion keys.
class Answer {
  const Answer({required this.intent, required this.answerKey, required this.params, required this.followUps});
  final String intent;
  final String answerKey;
  final Map<String, String> params;
  final List<String> followUps;

  factory Answer.fromJson(Map<String, Object?> j) => Answer(
      intent: j['intent'] as String? ?? '',
      answerKey: j['answer_key']! as String,
      params: ((j['params'] as Map?) ?? const {}).map((k, v) => MapEntry('$k', '$v')),
      followUps: ((j['follow_ups'] as List?) ?? const []).cast<String>());
}

/// Suggested questions shown in help and after a miss.
const suggestions = ['voice.suggestion.weather', 'voice.suggestion.disease', 'voice.suggestion.crop'];

/// Drives tap-to-talk against POST /v1/assistant/ask (the agent port's stub
/// on the server).
class VoiceController extends ChangeNotifier {
  VoiceController({required this.api, required this.input, required this.language});
  final ApiClient api;
  final VoiceInput input;
  final String Function() language;

  VoiceState state = VoiceState.idle;
  Answer? answer;
  ApiException? error;

  void _set(VoiceState s) {
    state = s;
    notifyListeners();
  }

  /// Tap-to-talk: listen, then ask. With no captured speech, show help.
  Future<void> talk() async {
    error = null;
    _set(VoiceState.listening);
    final u = await input.listen();
    if (state != VoiceState.listening) {
      return; // cancelled while listening
    }
    if (u == null) {
      _set(VoiceState.help);
      return;
    }
    await ask(u);
  }

  Future<void> stopListening() async {
    await input.cancel();
    _set(VoiceState.idle);
  }

  Future<void> ask(Utterance u) async {
    error = null;
    _set(VoiceState.understanding);
    try {
      final res = await api.post('/v1/assistant/ask', {
        'lang': language(),
        'transcript': u.transcript,
        'audio_media_id': ?u.audioMediaId,
      });
      final a = Answer.fromJson(res);
      answer = a;
      _set(res['understood'] == true ? VoiceState.response : VoiceState.notUnderstood);
    } on ApiException catch (e) {
      error = e;
      _set(VoiceState.notUnderstood);
    }
  }

  void help() => _set(VoiceState.help);

  void reset() {
    answer = null;
    error = null;
    _set(VoiceState.idle);
  }
}

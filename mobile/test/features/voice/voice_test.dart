import 'package:agrismart/app/app.dart';
import 'package:agrismart/app/router.dart';
import 'package:agrismart/features/voice/voice_controller.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../../flows/first_run.dart';
import '../../support/fakes.dart';

Future<(TestKit, ScriptedVoiceInput)> openVoice(WidgetTester tester) async {
  final input = ScriptedVoiceInput();
  final kit = (await tester.runAsync(() => TestKit.create(voiceInput: input)))!;
  await tester.runAsync(kit.onboarded);
  kit.stubAssistant();
  kit.stubHome();
  await tester.pumpWidget(AgriSmartApp(services: kit.services, router: buildRouter(initialLocation: '/voice')));
  await settle(tester);
  return (kit, input);
}

Future<void> say(WidgetTester tester, ScriptedVoiceInput input, String text) async {
  await tester.tap(find.byKey(const Key('voice-mic')));
  await tester.pump();
  expect(find.byKey(const Key('voice-listening')), findsOneWidget);
  input.pending.complete(Utterance(transcript: text));
  await settle(tester);
}

void main() {
  testWidgets('idle → listening → understanding → response, with follow-ups and navigation', (tester) async {
    final (kit, input) = await openVoice(tester);
    expect(find.text('Tap the microphone and ask a question'), findsOneWidget);
    expect(find.text('Tap to talk'), findsOneWidget);

    await tester.tap(find.byKey(const Key('voice-mic')));
    await tester.pump();
    expect(find.text('Listening…'), findsOneWidget);
    kit.server.on('POST', '/v1/assistant/ask', (_) async {
      await Future<void>.delayed(const Duration(milliseconds: 1));
      return const Reply(200, {'understood': true, 'intent': 'weather', 'answer_key': 'voice.answer.weather'});
    });
    input.pending.complete(const Utterance(transcript: 'Will it rain?'));
    await tester.pump();
    await tester.pump();
    expect(find.byKey(const Key('voice-understanding')), findsOneWidget);
    expect(tester.widget<FloatingActionButton>(find.byKey(const Key('voice-mic'))).onPressed, isNull);
    await settle(tester);
    expect(find.text('Light rain is likely in the next days. Plan spraying on dry mornings.'), findsOneWidget);
    final ask = kit.server.seen.lastWhere((r) => r.path == '/v1/assistant/ask').body! as Map;
    expect(ask, {'lang': 'en', 'transcript': 'Will it rain?'});

    kit.stubAssistant();
    await say(tester, input, 'what should I plant');
    expect(find.byKey(const Key('voice-response')), findsOneWidget);
    expect(find.text('Will it rain this week?'), findsOneWidget);
    await tapText(tester, 'Will it rain this week?');
    expect(find.text("Today's weather"), findsOneWidget);
    await tapText(tester, "Today's weather");
    expect(find.text('Next days'), findsOneWidget);
  });

  testWidgets('not understood, offline error, help and cancel', (tester) async {
    final (kit, input) = await openVoice(tester);
    await say(tester, input, 'hello there');
    expect(find.byKey(const Key('voice-not-understood')), findsOneWidget);
    expect(find.text('Sorry, I did not understand. Please try again.'), findsOneWidget);

    kit.server.offline = true;
    await tapText(tester, 'What is wrong with my rice?');
    expect(find.text('You are offline. Showing saved data.'), findsOneWidget);
    kit.server.offline = false;
    await tapText(tester, 'What is wrong with my rice?');
    expect(find.text('Take a photo of the leaf with the Plant Doctor to find out.'), findsOneWidget);
    await tapText(tester, 'Scan a leaf');
    expect(find.text('Scan leaf'), findsOneWidget);

    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(AgriSmartApp(services: kit.services, router: buildRouter(initialLocation: '/voice')));
    await settle(tester);
    await tester.tap(find.byKey(const Key('voice-help')));
    await settle(tester);
    expect(find.text('You can ask'), findsOneWidget);

    await tester.tap(find.byKey(const Key('voice-mic')));
    await tester.pump();
    await tester.tap(find.byKey(const Key('voice-mic'))); // stop while listening
    await settle(tester);
    expect(input.cancels, 1);
    expect(find.byKey(const Key('voice-idle')), findsOneWidget);

    await tester.tap(find.byKey(const Key('voice-mic')));
    await tester.pump();
    input.pending.complete(null); // nothing captured: help
    await settle(tester);
    expect(find.byKey(const Key('voice-help-state')), findsOneWidget);
    kit.services.voice.reset();
    await settle(tester);
    expect(find.byKey(const Key('voice-idle')), findsOneWidget);
  });

  testWidgets('answer audio plays the pre-recorded clip or says it is unavailable', (tester) async {
    final (kit, input) = await openVoice(tester);
    kit.stubVoice('en', ['voice.answer.crop']);
    await say(tester, input, 'what to plant');
    await tester.tap(find.byKey(const Key('voice-play')));
    await settle(tester);
    expect(kit.audio.played, hasLength(1));
    expect(find.text('Stop'), findsOneWidget);
    await say(tester, input, 'rain?');
    await tester.tap(find.byKey(const Key('voice-play')));
    await settle(tester);
    expect(find.text('Audio not available yet'), findsOneWidget);
  });

  test('no voice input captures nothing', () async {
    const input = NoVoiceInput();
    expect(await input.listen(), isNull);
    await input.cancel();
  });
}

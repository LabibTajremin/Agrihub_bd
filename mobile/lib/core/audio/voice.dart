import 'dart:async';

import 'package:audioplayers/audioplayers.dart';
import 'package:crypto/crypto.dart';
import 'package:flutter/foundation.dart';

import '../network/api_client.dart';
import '../network/cached.dart';
import '../storage/local_db.dart';

/// One pre-recorded clip in a language's voice manifest (§5.5).
class Clip {
  const Clip(this.url, this.durationMs, this.checksum);
  final String url;
  final int durationMs;
  final String checksum;
}

/// Pre-recorded voice clips: the manifest is cached like any read; clips are
/// downloaded lazily, verified and kept by checksum so they play offline.
/// Delivery only — no speech is generated here (§14).
class VoiceRepository {
  VoiceRepository({required this.reader, required this.api, required this.db});
  final CachedReader reader;
  final ApiClient api;
  final LocalDb db;

  Future<Map<String, Clip>> manifest(String lang) async {
    final m = await reader.read('voice.$lang', '/v1/voice/$lang', (j) => j);
    return {
      for (final MapEntry(:key, :value) in ((m.value['clips'] as Map?) ?? const {}).entries)
        '$key': Clip(value['url']! as String, value['duration_ms']! as int, value['checksum']! as String),
    };
  }

  /// The audio for [key] in [lang], or null when no clip exists or it cannot
  /// be fetched (offline and not yet cached, or a corrupt download).
  Future<Uint8List?> clip(String lang, String key) async {
    try {
      final c = (await manifest(lang))[key];
      if (c == null) {
        return null;
      }
      final cached = await db.blob('clip.${c.checksum}');
      if (cached != null) {
        return cached;
      }
      final bytes = await api.download(c.url);
      if (sha256.convert(bytes).toString() != c.checksum) {
        return null;
      }
      await db.putBlob('clip.${c.checksum}', bytes);
      return bytes;
    } on ApiException {
      return null;
    }
  }
}

/// Plays audio bytes.
abstract interface class AudioOut {
  Future<void> play(Uint8List bytes);
  Future<void> stop();

  /// Fires when a clip finishes on its own.
  Stream<void> get completed;
}

/// [AudioOut] over the audioplayers plugin.
class PluginAudioOut implements AudioOut {
  PluginAudioOut([AudioPlayer? player]) : _player = player;
  AudioPlayer? _player;

  // No position ticker: narration shows no progress bar, and the per-frame
  // updater would keep the UI scheduling frames while a clip plays.
  AudioPlayer get _p => _player ??= AudioPlayer()..positionUpdater = null;

  @override
  Future<void> play(Uint8List bytes) => _p.play(BytesSource(bytes, mimeType: 'audio/mpeg'));

  @override
  Future<void> stop() => _p.stop();

  @override
  Stream<void> get completed => _p.onPlayerComplete;
}

/// Which chart narration is playing; one at a time across the app.
class NarrationController extends ChangeNotifier {
  NarrationController({required this.voice, required this.out, required this.language});
  final VoiceRepository voice;
  final AudioOut out;
  final String Function() language;
  StreamSubscription<void>? _done;

  String? playing;

  /// Keys whose clip could not be found; the control says so.
  final unavailable = <String>{};

  /// Starts [key]'s clip, or stops it when it is already playing.
  Future<void> toggle(String key) async {
    if (playing == key) {
      await stop();
      return;
    }
    await stop();
    final bytes = await voice.clip(language(), key);
    if (bytes == null) {
      unavailable.add(key);
      notifyListeners();
      return;
    }
    _done ??= out.completed.listen((_) {
      playing = null;
      notifyListeners();
    });
    playing = key;
    notifyListeners();
    await out.play(bytes);
  }

  Future<void> stop() async {
    if (playing != null) {
      playing = null;
      notifyListeners();
      await out.stop();
    }
  }

  @override
  void dispose() {
    _done?.cancel();
    super.dispose();
  }
}

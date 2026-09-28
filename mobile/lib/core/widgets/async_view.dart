import 'package:flutter/material.dart';

import '../l10n/localization.dart';
import '../network/api_client.dart';
import '../theme/tokens.dart';

/// Loads a value and renders loading, error (with retry) and data states.
class AsyncView<T> extends StatefulWidget {
  const AsyncView({super.key, required this.load, required this.builder});
  final Future<T> Function() load;
  final Widget Function(BuildContext context, T value, Future<void> Function() reload) builder;

  @override
  State<AsyncView<T>> createState() => _AsyncViewState<T>();
}

class _AsyncViewState<T> extends State<AsyncView<T>> {
  T? _value;
  ApiException? _error;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  Future<void> _reload() async {
    setState(() => _loading = true);
    try {
      final v = await widget.load();
      _value = v;
      _error = null;
    } on ApiException catch (e) {
      _error = e;
    }
    if (mounted) {
      setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading && _value == null) {
      return const Center(child: CircularProgressIndicator());
    }
    final error = _error;
    if (error != null) {
      return ErrorPanel(message: context.tError(error), onRetry: _reload);
    }
    return widget.builder(context, _value as T, _reload);
  }
}

/// A full-area error with a retry button.
class ErrorPanel extends StatelessWidget {
  const ErrorPanel({super.key, required this.message, required this.onRetry});
  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(Tokens.spaceXl),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.cloud_off, size: Tokens.spaceXxxl, color: Tokens.textSecondary),
              const SizedBox(height: Tokens.spaceMd),
              Text(message, style: Tokens.body, textAlign: TextAlign.center),
              const SizedBox(height: Tokens.spaceLg),
              OutlinedButton(onPressed: onRetry, child: Text(context.t('common.retry'))),
            ],
          ),
        ),
      );
}

/// A centred empty-state message.
class EmptyState extends StatelessWidget {
  const EmptyState({super.key, required this.message, this.icon = Icons.inbox_outlined});
  final String message;
  final IconData icon;

  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(Tokens.spaceXl),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            Icon(icon, size: Tokens.spaceXxxl, color: Tokens.textSecondary),
            const SizedBox(height: Tokens.spaceMd),
            Text(message, style: Tokens.body.copyWith(color: Tokens.textSecondary), textAlign: TextAlign.center),
          ]),
        ),
      );
}

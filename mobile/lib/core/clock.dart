import 'package:uuid/uuid.dart';

/// Injected time source; tests use [FixedClock].
abstract interface class Clock {
  DateTime now();
}

class SystemClock implements Clock {
  const SystemClock();
  @override
  DateTime now() => DateTime.now().toUtc();
}

class FixedClock implements Clock {
  FixedClock(this.current);
  DateTime current;
  @override
  DateTime now() => current;
  void advance(Duration d) => current = current.add(d);
}

/// Time-ordered UUIDv7 identifiers (idempotency keys, local IDs).
abstract interface class IdGenerator {
  String next();
}

class UuidV7Generator implements IdGenerator {
  UuidV7Generator([Uuid? uuid]) : _uuid = uuid ?? const Uuid();
  final Uuid _uuid;
  @override
  String next() => _uuid.v7();
}

class SequenceIdGenerator implements IdGenerator {
  int _n = 0;
  @override
  String next() => '00000000-0000-7000-8000-${(++_n).toRadixString(16).padLeft(12, '0')}';
}

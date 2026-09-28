// Coverage gate for the Flutter app: parses coverage/lcov.info, drops the
// permitted generated-code exclusions and fails below the threshold.
import 'dart:io';

const excluded = <String>['.g.dart', '.freezed.dart', 'l10n/generated/'];

class FileCoverage {
  FileCoverage(this.path);
  final String path;
  final Map<int, int> lines = {};
  int get total => lines.length;
  int get hit => lines.values.where((c) => c > 0).length;
}

List<FileCoverage> parseLcov(String content) {
  final files = <FileCoverage>[];
  FileCoverage? current;
  for (final raw in content.split('\n')) {
    final line = raw.trim();
    if (line.startsWith('SF:')) {
      current = FileCoverage(line.substring(3));
      files.add(current);
    } else if (line.startsWith('DA:') && current != null) {
      final parts = line.substring(3).split(',');
      final n = int.parse(parts[0]);
      final c = int.parse(parts[1]);
      current.lines[n] = (current.lines[n] ?? 0) + c;
    } else if (line == 'end_of_record') {
      current = null;
    }
  }
  return files;
}

bool isExcluded(String path) => excluded.any(path.contains);

void main(List<String> args) {
  final path = args.isNotEmpty ? args[0] : 'coverage/lcov.info';
  final threshold = args.length > 1 ? double.parse(args[1]) : 100.0;
  final files = parseLcov(File(path).readAsStringSync())
      .where((f) => !isExcluded(f.path))
      .toList();
  var total = 0;
  var hit = 0;
  final failing = <String>[];
  for (final f in files) {
    total += f.total;
    hit += f.hit;
    if (f.hit < f.total) {
      final missed = f.lines.entries.where((e) => e.value == 0).map((e) => e.key).toList()..sort();
      failing.add('FAIL ${f.path} ${f.hit}/${f.total} missed lines: ${missed.join(',')}');
    }
  }
  final pct = total == 0 ? 100.0 : hit * 100 / total;
  failing.forEach(stdout.writeln);
  stdout.writeln('coverage: ${pct.toStringAsFixed(2)}% of $total lines in ${files.length} files (gate $threshold%)');
  if (pct < threshold) {
    exitCode = 1;
  }
}

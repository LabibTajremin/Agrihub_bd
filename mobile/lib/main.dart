import 'package:flutter/material.dart';

void main() => runApp(const AgriSmartApp());

class AgriSmartApp extends StatelessWidget {
  const AgriSmartApp({super.key});

  @override
  Widget build(BuildContext context) {
    return const MaterialApp(
      title: 'AgriSmart',
      home: Scaffold(body: Center(child: Text('AgriSmart'))),
    );
  }
}

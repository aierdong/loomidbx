import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'screens/home_screen.dart';

void main() {
  runApp(const ProviderScope(child: LoomiDBXApp()));
}

class LoomiDBXApp extends StatelessWidget {
  const LoomiDBXApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'LoomiDBX',
      theme: ThemeData(
        primarySwatch: Colors.blue,
        colorScheme: const ColorScheme.light(
          primary: Color(0xFF2563EB),
          secondary: Color(0xFF64748B),
          error: Color(0xFFEF4444),
        ),
      ),
      home: const HomeScreen(),
    );
  }
}

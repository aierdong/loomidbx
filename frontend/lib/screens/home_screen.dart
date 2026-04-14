import 'package:flutter/material.dart';

/// Placeholder home until main layout (sidebar / workspace) is implemented.
class HomeScreen extends StatelessWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('LoomiDBX')),
      body: const Center(
        child: Text('LoomiDBX — 项目骨架已就绪'),
      ),
    );
  }
}

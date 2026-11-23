import 'package:flutter/material.dart';

import '../api/agentforge_api.dart';
import 'agent_screen.dart';
import 'memory_screen.dart';
import 'status_screen.dart';
import 'timeline_screen.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({
    super.key,
    required this.api,
    required this.onOpenSettings,
  });

  final AgentForgeApi api;
  final VoidCallback onOpenSettings;

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  int _index = 0;

  @override
  Widget build(BuildContext context) {
    final screens = [
      AgentScreen(api: widget.api),
      MemoryScreen(api: widget.api),
      TimelineScreen(api: widget.api),
      StatusScreen(api: widget.api),
    ];

    return Scaffold(
      body: Row(
        children: [
          NavigationRail(
            selectedIndex: _index,
            onDestinationSelected: (value) => setState(() => _index = value),
            labelType: NavigationRailLabelType.all,
            destinations: const [
              NavigationRailDestination(
                icon: Icon(Icons.chat_bubble_outline),
                selectedIcon: Icon(Icons.chat_bubble),
                label: Text('对话'),
              ),
              NavigationRailDestination(
                icon: Icon(Icons.memory_outlined),
                selectedIcon: Icon(Icons.memory),
                label: Text('记忆'),
              ),
              NavigationRailDestination(
                icon: Icon(Icons.timeline_outlined),
                selectedIcon: Icon(Icons.timeline),
                label: Text('时间线'),
              ),
              NavigationRailDestination(
                icon: Icon(Icons.monitor_heart_outlined),
                selectedIcon: Icon(Icons.monitor_heart),
                label: Text('状态'),
              ),
            ],
          ),
          const VerticalDivider(width: 1),
          Expanded(
            child: screens[_index],
          ),
        ],
      ),
      floatingActionButton: _index == 0
          ? null
          : FloatingActionButton.small(
              onPressed: widget.onOpenSettings,
              tooltip: '连接设置',
              child: const Icon(Icons.settings),
            ),
    );
  }
}

import 'package:agentforge_gui/models/agent_event.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('AgentEvent parses a plan event', () {
    final event = AgentEvent.fromJson(const {
      'type': 'plan',
      'actions': [
        {'id': 'action-0', 'type': 'run_command', 'status': 'pending'},
      ],
    });
    expect(event.type, 'plan');
    expect(event.actions, hasLength(1));
    expect(event.action, isNull);
  });

  test('AgentEvent parses an error event', () {
    final event = AgentEvent.fromJson(const {
      'type': 'error',
      'error': 'boom',
    });
    expect(event.type, 'error');
    expect(event.error, 'boom');
  });
}

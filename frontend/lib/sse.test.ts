/**
 * @jest-environment jsdom
 */
import { renderHook, act, cleanup } from '@testing-library/react';
import { useSSE } from './sse';

// Mock EventSource
class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  listeners: Record<string, ((e: { data: string }) => void)[]> = {};
  closed = false;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
    // Trigger onopen async
    setTimeout(() => this.onopen?.(), 0);
  }

  addEventListener(type: string, handler: (e: { data: string }) => void) {
    if (!this.listeners[type]) this.listeners[type] = [];
    this.listeners[type].push(handler);
  }

  close() {
    this.closed = true;
  }

  // Test helper: simulate receiving an event
  emit(type: string, data: object) {
    for (const handler of this.listeners[type] || []) {
      handler({ data: JSON.stringify(data) });
    }
  }
}

beforeAll(() => {
  (global as unknown as Record<string, unknown>).EventSource = MockEventSource;
});

afterEach(() => {
  MockEventSource.instances = [];
  cleanup();
});

test('useSSE connects and receives typed events', async () => {
  const { result } = renderHook(() => useSSE('exec-123'));

  // Should have created an EventSource
  expect(MockEventSource.instances).toHaveLength(1);
  const es = MockEventSource.instances[0];
  expect(es.url).toContain('executionId=exec-123');

  // Simulate an event
  await act(async () => {
    es.emit('AgentStarted', { type: 'AgentStarted', execution_id: 'exec-123', agent_id: 'agent-1' });
  });

  expect(result.current.events).toHaveLength(1);
  expect(result.current.events[0].type).toBe('AgentStarted');
  expect(result.current.events[0].agent_id).toBe('agent-1');
});

test('useSSE closes EventSource on unmount', () => {
  const { unmount } = renderHook(() => useSSE('exec-456'));
  const es = MockEventSource.instances[0];
  expect(es.closed).toBe(false);

  unmount();
  expect(es.closed).toBe(true);
});

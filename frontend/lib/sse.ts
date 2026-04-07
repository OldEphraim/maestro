'use client';

import { useEffect, useRef, useState, useCallback } from 'react';

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

export interface SSEEvent {
  type: string;
  execution_id?: string;
  agent_id?: string;
  from?: string;
  to?: string;
  payload?: unknown;
}

export function useSSE(executionId?: string) {
  const [events, setEvents] = useState<SSEEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const esRef = useRef<EventSource | null>(null);

  const clearEvents = useCallback(() => setEvents([]), []);

  useEffect(() => {
    const url = executionId
      ? `${API_URL}/api/events?executionId=${executionId}`
      : `${API_URL}/api/events`;

    const es = new EventSource(url);
    esRef.current = es;

    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);

    const eventTypes = [
      'ExecutionStarted', 'AgentStarted', 'AgentCompleted',
      'MessageDispatched', 'WhatsAppSent', 'ExecutionCompleted',
      'ExecutionFailed', 'StepTimedOut', 'ExternalMessageReceived',
    ];

    for (const type of eventTypes) {
      es.addEventListener(type, (e: MessageEvent) => {
        try {
          const data: SSEEvent = JSON.parse(e.data);
          setEvents(prev => [...prev, data]);
        } catch {
          // ignore parse errors
        }
      });
    }

    return () => {
      es.close();
      esRef.current = null;
      setConnected(false);
    };
  }, [executionId]);

  return { events, connected, clearEvents };
}

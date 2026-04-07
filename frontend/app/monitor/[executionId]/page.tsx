'use client';

import { useEffect, useState, useRef } from 'react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import { ArrowLeft, MessageSquare, Clock, CheckCircle, XCircle, Loader, AlertTriangle, DollarSign } from 'lucide-react';
import { getExecution, getMessages, listAgents, type Execution, type Message, type Agent } from '@/lib/api';
import { useSSE, type SSEEvent } from '@/lib/sse';

const statusIcon: Record<string, React.ReactNode> = {
  idle: <Clock size={14} className="text-slate-400" />,
  running: <Loader size={14} className="text-blue-400 animate-spin" />,
  completed: <CheckCircle size={14} className="text-green-400" />,
  failed: <XCircle size={14} className="text-red-400" />,
  timed_out: <AlertTriangle size={14} className="text-yellow-400" />,
};

const statusColor: Record<string, string> = {
  idle: 'bg-slate-700 text-slate-300',
  running: 'bg-blue-900 text-blue-300',
  completed: 'bg-green-900 text-green-300',
  failed: 'bg-red-900 text-red-300',
  timed_out: 'bg-yellow-900 text-yellow-300',
};

export default function MonitorPage() {
  const params = useParams();
  const executionId = params.executionId as string;
  const [execution, setExecution] = useState<Execution | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [agentMap, setAgentMap] = useState<Record<string, Agent>>({});
  const [agentStatuses, setAgentStatuses] = useState<Record<string, string>>({});
  const [elapsed, setElapsed] = useState(0);
  const { events } = useSSE(executionId);
  const timelineRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    listAgents().then(agents => {
      const m: Record<string, Agent> = {};
      agents.forEach(a => { m[a.id] = a; });
      setAgentMap(m);
    });
  }, []);

  useEffect(() => {
    getExecution(executionId).then(setExecution).catch(console.error);
  }, [executionId]);

  // Poll messages periodically
  useEffect(() => {
    const load = () => getMessages(executionId).then(setMessages).catch(() => {});
    load();
    const interval = setInterval(load, 3000);
    return () => clearInterval(interval);
  }, [executionId]);

  // Update agent statuses from SSE events
  useEffect(() => {
    for (const evt of events) {
      if (evt.type === 'AgentStarted' && evt.agent_id) {
        setAgentStatuses(prev => ({ ...prev, [evt.agent_id!]: 'running' }));
      }
      if (evt.type === 'AgentCompleted' && evt.agent_id) {
        setAgentStatuses(prev => ({ ...prev, [evt.agent_id!]: 'completed' }));
      }
      if (evt.type === 'ExecutionCompleted' || evt.type === 'ExecutionFailed') {
        getExecution(executionId).then(setExecution).catch(() => {});
        getMessages(executionId).then(setMessages).catch(() => {});
      }
      if (evt.type === 'StepTimedOut' && evt.agent_id) {
        setAgentStatuses(prev => ({ ...prev, [evt.agent_id!]: 'timed_out' }));
      }
    }
  }, [events, executionId]);

  // Elapsed timer
  useEffect(() => {
    if (!execution) return;
    if (execution.status !== 'running') {
      if (execution.completed_at && execution.started_at) {
        setElapsed(Math.round((new Date(execution.completed_at).getTime() - new Date(execution.started_at).getTime()) / 1000));
      }
      return;
    }
    const start = new Date(execution.started_at).getTime();
    const interval = setInterval(() => setElapsed(Math.round((Date.now() - start) / 1000)), 1000);
    return () => clearInterval(interval);
  }, [execution]);

  // Auto-scroll timeline
  useEffect(() => {
    if (timelineRef.current) {
      timelineRef.current.scrollTop = timelineRef.current.scrollHeight;
    }
  }, [events, messages]);

  const uniqueAgentIds = [...new Set(events.filter(e => e.agent_id).map(e => e.agent_id!))];

  return (
    <div className="flex flex-col h-[calc(100vh-52px)]">
      {/* Top bar */}
      <div className="flex items-center justify-between px-4 py-2 bg-slate-900 border-b border-slate-700">
        <div className="flex items-center gap-3">
          <Link href="/workflows" className="text-slate-400 hover:text-white"><ArrowLeft size={16} /></Link>
          <span className="text-white font-medium">Execution</span>
          <span className={`text-xs px-2 py-0.5 rounded ${statusColor[execution?.status || 'running']}`}>
            {execution?.status || 'running'}
          </span>
          <span className="text-xs text-slate-400 flex items-center gap-1">
            <Clock size={12} /> {elapsed}s
          </span>
        </div>
        <span className="text-xs text-slate-500 font-mono">{executionId.slice(0, 8)}...</span>
      </div>

      <div className="flex flex-1 overflow-hidden">
        {/* Left panel — agent status cards */}
        <div className="w-1/3 border-r border-slate-700 p-4 overflow-y-auto">
          <h3 className="text-sm font-medium text-slate-400 mb-3">Agent Status</h3>
          <div className="space-y-2">
            {uniqueAgentIds.map(agentId => {
              const agent = agentMap[agentId];
              const status = agentStatuses[agentId] || 'idle';
              return (
                <div key={agentId} className="bg-slate-800 border border-slate-700 rounded-lg p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-white">{agent?.name || agentId.slice(0, 8)}</span>
                    <div className="flex items-center gap-1">
                      {statusIcon[status]}
                      <span className={`text-xs px-1.5 py-0.5 rounded ${statusColor[status]}`}>{status}</span>
                    </div>
                  </div>
                  {agent?.role && <span className="text-xs text-slate-500">{agent.role}</span>}
                </div>
              );
            })}
            {uniqueAgentIds.length === 0 && (
              <p className="text-sm text-slate-500">Waiting for agents to start...</p>
            )}
          </div>

          {/* Cost summary */}
          {execution?.cost_summary && (execution.cost_summary.total_tokens_in > 0 || execution.cost_summary.total_tokens_out > 0) && (
            <div className="mt-4">
              <h3 className="text-sm font-medium text-slate-400 mb-3 flex items-center gap-1"><DollarSign size={14} /> Cost Summary</h3>
              <div className="bg-slate-800 border border-slate-700 rounded-lg p-3 space-y-2">
                <div className="flex justify-between text-sm">
                  <span className="text-slate-400">Tokens In</span>
                  <span className="text-white font-mono">{execution.cost_summary.total_tokens_in.toLocaleString()}</span>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-slate-400">Tokens Out</span>
                  <span className="text-white font-mono">{execution.cost_summary.total_tokens_out.toLocaleString()}</span>
                </div>
                <div className="border-t border-slate-700 pt-2 flex justify-between text-sm">
                  <span className="text-slate-400">Estimated Cost</span>
                  <span className="text-green-400 font-mono">${execution.cost_summary.total_cost_usd.toFixed(4)}</span>
                </div>
                {execution.cost_summary.agent_breakdown && execution.cost_summary.agent_breakdown.length > 1 && (
                  <div className="border-t border-slate-700 pt-2 space-y-1">
                    <span className="text-xs text-slate-500">Per agent</span>
                    {execution.cost_summary.agent_breakdown.map((ac, i) => (
                      <div key={i} className="flex justify-between text-xs">
                        <span className="text-slate-400">{agentMap[ac.agent_id]?.name || ac.agent_id.slice(0, 8)}</span>
                        <span className="text-slate-300 font-mono">{(ac.tokens_in + ac.tokens_out).toLocaleString()} tok / ${ac.cost_usd.toFixed(4)}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>

        {/* Right panel — event timeline */}
        <div ref={timelineRef} className="flex-1 p-4 overflow-y-auto">
          <h3 className="text-sm font-medium text-slate-400 mb-3">Event Timeline</h3>
          <div className="space-y-2">
            {events.map((evt, i) => (
              <div key={i} className="bg-slate-800 border border-slate-700 rounded px-3 py-2 text-sm">
                <div className="flex items-center gap-2 mb-1">
                  <span className={`text-xs font-mono px-1.5 py-0.5 rounded ${
                    evt.type.includes('Failed') || evt.type.includes('TimedOut') ? 'bg-red-900 text-red-300' :
                    evt.type.includes('Completed') ? 'bg-green-900 text-green-300' :
                    evt.type.includes('WhatsApp') ? 'bg-green-900 text-green-300' :
                    evt.type.includes('External') ? 'bg-purple-900 text-purple-300' :
                    'bg-slate-700 text-slate-300'
                  }`}>{evt.type}</span>
                  {evt.agent_id && <span className="text-xs text-slate-500">{agentMap[evt.agent_id]?.name || evt.agent_id.slice(0, 8)}</span>}
                </div>
                {evt.type === 'WhatsAppSent' && <span className="text-xs text-green-400 flex items-center gap-1"><MessageSquare size={10} /> {evt.to}</span>}
                {evt.type === 'ExternalMessageReceived' && <span className="text-xs text-purple-400 flex items-center gap-1"><MessageSquare size={10} /> From: {evt.from}</span>}
                {evt.type === 'MessageDispatched' && <span className="text-xs text-slate-400">{agentMap[evt.from!]?.name || 'Agent'} → {agentMap[evt.to!]?.name || 'Agent'}</span>}
                {evt.type === 'ExecutionFailed' && <span className="text-xs text-red-400">{String(evt.payload)}</span>}
              </div>
            ))}

            {/* Inter-agent messages */}
            {messages.length > 0 && (
              <>
                <h4 className="text-xs text-slate-500 mt-4 mb-2 uppercase tracking-wide">Agent Messages</h4>
                {messages.map(msg => (
                  <div key={msg.id} className="bg-slate-800 border border-slate-700 rounded px-3 py-2">
                    <div className="flex items-center gap-2 mb-1">
                      <span className="text-xs font-medium text-blue-400">{agentMap[msg.from_agent_id]?.name || 'Agent'}</span>
                      {msg.channel === 'whatsapp' && <MessageSquare size={10} className="text-green-400" />}
                      <span className="text-xs text-slate-500">{msg.channel}</span>
                    </div>
                    <pre className="text-xs text-slate-300 whitespace-pre-wrap max-h-40 overflow-y-auto">{msg.content.slice(0, 500)}{msg.content.length > 500 ? '...' : ''}</pre>
                  </div>
                ))}
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

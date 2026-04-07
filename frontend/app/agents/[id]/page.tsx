'use client';

import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import { Agent, getAgent } from '@/lib/api';
import AgentModal from '@/components/agents/AgentModal';
import { ArrowLeft, MessageSquare } from 'lucide-react';

export default function AgentDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const [agent, setAgent] = useState<Agent | null>(null);
  const [editOpen, setEditOpen] = useState(false);

  const load = () => getAgent(id).then(setAgent).catch(console.error);
  useEffect(() => { load(); }, [id]);

  if (!agent) return <div className="p-6 text-slate-400">Loading...</div>;

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <Link href="/agents" className="flex items-center gap-1 text-sm text-slate-400 hover:text-white mb-4">
        <ArrowLeft size={14} /> Back to agents
      </Link>

      <div className="bg-slate-800 border border-slate-700 rounded-lg p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h1 className="text-2xl font-bold text-white">{agent.name}</h1>
            <span className="text-sm text-slate-400 bg-slate-900 px-2 py-0.5 rounded">{agent.role}</span>
          </div>
          <button onClick={() => setEditOpen(true)} className="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-white rounded text-sm">Edit</button>
        </div>

        <div className="grid grid-cols-2 gap-4 text-sm">
          <div>
            <label className="text-slate-400">Model</label>
            <p className="text-white font-mono">{agent.model}</p>
          </div>
          <div>
            <label className="text-slate-400">Channels</label>
            <div className="flex gap-1 mt-1">
              {agent.channels?.map(ch => (
                <span key={ch} className="bg-slate-700 px-2 py-0.5 rounded flex items-center gap-1">
                  {ch === 'whatsapp' && <MessageSquare size={10} />}{ch}
                </span>
              ))}
            </div>
          </div>
        </div>

        <div className="mt-4">
          <label className="text-sm text-slate-400">System Prompt</label>
          <pre className="mt-1 bg-slate-900 p-3 rounded text-sm text-slate-300 whitespace-pre-wrap max-h-60 overflow-y-auto">{agent.system_prompt}</pre>
        </div>

        {agent.memory && Object.keys(agent.memory).length > 0 && (
          <div className="mt-4">
            <label className="text-sm text-slate-400">Memory</label>
            <div className="mt-1 space-y-1">
              {Object.entries(agent.memory).map(([k, v]) => (
                <div key={k} className="flex gap-2 text-sm">
                  <span className="text-blue-400 font-mono">{k}:</span>
                  <span className="text-slate-300">{v}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      <AgentModal agent={agent} open={editOpen} onClose={() => setEditOpen(false)} onSaved={load} />
    </div>
  );
}

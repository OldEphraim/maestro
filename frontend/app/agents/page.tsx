'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { Agent, listAgents, deleteAgent } from '@/lib/api';
import AgentModal from '@/components/agents/AgentModal';
import { Plus, Trash2, MessageSquare } from 'lucide-react';

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [modalOpen, setModalOpen] = useState(false);
  const [editAgent, setEditAgent] = useState<Agent | undefined>();

  const load = () => listAgents().then(setAgents).catch(console.error);
  useEffect(() => { load(); }, []);

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this agent?')) return;
    await deleteAgent(id);
    load();
  };

  return (
    <div className="p-6 max-w-6xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-white">Agents</h1>
        <button onClick={() => { setEditAgent(undefined); setModalOpen(true); }} className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg text-sm">
          <Plus size={16} /> New Agent
        </button>
      </div>

      {agents.length === 0 ? (
        <p className="text-slate-500 text-center py-12">No agents yet. Create one or load a template.</p>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {agents.map(agent => (
            <div key={agent.id} className="bg-slate-800 border border-slate-700 rounded-lg p-4 flex flex-col gap-2">
              <div className="flex items-center justify-between">
                <Link href={`/agents/${agent.id}`} className="text-white font-medium hover:text-blue-400">{agent.name}</Link>
                <div className="flex gap-1">
                  <button onClick={() => { setEditAgent(agent); setModalOpen(true); }} className="text-slate-400 hover:text-white p-1 text-xs">Edit</button>
                  <button onClick={() => handleDelete(agent.id)} className="text-slate-400 hover:text-red-400 p-1"><Trash2 size={14} /></button>
                </div>
              </div>
              <span className="text-xs text-slate-400 bg-slate-900 px-2 py-0.5 rounded w-fit">{agent.role}</span>
              <p className="text-xs text-slate-500 line-clamp-2">{agent.system_prompt}</p>
              <div className="flex gap-1 mt-1">
                {agent.channels?.map(ch => (
                  <span key={ch} className="text-xs bg-slate-700 px-2 py-0.5 rounded flex items-center gap-1">
                    {ch === 'whatsapp' && <MessageSquare size={10} />}{ch}
                  </span>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      <AgentModal agent={editAgent} open={modalOpen} onClose={() => setModalOpen(false)} onSaved={load} />
    </div>
  );
}

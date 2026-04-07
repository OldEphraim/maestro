'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { Workflow, listWorkflows, createWorkflow, deleteWorkflow } from '@/lib/api';
import { Plus, Trash2, LayoutGrid } from 'lucide-react';

export default function WorkflowsPage() {
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const router = useRouter();

  const load = () => listWorkflows().then(setWorkflows).catch(console.error);
  useEffect(() => { load(); }, []);

  const handleNew = async () => {
    const wf = await createWorkflow({ name: 'Untitled Workflow', status: 'draft' });
    router.push(`/workflows/${wf.id}`);
  };

  return (
    <div className="p-6 max-w-6xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-white">Workflows</h1>
        <div className="flex gap-2">
          <Link href="/templates" className="flex items-center gap-2 px-4 py-2 bg-slate-700 hover:bg-slate-600 text-white rounded-lg text-sm">
            <LayoutGrid size={16} /> Browse Templates
          </Link>
          <button onClick={handleNew} className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg text-sm">
            <Plus size={16} /> New Workflow
          </button>
        </div>
      </div>

      {workflows.length === 0 ? (
        <p className="text-slate-500 text-center py-12">No workflows yet. Create one or load a template.</p>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {workflows.map(wf => (
            <div key={wf.id} className="bg-slate-800 border border-slate-700 rounded-lg p-4">
              <div className="flex items-center justify-between mb-2">
                <Link href={`/workflows/${wf.id}`} className="text-white font-medium hover:text-blue-400">{wf.name}</Link>
                <button onClick={async () => { await deleteWorkflow(wf.id); load(); }} className="text-slate-400 hover:text-red-400"><Trash2 size={14} /></button>
              </div>
              <div className="flex gap-2 text-xs">
                <span className={`px-2 py-0.5 rounded ${wf.status === 'active' ? 'bg-green-900 text-green-300' : 'bg-slate-700 text-slate-400'}`}>{wf.status}</span>
                {wf.template_id && <span className="bg-slate-700 text-slate-400 px-2 py-0.5 rounded">{wf.template_id}</span>}
              </div>
              {wf.description && <p className="text-xs text-slate-500 mt-2 line-clamp-2">{wf.description}</p>}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { loadTemplate } from '@/lib/api';
import { Loader, Zap, Shield } from 'lucide-react';

const templates = [
  {
    id: 'nova-recovery',
    name: 'Failed Transaction Recovery Pipeline (NOVA)',
    icon: <Zap size={24} className="text-yellow-400" />,
    description: "A miniaturized version of Yuno's NOVA product. A Transaction Monitor polls for failed transactions, a Recovery Orchestrator contacts customers via WhatsApp, and a Reconciliation Reporter summarizes outcomes.",
  },
  {
    id: 'connector-integration',
    name: 'Payment Connector Integration Pipeline',
    icon: <Shield size={24} className="text-blue-400" />,
    description: "Mirrors Yuno's PSP connector onboarding workflow: a Scout researches the API, a Builder generates the Go adapter, a Compliance Reviewer checks for PCI DSS gaps. The Reviewer's feedback loops back to the Builder until the adapter passes review.",
  },
];

export default function TemplatesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<string | null>(null);

  const handleLoad = async (id: string) => {
    setLoading(id);
    try {
      const result = await loadTemplate(id);
      router.push(`/workflows/${result.workflow_id}`);
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to load template');
      setLoading(null);
    }
  };

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <h1 className="text-2xl font-bold text-white mb-2">Workflow Templates</h1>
      <p className="text-slate-400 mb-6">Pre-built workflow templates inspired by Yuno&apos;s payment orchestration challenges.</p>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {templates.map(t => (
          <div key={t.id} className="bg-slate-800 border border-slate-700 rounded-lg p-6 flex flex-col">
            <div className="flex items-center gap-3 mb-3">
              {t.icon}
              <h2 className="text-lg font-semibold text-white">{t.name}</h2>
            </div>
            <p className="text-sm text-slate-400 flex-1 mb-4">{t.description}</p>
            <button
              onClick={() => handleLoad(t.id)}
              disabled={loading === t.id}
              className="flex items-center justify-center gap-2 w-full px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white rounded-lg text-sm"
            >
              {loading === t.id ? <><Loader size={14} className="animate-spin" /> Loading...</> : 'Load Template'}
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}

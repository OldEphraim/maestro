'use client';

import { useState, useEffect } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import * as Tabs from '@radix-ui/react-tabs';
import { X, Trash2 } from 'lucide-react';
import { Agent, Schedule, createAgent, updateAgent, setMemory, deleteMemoryKey, getSchedules, createSchedule, deleteSchedule, toggleSchedule } from '@/lib/api';

interface Props {
  agent?: Agent;
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
}

export default function AgentModal({ agent, open, onClose, onSaved }: Props) {
  const isEdit = !!agent;
  const [name, setName] = useState(agent?.name || '');
  const [role, setRole] = useState(agent?.role || '');
  const [systemPrompt, setSystemPrompt] = useState(agent?.system_prompt || '');
  const [model, setModel] = useState(agent?.model || 'claude-sonnet-4-5-20250929');
  const [channels, setChannels] = useState(agent?.channels?.join(', ') || 'internal');
  const [maxTokens, setMaxTokens] = useState(agent?.guardrails?.max_tokens_per_run || 0);
  const [maxRuns, setMaxRuns] = useState(agent?.guardrails?.max_runs_per_hour || 0);
  const [memEntries, setMemEntries] = useState<{ key: string; value: string }[]>(
    agent?.memory ? Object.entries(agent.memory).map(([k, v]) => ({ key: k, value: v })) : []
  );
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [newCronExpr, setNewCronExpr] = useState('');
  const [newTaskPrompt, setNewTaskPrompt] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (isEdit && agent) {
      getSchedules(agent.id).then(setSchedules).catch(() => {});
    }
  }, [isEdit, agent]);

  const handleAddSchedule = async () => {
    if (!agent || !newCronExpr || !newTaskPrompt) return;
    setSaving(true);
    try {
      const sch = await createSchedule(agent.id, newCronExpr, newTaskPrompt);
      setSchedules(prev => [...prev, sch]);
      setNewCronExpr('');
      setNewTaskPrompt('');
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to create schedule');
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteSchedule = async (scheduleId: string) => {
    if (!agent) return;
    try {
      await deleteSchedule(agent.id, scheduleId);
      setSchedules(prev => prev.filter(s => s.id !== scheduleId));
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to delete schedule');
    }
  };

  const handleToggleSchedule = async (scheduleId: string, enabled: boolean) => {
    if (!agent) return;
    try {
      await toggleSchedule(agent.id, scheduleId, enabled);
      setSchedules(prev => prev.map(s => s.id === scheduleId ? { ...s, enabled } : s));
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to toggle schedule');
    }
  };

  const handleSaveBasic = async () => {
    setSaving(true);
    try {
      const data = {
        name, role, system_prompt: systemPrompt, model,
        tools: [] as string[],
        channels: channels.split(',').map(c => c.trim()).filter(Boolean),
        guardrails: { max_tokens_per_run: maxTokens, max_runs_per_hour: maxRuns },
      };
      if (isEdit && agent) {
        await updateAgent(agent.id, data);
      } else {
        await createAgent(data);
      }
      onSaved();
      onClose();
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const handleSaveMemory = async () => {
    if (!agent) return;
    setSaving(true);
    try {
      const existingKeys = agent.memory ? Object.keys(agent.memory) : [];
      const newKeys = memEntries.map(e => e.key);
      for (const k of existingKeys) {
        if (!newKeys.includes(k)) await deleteMemoryKey(agent.id, k);
      }
      for (const entry of memEntries) {
        if (entry.key && entry.value) await setMemory(agent.id, entry.key, entry.value);
      }
      onSaved();
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to save memory');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog.Root open={open} onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60" />
        <Dialog.Content className="fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 bg-slate-800 rounded-xl p-6 w-[600px] max-h-[80vh] overflow-y-auto border border-slate-700">
          <div className="flex items-center justify-between mb-4">
            <Dialog.Title className="text-lg font-semibold text-white">
              {isEdit ? `Edit ${agent.name}` : 'New Agent'}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button className="text-slate-400 hover:text-white"><X size={18} /></button>
            </Dialog.Close>
          </div>

          <Tabs.Root defaultValue="basic">
            <Tabs.List className="flex gap-1 mb-4 border-b border-slate-700">
              <Tabs.Trigger value="basic" className="px-4 py-2 text-sm text-slate-400 data-[state=active]:text-white data-[state=active]:border-b-2 data-[state=active]:border-blue-500">Basic</Tabs.Trigger>
              <Tabs.Trigger value="memory" className="px-4 py-2 text-sm text-slate-400 data-[state=active]:text-white data-[state=active]:border-b-2 data-[state=active]:border-blue-500">Memory</Tabs.Trigger>
              <Tabs.Trigger value="guardrails" className="px-4 py-2 text-sm text-slate-400 data-[state=active]:text-white data-[state=active]:border-b-2 data-[state=active]:border-blue-500">Guardrails</Tabs.Trigger>
              <Tabs.Trigger value="schedules" className="px-4 py-2 text-sm text-slate-400 data-[state=active]:text-white data-[state=active]:border-b-2 data-[state=active]:border-blue-500">Schedules</Tabs.Trigger>
            </Tabs.List>

            <Tabs.Content value="basic" className="space-y-3">
              <div>
                <label className="block text-sm text-slate-400 mb-1">Name</label>
                <input value={name} onChange={e => setName(e.target.value)} className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-2 text-white" />
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">Role</label>
                <input value={role} onChange={e => setRole(e.target.value)} className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-2 text-white" />
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">System Prompt</label>
                <textarea value={systemPrompt} onChange={e => setSystemPrompt(e.target.value)} rows={6} className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-2 text-white font-mono text-sm" />
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">Model</label>
                <input value={model} onChange={e => setModel(e.target.value)} className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-2 text-white" />
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">Channels (comma-separated)</label>
                <input value={channels} onChange={e => setChannels(e.target.value)} className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-2 text-white" />
              </div>
              <button onClick={handleSaveBasic} disabled={saving} className="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white rounded text-sm">
                {saving ? 'Saving...' : isEdit ? 'Update Agent' : 'Create Agent'}
              </button>
            </Tabs.Content>

            <Tabs.Content value="memory" className="space-y-3">
              {memEntries.map((entry, i) => (
                <div key={i} className="flex gap-2">
                  <input placeholder="Key" value={entry.key} onChange={e => { const n = [...memEntries]; n[i].key = e.target.value; setMemEntries(n); }} className="flex-1 bg-slate-900 border border-slate-700 rounded px-3 py-2 text-white text-sm" />
                  <input placeholder="Value" value={entry.value} onChange={e => { const n = [...memEntries]; n[i].value = e.target.value; setMemEntries(n); }} className="flex-1 bg-slate-900 border border-slate-700 rounded px-3 py-2 text-white text-sm" />
                  <button onClick={() => setMemEntries(memEntries.filter((_, j) => j !== i))} className="text-red-400 hover:text-red-300 px-2">Remove</button>
                </div>
              ))}
              <button onClick={() => setMemEntries([...memEntries, { key: '', value: '' }])} className="text-sm text-blue-400 hover:text-blue-300">+ Add memory entry</button>
              {isEdit && (
                <button onClick={handleSaveMemory} disabled={saving} className="block px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white rounded text-sm">
                  {saving ? 'Saving...' : 'Save Memory'}
                </button>
              )}
            </Tabs.Content>

            <Tabs.Content value="guardrails" className="space-y-3">
              <div>
                <label className="block text-sm text-slate-400 mb-1">Max Tokens Per Run</label>
                <input type="number" value={maxTokens} onChange={e => setMaxTokens(Number(e.target.value))} className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-2 text-white" />
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">Max Runs Per Hour</label>
                <input type="number" value={maxRuns} onChange={e => setMaxRuns(Number(e.target.value))} className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-2 text-white" />
              </div>
              <p className="text-xs text-slate-500">Guardrails are saved with the Basic tab.</p>
            </Tabs.Content>

            <Tabs.Content value="schedules" className="space-y-3">
              {!isEdit ? (
                <p className="text-slate-500 text-sm py-8 text-center">Save the agent first, then add schedules.</p>
              ) : (
                <>
                  {schedules.map(sch => (
                    <div key={sch.id} className="bg-slate-900 border border-slate-700 rounded p-3">
                      <div className="flex items-center justify-between mb-1">
                        <code className="text-sm text-blue-400">{sch.cron_expr}</code>
                        <div className="flex items-center gap-2">
                          <button onClick={() => handleToggleSchedule(sch.id, !sch.enabled)} className={`text-xs px-2 py-0.5 rounded ${sch.enabled ? 'bg-green-900 text-green-300' : 'bg-slate-700 text-slate-400'}`}>
                            {sch.enabled ? 'Enabled' : 'Disabled'}
                          </button>
                          <button onClick={() => handleDeleteSchedule(sch.id)} className="text-red-400 hover:text-red-300"><Trash2 size={14} /></button>
                        </div>
                      </div>
                      <p className="text-xs text-slate-400 line-clamp-2">{sch.task_prompt}</p>
                      {sch.last_run && <p className="text-xs text-slate-500 mt-1">Last run: {new Date(sch.last_run).toLocaleString()}</p>}
                    </div>
                  ))}
                  <div className="border-t border-slate-700 pt-3 space-y-2">
                    <div>
                      <label className="block text-xs text-slate-400 mb-1">Cron Expression</label>
                      <input value={newCronExpr} onChange={e => setNewCronExpr(e.target.value)} placeholder="e.g. 0 9 * * * (daily at 9am)" className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-2 text-white text-sm font-mono" />
                    </div>
                    <div>
                      <label className="block text-xs text-slate-400 mb-1">Task Prompt</label>
                      <textarea value={newTaskPrompt} onChange={e => setNewTaskPrompt(e.target.value)} rows={3} placeholder="What should the agent do when this schedule fires?" className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-2 text-white text-sm" />
                    </div>
                    <button onClick={handleAddSchedule} disabled={saving || !newCronExpr || !newTaskPrompt} className="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white rounded text-sm">
                      {saving ? 'Adding...' : 'Add Schedule'}
                    </button>
                  </div>
                </>
              )}
            </Tabs.Content>
          </Tabs.Root>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

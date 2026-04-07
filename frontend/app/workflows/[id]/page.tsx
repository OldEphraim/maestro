'use client';

import { useEffect, useState, useCallback, useMemo } from 'react';
import { useParams, useRouter } from 'next/navigation';
import {
  ReactFlow, Background, Controls, useNodesState, useEdgesState, addEdge,
  type Connection, type Node, type Edge,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import Link from 'next/link';
import { ArrowLeft, Play, Plus } from 'lucide-react';
import { getWorkflow, listAgents, executeWorkflow, createNode, createEdge as apiCreateEdge, updateEdge, type Workflow, type Agent, type WorkflowEdge } from '@/lib/api';
import AgentNode from '@/components/flow/AgentNode';

const nodeTypes = { agent: AgentNode };

export default function WorkflowEditorPage() {
  const params = useParams();
  const wfId = params.id as string;
  const router = useRouter();
  const [workflow, setWorkflow] = useState<Workflow | null>(null);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const [showAddMenu, setShowAddMenu] = useState(false);
  const [running, setRunning] = useState(false);
  const [selectedEdge, setSelectedEdge] = useState<{ id: string; condition: string; priority: number } | null>(null);
  const [edgeCondition, setEdgeCondition] = useState('always');
  const [edgePriority, setEdgePriority] = useState(0);
  const [edgeCustomCondition, setEdgeCustomCondition] = useState('');

  const agentMap = useMemo(() => {
    const m: Record<string, Agent> = {};
    agents.forEach(a => { m[a.id] = a; });
    return m;
  }, [agents]);

  useEffect(() => {
    listAgents().then(setAgents).catch(console.error);
  }, []);

  useEffect(() => {
    getWorkflow(wfId).then(wf => {
      setWorkflow(wf);
      if (wf.nodes) {
        setNodes(wf.nodes.map(n => ({
          id: n.id,
          type: 'agent',
          position: { x: n.position_x, y: n.position_y },
          data: {
            label: n.label,
            role: agentMap[n.agent_id]?.role || '',
            channels: agentMap[n.agent_id]?.channels || [],
            isEntry: n.is_entry,
          },
        })));
      }
      if (wf.edges) {
        setEdges(wf.edges.map(e => ({
          id: e.id,
          source: e.source_node_id,
          target: e.target_node_id,
          label: e.condition || 'always',
          animated: true,
          style: { stroke: '#64748b' },
          labelStyle: { fill: '#94a3b8', fontSize: 11 },
        })));
      }
    }).catch(console.error);
  }, [wfId, agentMap, setNodes, setEdges]);

  const onConnect = useCallback(async (connection: Connection) => {
    if (!connection.source || !connection.target) return;
    try {
      const edge = await apiCreateEdge(wfId, {
        source_node_id: connection.source,
        target_node_id: connection.target,
        condition: 'always',
        priority: 0,
      });
      setEdges(eds => addEdge({
        ...connection,
        id: edge.id,
        label: 'always',
        animated: true,
        style: { stroke: '#64748b' },
        labelStyle: { fill: '#94a3b8', fontSize: 11 },
      }, eds));
    } catch (e) {
      console.error(e);
    }
  }, [wfId, setEdges]);

  const handleAddNode = async (agent: Agent) => {
    setShowAddMenu(false);
    try {
      const node = await createNode(wfId, {
        agent_id: agent.id,
        label: agent.name,
        position_x: 100 + nodes.length * 300,
        position_y: 200,
        is_entry: nodes.length === 0,
      });
      setNodes(nds => [...nds, {
        id: node.id,
        type: 'agent',
        position: { x: node.position_x, y: node.position_y },
        data: { label: agent.name, role: agent.role, channels: agent.channels, isEntry: node.is_entry },
      }]);
    } catch (e) {
      console.error(e);
    }
  };

  const handleRun = async () => {
    setRunning(true);
    try {
      const res = await executeWorkflow(wfId);
      router.push(`/monitor/${res.execution_id}`);
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to execute');
      setRunning(false);
    }
  };

  const onEdgeClick = useCallback((_: React.MouseEvent, edge: Edge) => {
    const condition = (edge.label as string) || 'always';
    const priority = workflow?.edges?.find(e => e.id === edge.id)?.priority ?? 0;
    const presets = ['always', 'approved', 'rejected'];
    const isPreset = presets.includes(condition);
    setSelectedEdge({ id: edge.id, condition, priority });
    setEdgeCondition(isPreset ? condition : 'custom');
    setEdgeCustomCondition(isPreset ? '' : condition);
    setEdgePriority(priority);
  }, [workflow]);

  const handleSaveEdge = async () => {
    if (!selectedEdge) return;
    const condition = edgeCondition === 'custom' ? edgeCustomCondition : edgeCondition;
    try {
      await updateEdge(wfId, selectedEdge.id, { condition, priority: edgePriority });
      setEdges(eds => eds.map(e => e.id === selectedEdge.id ? {
        ...e,
        label: condition,
      } : e));
      // Update workflow edges cache for priority tracking
      if (workflow?.edges) {
        const idx = workflow.edges.findIndex(e => e.id === selectedEdge.id);
        if (idx !== -1) {
          workflow.edges[idx].condition = condition;
          workflow.edges[idx].priority = edgePriority;
        }
      }
      setSelectedEdge(null);
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to update edge');
    }
  };

  if (!workflow) return <div className="p-6 text-slate-400">Loading...</div>;

  return (
    <div className="flex flex-col h-[calc(100vh-52px)]">
      <div className="flex items-center justify-between px-4 py-2 bg-slate-900 border-b border-slate-700">
        <div className="flex items-center gap-3">
          <Link href="/workflows" className="text-slate-400 hover:text-white"><ArrowLeft size={16} /></Link>
          <h2 className="text-white font-medium">{workflow.name}</h2>
          <span className={`text-xs px-2 py-0.5 rounded ${workflow.status === 'active' ? 'bg-green-900 text-green-300' : 'bg-slate-700 text-slate-400'}`}>{workflow.status}</span>
        </div>
        <div className="flex gap-2">
          <div className="relative">
            <button onClick={() => setShowAddMenu(!showAddMenu)} className="flex items-center gap-1 px-3 py-1.5 bg-slate-700 hover:bg-slate-600 text-white rounded text-sm">
              <Plus size={14} /> Add Node
            </button>
            {showAddMenu && (
              <div className="absolute top-full mt-1 right-0 bg-slate-800 border border-slate-700 rounded-lg shadow-xl z-50 w-64 max-h-60 overflow-y-auto">
                {agents.map(a => (
                  <button key={a.id} onClick={() => handleAddNode(a)} className="w-full text-left px-4 py-2 text-sm text-slate-300 hover:bg-slate-700 hover:text-white">
                    {a.name} <span className="text-slate-500">({a.role})</span>
                  </button>
                ))}
                {agents.length === 0 && <p className="px-4 py-2 text-sm text-slate-500">No agents found</p>}
              </div>
            )}
          </div>
          <button onClick={handleRun} disabled={running} className="flex items-center gap-1 px-3 py-1.5 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white rounded text-sm">
            <Play size={14} /> {running ? 'Starting...' : 'Run Workflow'}
          </button>
        </div>
      </div>
      <div className="flex-1 relative">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onEdgeClick={onEdgeClick}
          nodeTypes={nodeTypes}
          fitView
          className="bg-slate-950"
        >
          <Background color="#334155" gap={20} />
          <Controls className="!bg-slate-800 !border-slate-700 [&>button]:!bg-slate-700 [&>button]:!border-slate-600 [&>button]:!text-white" />
        </ReactFlow>
        {selectedEdge && (
          <div className="absolute bottom-4 left-1/2 -translate-x-1/2 bg-slate-800 border border-slate-700 rounded-lg shadow-xl p-4 z-50 w-80">
            <div className="flex items-center justify-between mb-3">
              <h4 className="text-sm font-medium text-white">Edge Condition</h4>
              <button onClick={() => setSelectedEdge(null)} className="text-slate-400 hover:text-white text-sm">Close</button>
            </div>
            <div className="space-y-3">
              <div>
                <label className="block text-xs text-slate-400 mb-1">Condition</label>
                <select value={edgeCondition} onChange={e => setEdgeCondition(e.target.value)} className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-1.5 text-white text-sm">
                  <option value="always">always</option>
                  <option value="approved">approved</option>
                  <option value="rejected">rejected</option>
                  <option value="custom">custom...</option>
                </select>
              </div>
              {edgeCondition === 'custom' && (
                <div>
                  <label className="block text-xs text-slate-400 mb-1">Custom condition (substring match)</label>
                  <input value={edgeCustomCondition} onChange={e => setEdgeCustomCondition(e.target.value)} className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-1.5 text-white text-sm" placeholder="e.g. error, retry" />
                </div>
              )}
              <div>
                <label className="block text-xs text-slate-400 mb-1">Priority (lower = evaluated first)</label>
                <input type="number" value={edgePriority} onChange={e => setEdgePriority(Number(e.target.value))} className="w-full bg-slate-900 border border-slate-700 rounded px-3 py-1.5 text-white text-sm" />
              </div>
              <button onClick={handleSaveEdge} className="w-full px-3 py-1.5 bg-blue-600 hover:bg-blue-700 text-white rounded text-sm">Save</button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

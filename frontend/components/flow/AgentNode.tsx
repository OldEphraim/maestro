'use client';

import { Handle, Position } from '@xyflow/react';
import { MessageSquare } from 'lucide-react';

interface AgentNodeData {
  label: string;
  role: string;
  channels: string[];
  isEntry: boolean;
}

export default function AgentNode({ data }: { data: AgentNodeData }) {
  return (
    <div className={`bg-slate-800 border rounded-lg px-4 py-3 min-w-[180px] ${data.isEntry ? 'border-blue-500' : 'border-slate-600'}`}>
      <Handle type="target" position={Position.Left} className="!bg-slate-500" />
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium text-white">{data.label}</span>
        {data.channels?.includes('whatsapp') && <MessageSquare size={12} className="text-green-400" />}
      </div>
      <span className="text-xs text-slate-400">{data.role}</span>
      {data.isEntry && <span className="block text-[10px] text-blue-400 mt-1">Entry point</span>}
      <Handle type="source" position={Position.Right} className="!bg-slate-500" />
    </div>
  );
}

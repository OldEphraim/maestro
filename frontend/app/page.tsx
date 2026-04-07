import Link from "next/link";

export default function Home() {
  return (
    <div className="flex flex-col items-center justify-center min-h-[80vh] gap-8 p-8">
      <h1 className="text-4xl font-bold text-white">Maestro</h1>
      <p className="text-slate-400 text-center max-w-lg">
        AI Agent Orchestration Platform. Create agents, connect them into workflows,
        and watch them collaborate in real time.
      </p>
      <div className="flex gap-4">
        <Link href="/templates" className="px-6 py-3 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium">
          Browse Templates
        </Link>
        <Link href="/agents" className="px-6 py-3 bg-slate-700 hover:bg-slate-600 text-white rounded-lg font-medium">
          Manage Agents
        </Link>
      </div>
    </div>
  );
}

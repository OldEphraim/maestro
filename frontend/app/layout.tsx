import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import Link from "next/link";
import "./globals.css";

const geistSans = Geist({ variable: "--font-geist-sans", subsets: ["latin"] });
const geistMono = Geist_Mono({ variable: "--font-geist-mono", subsets: ["latin"] });

export const metadata: Metadata = {
  title: "Maestro",
  description: "AI Agent Orchestration Platform",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${geistSans.variable} ${geistMono.variable} h-full`}>
      <body className="min-h-full flex flex-col bg-[var(--background)] text-[var(--foreground)]">
        <nav className="border-b border-slate-700 bg-slate-900 px-6 py-3 flex items-center gap-6">
          <Link href="/" className="text-lg font-bold text-white">Maestro</Link>
          <Link href="/agents" className="text-sm text-slate-300 hover:text-white">Agents</Link>
          <Link href="/workflows" className="text-sm text-slate-300 hover:text-white">Workflows</Link>
          <Link href="/templates" className="text-sm text-slate-300 hover:text-white">Templates</Link>
        </nav>
        <main className="flex-1">{children}</main>
      </body>
    </html>
  );
}

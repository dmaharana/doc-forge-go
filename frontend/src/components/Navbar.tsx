import React from 'react';
import { useAuth } from '@/context/AuthContext';
import {
  FileText,
  FolderGit2,
  LayoutDashboard,
  LogOut,
  Settings,
  Users,
  Cpu,
} from 'lucide-react';

interface NavbarProps {
  currentView: string;
  onNavigate: (view: string, id?: string) => void;
}

export const Navbar: React.FC<NavbarProps> = ({ currentView, onNavigate }) => {
  const { user, logout, isAdmin } = useAuth();

  return (
    <header className="sticky top-0 z-50 w-full border-b bg-card/95 backdrop-blur supports-[backdrop-filter]:bg-card/60">
      <div className="container mx-auto flex h-16 items-center justify-between px-4">
        {/* Brand */}
        <div className="flex items-center gap-6">
          <button
            onClick={() => onNavigate('dashboard')}
            className="flex items-center gap-2 font-bold text-lg tracking-tight hover:opacity-90"
          >
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
              <FileText className="h-5 w-5" />
            </div>
            <div className="flex flex-col text-left">
              <span className="font-extrabold text-foreground leading-none">DocForge</span>
              <span className="text-[10px] text-muted-foreground font-mono mt-0.5">Enterprise AI Docs</span>
            </div>
          </button>

          {/* Main Navigation */}
          <nav className="hidden md:flex items-center gap-1 text-sm font-medium">
            <button
              onClick={() => onNavigate('dashboard')}
              className={`flex items-center gap-2 rounded-md px-3 py-2 transition-colors ${
                currentView === 'dashboard'
                  ? 'bg-secondary text-foreground font-semibold'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              }`}
            >
              <LayoutDashboard className="h-4 w-4" />
              Dashboard
            </button>

            <button
              onClick={() => onNavigate('projects')}
              className={`flex items-center gap-2 rounded-md px-3 py-2 transition-colors ${
                currentView.startsWith('project')
                  ? 'bg-secondary text-foreground font-semibold'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              }`}
            >
              <FolderGit2 className="h-4 w-4" />
              Projects
            </button>

            <button
              onClick={() => onNavigate('settings')}
              className={`flex items-center gap-2 rounded-md px-3 py-2 transition-colors ${
                currentView === 'settings'
                  ? 'bg-secondary text-foreground font-semibold'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              }`}
            >
              <Settings className="h-4 w-4" />
              Settings
            </button>

            {isAdmin && (
              <div className="flex items-center gap-1 border-l pl-2 ml-1">
                <button
                  onClick={() => onNavigate('admin-users')}
                  className={`flex items-center gap-1.5 rounded-md px-3 py-2 text-xs transition-colors ${
                    currentView === 'admin-users'
                      ? 'bg-primary/10 text-primary font-semibold'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                  }`}
                >
                  <Users className="h-3.5 w-3.5" />
                  Users
                </button>

                <button
                  onClick={() => onNavigate('admin-llm')}
                  className={`flex items-center gap-1.5 rounded-md px-3 py-2 text-xs transition-colors ${
                    currentView === 'admin-llm'
                      ? 'bg-primary/10 text-primary font-semibold'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                  }`}
                >
                  <Cpu className="h-3.5 w-3.5" />
                  LLM Config
                </button>
              </div>
            )}
          </nav>
        </div>

        {/* User Status & Logout */}
        <div className="flex items-center gap-3">
          {user && (
            <div className="flex items-center gap-2 text-right">
              <div className="hidden sm:block">
                <div className="text-xs font-semibold text-foreground leading-none">{user.name}</div>
                <div className="text-[11px] text-muted-foreground mt-0.5">{user.email}</div>
              </div>
              <span
                className={`text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-full ${
                  user.role === 'admin'
                    ? 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300 border border-amber-300'
                    : 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300'
                }`}
              >
                {user.role}
              </span>
            </div>
          )}

          <button
            onClick={logout}
            title="Log Out"
            className="flex h-9 w-9 items-center justify-center rounded-md border text-muted-foreground hover:bg-destructive/10 hover:text-destructive transition-colors"
          >
            <LogOut className="h-4 w-4" />
          </button>
        </div>
      </div>
    </header>
  );
};

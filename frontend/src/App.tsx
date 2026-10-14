import React, { useState } from 'react';
import { AuthProvider, useAuth } from '@/context/AuthContext';
import { Navbar } from '@/components/Navbar';
import { LoginPage } from '@/pages/LoginPage';
import { DashboardPage } from '@/pages/DashboardPage';
import { ProjectsListPage } from '@/pages/ProjectsListPage';
import { ProjectDetailPage } from '@/pages/ProjectDetailPage';
import { DocumentReviewPage } from '@/pages/DocumentReviewPage';
import { AdminUsersPage } from '@/pages/AdminUsersPage';
import { AdminLLMPage } from '@/pages/AdminLLMPage';
import { SettingsPage } from '@/pages/SettingsPage';
import { RefreshCw } from 'lucide-react';

const MainApp: React.FC = () => {
  const { user, loading } = useAuth();
  const [currentView, setCurrentView] = useState<string>('dashboard');
  const [activeId, setActiveId] = useState<string | undefined>(undefined);

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50 dark:bg-slate-950 text-muted-foreground gap-2">
        <RefreshCw className="h-5 w-5 animate-spin" /> Loading DocForge...
      </div>
    );
  }

  if (!user) {
    return <LoginPage />;
  }

  const navigate = (view: string, id?: string) => {
    setCurrentView(view);
    setActiveId(id);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  return (
    <div className="min-h-screen flex flex-col bg-slate-50/50 dark:bg-slate-950 text-foreground">
      <Navbar currentView={currentView} onNavigate={navigate} />

      <main className="flex-1">
        {currentView === 'dashboard' && <DashboardPage onNavigate={navigate} />}
        {currentView === 'projects' && <ProjectsListPage onNavigate={navigate} />}
        {currentView === 'project-detail' && activeId && (
          <ProjectDetailPage projectId={activeId} onNavigate={navigate} />
        )}
        {currentView === 'document-review' && activeId && (
          <DocumentReviewPage documentId={activeId} onNavigate={navigate} />
        )}
        {currentView === 'admin-users' && <AdminUsersPage />}
        {currentView === 'admin-llm' && <AdminLLMPage />}
        {currentView === 'settings' && <SettingsPage />}
      </main>
    </div>
  );
};

export default function App() {
  return (
    <AuthProvider>
      <MainApp />
    </AuthProvider>
  );
}

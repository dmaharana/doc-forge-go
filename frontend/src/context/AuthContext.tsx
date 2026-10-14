import React, { createContext, useContext, useEffect, useState } from 'react';
import type { User } from '@/types';
import { api, removeToken } from '@/lib/api';

interface AuthContextType {
  user: User | null;
  loading: boolean;
  login: (email: string, pass: string) => Promise<void>;
  loginWithAtlassian: (code?: string) => Promise<void>;
  logout: () => void;
  isAdmin: boolean;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState<boolean>(true);

  const refreshUser = async () => {
    try {
      const u = await api.getMe();
      setUser(u);
    } catch {
      setUser(null);
      removeToken();
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    const token = localStorage.getItem('docforge_token');
    if (token) {
      refreshUser();
    } else {
      setLoading(false);
    }

    const handleUnauthorized = () => {
      setUser(null);
    };

    window.addEventListener('auth:unauthorized', handleUnauthorized);
    return () => window.removeEventListener('auth:unauthorized', handleUnauthorized);
  }, []);

  const login = async (email: string, pass: string) => {
    setLoading(true);
    try {
      const res = await api.login(email, pass);
      setUser(res.user);
    } finally {
      setLoading(false);
    }
  };

  const loginWithAtlassian = async (code: string = 'mock_atlassian_code') => {
    setLoading(true);
    try {
      const res = await api.atlassianSSOCallback(code);
      setUser(res.user);
    } finally {
      setLoading(false);
    }
  };

  const logout = () => {
    removeToken();
    setUser(null);
  };

  const isAdmin = user?.role === 'admin';

  return (
    <AuthContext.Provider value={{ user, loading, login, loginWithAtlassian, logout, isAdmin }}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};

import React, { useEffect, useState } from 'react';
import type { User, UserRole } from '@/types';
import { api } from '@/lib/api';
import { Plus, RefreshCw, Users } from 'lucide-react';

export const AdminUsersPage: React.FC = () => {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddModal, setShowAddModal] = useState(false);

  // Form
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState<UserRole>('user');
  const [submitting, setSubmitting] = useState(false);

  const loadUsers = async () => {
    setLoading(true);
    try {
      const list = await api.adminListUsers();
      setUsers(list || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadUsers();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    try {
      await api.adminCreateUser({ name, email, role, password });
      setShowAddModal(false);
      setName('');
      setEmail('');
      setPassword('');
      setRole('user');
      await loadUsers();
    } catch (err: any) {
      alert(err.message || 'Failed to create user');
    } finally {
      setSubmitting(false);
    }
  };

  const handleToggleActive = async (u: User) => {
    try {
      await api.adminUpdateUser(u.id, {
        name: u.name,
        role: u.role,
        is_active: !u.is_active,
      });
      setUsers((prev) =>
        prev.map((item) => (item.id === u.id ? { ...item, is_active: !item.is_active } : item))
      );
    } catch (err: any) {
      alert(err.message || 'Failed to update user');
    }
  };

  return (
    <div className="max-w-7xl mx-auto py-6 px-4 space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b pb-5">
        <div>
          <h1 className="text-2xl font-black tracking-tight flex items-center gap-2">
            <Users className="h-6 w-6 text-primary" /> User Management
          </h1>
          <p className="text-sm text-muted-foreground">
            Manage system access, assign Administrator or User roles, and deactivate accounts
          </p>
        </div>

        <button
          onClick={() => setShowAddModal(true)}
          className="inline-flex items-center gap-2 px-4 py-2.5 rounded-lg bg-primary text-primary-foreground font-semibold text-sm shadow hover:opacity-90"
        >
          <Plus className="h-4 w-4" /> Add User
        </button>
      </div>

      {loading ? (
        <div className="p-16 text-center text-muted-foreground flex items-center justify-center gap-2">
          <RefreshCw className="h-5 w-5 animate-spin" /> Loading users...
        </div>
      ) : (
        <div className="border rounded-xl bg-card overflow-hidden">
          <table className="w-full text-left text-sm">
            <thead className="bg-muted/50 border-b text-xs uppercase font-semibold text-muted-foreground">
              <tr>
                <th className="py-3 px-4">User</th>
                <th className="py-3 px-4">Role</th>
                <th className="py-3 px-4">Status</th>
                <th className="py-3 px-4">Created Date</th>
                <th className="py-3 px-4 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y text-xs">
              {users.map((u) => (
                <tr key={u.id} className="hover:bg-muted/30">
                  <td className="py-3 px-4">
                    <div className="font-bold text-foreground">{u.name}</div>
                    <div className="text-muted-foreground text-[11px]">{u.email}</div>
                  </td>
                  <td className="py-3 px-4">
                    <span
                      className={`px-2 py-0.5 rounded-full text-[10px] font-bold uppercase ${
                        u.role === 'admin'
                          ? 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300'
                          : 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300'
                      }`}
                    >
                      {u.role}
                    </span>
                  </td>
                  <td className="py-3 px-4">
                    <span
                      className={`px-2 py-0.5 rounded-full text-[10px] font-bold uppercase ${
                        u.is_active
                          ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
                          : 'bg-red-100 text-red-800'
                      }`}
                    >
                      {u.is_active ? 'Active' : 'Deactivated'}
                    </span>
                  </td>
                  <td className="py-3 px-4 text-muted-foreground">
                    {new Date(u.created_at).toLocaleDateString()}
                  </td>
                  <td className="py-3 px-4 text-right">
                    <button
                      onClick={() => handleToggleActive(u)}
                      className={`px-3 py-1 rounded-lg border font-medium text-xs transition ${
                        u.is_active
                          ? 'border-destructive/30 text-destructive hover:bg-destructive/10'
                          : 'border-emerald-500/30 text-emerald-600 hover:bg-emerald-50'
                      }`}
                    >
                      {u.is_active ? 'Deactivate' : 'Activate'}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Add User Modal */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-md border rounded-2xl shadow-2xl p-6 space-y-4">
            <h2 className="text-lg font-bold">Create New User</h2>
            <form onSubmit={handleCreate} className="space-y-3 text-xs">
              <div className="space-y-1">
                <label className="font-semibold uppercase text-muted-foreground">Full Name</label>
                <input
                  type="text"
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border bg-background"
                  placeholder="e.g. Jane Doe"
                />
              </div>

              <div className="space-y-1">
                <label className="font-semibold uppercase text-muted-foreground">Email Address</label>
                <input
                  type="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border bg-background"
                  placeholder="jane@company.com"
                />
              </div>

              <div className="space-y-1">
                <label className="font-semibold uppercase text-muted-foreground">Initial Password</label>
                <input
                  type="password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border bg-background"
                  placeholder="••••••••"
                />
              </div>

              <div className="space-y-1">
                <label className="font-semibold uppercase text-muted-foreground">Role</label>
                <select
                  value={role}
                  onChange={(e) => setRole(e.target.value as UserRole)}
                  className="w-full px-3 py-2 rounded-lg border bg-background"
                >
                  <option value="user">User (Standard Access)</option>
                  <option value="admin">Admin (System Configuration & User Management)</option>
                </select>
              </div>

              <div className="flex justify-end gap-2 pt-3">
                <button
                  type="button"
                  onClick={() => setShowAddModal(false)}
                  className="px-3.5 py-1.5 rounded-lg border font-medium hover:bg-muted"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submitting}
                  className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground font-semibold shadow hover:opacity-90 disabled:opacity-50"
                >
                  {submitting ? 'Creating...' : 'Create User'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};

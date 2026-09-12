import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { api, ApiError, cancelRequests, errorMessage, isAbort, json, setUnauthorizedHandler } from '../lib/api';
import { AuthContext } from '../lib/auth-context';
import { clearInvitationToken } from '../lib/invitation';
import type { User } from '../lib/types';
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const endSession = useCallback((message = '') => {
    cancelRequests(); setUser(null); setNotice(message); setError(''); setLoading(false); clearInvitationToken();
  }, []);
  const refresh = useCallback(async () => {
    setLoading(true); setError('');
    try { setUser(await api<User>('/me')); }
    catch (err) {
      if (err instanceof ApiError && err.status === 401) setUser(null);
      else if (!isAbort(err)) setError(errorMessage(err));
    }
    finally { setLoading(false); }
  }, []);
  useEffect(() => {
    setUnauthorizedHandler(() => { setUser(null); setLoading(false); setError(''); setNotice('Сессия завершена или недействительна. Войдите снова.'); });
    void refresh();
    return () => { setUnauthorizedHandler(undefined); cancelRequests(); };
  }, [refresh]);
  useEffect(() => {
    // Recheck access on return to the tab (including a restored page).
    const check = () => { if (document.visibilityState === 'visible' && user) void api<User>('/me').then(setUser).catch(() => {}); };
    document.addEventListener('visibilitychange', check);
    window.addEventListener('pageshow', check);
    return () => { document.removeEventListener('visibilitychange', check); window.removeEventListener('pageshow', check); };
  }, [user]);
  const acceptUser = useCallback((next: User) => { setUser(next); setNotice(''); setError(''); setLoading(false); }, []);
  const logout = useCallback(async () => { await api<void>('/auth/logout', json('POST')); endSession('Вы вышли из SweetNet.'); }, [endSession]);
  return <AuthContext value={{ user, loading, error, notice, refresh, acceptUser, endSession, logout }}>{children}</AuthContext>;
}

import { createContext, useContext } from 'react';
import type { User } from './types';
export interface AuthState {
  user: User | null; loading: boolean; error: string; notice: string;
  refresh: () => Promise<void>; acceptUser: (user: User) => void;
  endSession: (notice?: string) => void; logout: () => Promise<void>;
}
export const AuthContext = createContext<AuthState | null>(null);
export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error('AuthProvider is required');
  return value;
}

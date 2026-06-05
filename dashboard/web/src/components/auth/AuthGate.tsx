// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react';
import { Loader2 } from 'lucide-react';
import { getAuthStatus, type AuthStatus } from '../../services/auth';
import { setUnauthorizedHandler } from '../../services/api';
import { LoginScreen } from './LoginScreen';
import { AppPasswordFirstRun } from './AppPasswordFirstRun';

interface AuthContextValue {
  status: AuthStatus | null;
  refresh: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue>({
  status: null,
  refresh: async () => {},
});

export const useAuth = () => useContext(AuthContext);

// AuthGate wraps the whole app. When the optional app password is enabled and
// the session is missing, it shows the login screen instead of the app. When
// disabled, it is a transparent pass-through. A 401 from any API call (session
// expiry) flips the state back to "needs login" via the shared handler.
export function AuthGate({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      setStatus(await getAuthStatus());
    } catch {
      // If status is unreachable, fail open (treat as disabled) so a backend
      // hiccup never hard-locks the dashboard.
      setStatus({
        enabled: false,
        configured: false,
        authenticated: false,
        setupDismissed: true,
      });
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    setUnauthorizedHandler(() =>
      setStatus((s) => (s ? { ...s, authenticated: false } : s)),
    );
    return () => setUnauthorizedHandler(null);
  }, []);

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (status?.enabled && !status.authenticated) {
    return <LoginScreen onSuccess={refresh} />;
  }

  const showFirstRun = !!status && !status.configured && !status.setupDismissed;
  return (
    <AuthContext.Provider value={{ status, refresh }}>
      {children}
      {showFirstRun && <AppPasswordFirstRun onDone={refresh} />}
    </AuthContext.Provider>
  );
}

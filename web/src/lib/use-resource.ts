import { useCallback, useEffect, useState, type SetStateAction } from 'react';
import { api, errorMessage, isAbort } from './api';
export function useResource<T>(path: string) {
  const [state, setState] = useState<{ path: string; data: T | null; error: string; loading: boolean }>({ path, data: null, error: '', loading: true });
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    api<T>(path, { signal: controller.signal }).then(data => {
      if (!controller.signal.aborted) setState({ path, data, error: '', loading: false });
    }).catch(error => {
      if (!controller.signal.aborted && !isAbort(error)) setState(previous => ({ path, data: previous.path === path ? previous.data : null, error: errorMessage(error), loading: false }));
    });
    return () => controller.abort();
  }, [path, revision]);
  const retry = useCallback(() => { setState(s => ({ ...s, loading: true, error: '' })); setRevision(r => r + 1); }, []);
  const setData = useCallback((next: SetStateAction<T | null>) => setState(previous => ({ path, data: typeof next === 'function' ? (next as (value: T | null) => T | null)(previous.data) : next, error: '', loading: false })), [path]);
  return { ...(state.path === path ? state : { data: null, loading: true, error: '' }), retry, setData };
}

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string, public fields?: Record<string, string>) {
    super(message);
  }
}
const pending = new Set<AbortController>();
let onUnauthorized: (() => void) | undefined;
export function setUnauthorizedHandler(handler: (() => void) | undefined) { onUnauthorized = handler; }
export function cancelRequests() {
  pending.forEach(controller => controller.abort());
  pending.clear();
}
async function request<T>(path: string, options: RequestInit, decode: (response: Response) => Promise<T>) {
  const controller = new AbortController();
  const signal = options.signal ? AbortSignal.any([options.signal, controller.signal]) : controller.signal;
  pending.add(controller);
  try {
    const response = await fetch(`/api/v1${path}`, {
      ...options, signal, credentials: 'same-origin', cache: 'no-store',
      headers: { ...(options.body && !(options.body instanceof FormData) ? { 'Content-Type': 'application/json' } : {}), ...options.headers },
    });
    if (!response.ok) {
      if (response.status === 401) {
        pending.delete(controller);
        cancelRequests();
        onUnauthorized?.();
      }
      const payload = await response.json().catch(() => null);
      throw new ApiError(response.status, payload?.error?.code ?? 'REQUEST_ERROR',
        payload?.error?.message ?? (response.status === 401 ? 'Сессия истекла. Войдите снова.' : 'Не удалось выполнить запрос. Попробуйте ещё раз.'), payload?.error?.fields);
    }
    const result = await decode(response);
    signal.throwIfAborted();
    return result;
  } finally { pending.delete(controller); }
}
export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  return request<T>(path, options ?? {}, response => response.status === 204 ? Promise.resolve(undefined as T) : response.json() as Promise<T>);
}
export async function mediaBlob(id: string, signal: AbortSignal) {
  return request(`/media/${encodeURIComponent(id)}`, { signal }, response => response.blob());
}
export const json = (method: string, body?: unknown): RequestInit => ({ method, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message + (error.fields ? ` ${Object.values(error.fields).join(' ')}` : '');
  return 'Не удалось связаться с сервером. Проверьте соединение и попробуйте ещё раз.';
}
export function isAbort(error: unknown) { return error instanceof DOMException && error.name === 'AbortError'; }

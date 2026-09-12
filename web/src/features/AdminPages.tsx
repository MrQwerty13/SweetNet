import { useState } from 'react';
import { api, errorMessage, json } from '../lib/api';
import type { Invite, User } from '../lib/types';
import { useResource } from '../lib/use-resource';
import { formatDate } from '../lib/validation';
import {
  Button,
  ConfirmDialog,
  EmptyState,
  ErrorNotice,
  Loading,
  PageHeading,
} from '../components/ui';
function inviteStatus(invite: Invite) {
  if (invite.used_at) return 'Использовано';
  if (invite.revoked_at) return 'Отозвано';
  if (new Date(invite.expires_at).getTime() <= Date.now()) return 'Истекло';
  return 'Активно';
}
export function InvitesPage() {
  const { data, loading, error, retry, setData } = useResource<{ items: Invite[] }>(
    '/admin/invites',
  );
  const [created, setCreated] = useState<{ id: string; url: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState('');
  const [selected, setSelected] = useState<Invite | null>(null);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');
  async function create() {
    if (busy || !data) return;
    setBusy(true);
    setActionError('');
    try {
      const result = await api<Invite & { url: string }>('/admin/invites', json('POST'));
      setCreated({ id: result.id, url: new URL(result.url, window.location.origin).href });
      setCopied(false);
      setCopyError('');
      const { id, created_at, expires_at, used_at, revoked_at } = result;
      setData({ items: [{ id, created_at, expires_at, used_at, revoked_at }, ...data.items] });
    } catch (err) {
      setActionError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  async function revoke() {
    if (busy || !selected || !data) return;
    setBusy(true);
    setActionError('');
    try {
      await api<void>(`/admin/invites/${encodeURIComponent(selected.id)}`, json('DELETE'));
      setData({
        items: data.items.map((invite) =>
          invite.id === selected.id ? { ...invite, revoked_at: new Date().toISOString() } : invite,
        ),
      });
      if (created?.id === selected.id) setCreated(null);
      setSelected(null);
    } catch (err) {
      setActionError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  async function copy() {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(created.url);
      setCopied(true);
      setCopyError('');
    } catch {
      setCopyError('Не удалось скопировать автоматически. Выделите ссылку и скопируйте вручную.');
    }
  }
  return (
    <>
      <PageHeading
        title="Приглашения"
        subtitle="Позовите друзей в ваш круг."
        action={
          <Button onClick={() => void create()} disabled={busy || loading || !data}>
            {busy && !selected ? 'Создаём…' : 'Создать приглашение'}
          </Button>
        }
      />
      <p className="muted">Каждое приглашение действует 7 суток и подходит для одного человека.</p>
      {created && (
        <section className="invite-result" aria-labelledby="invite-ready">
          <h2 id="invite-ready">Приглашение готово</h2>
          <p>
            Сохраните ссылку и отправьте другу лично. Она показывается только сейчас и исчезнет при
            уходе со страницы.
          </p>
          <label htmlFor="invite-url">Ссылка приглашения</label>
          <input id="invite-url" value={created.url} readOnly onFocus={(e) => e.target.select()} />
          <div className="form-actions">
            <Button variant="secondary" onClick={() => void copy()}>
              {copied ? 'Скопировано' : 'Скопировать ссылку'}
            </Button>
            <Button variant="text" onClick={() => setCreated(null)}>
              Скрыть ссылку
            </Button>
          </div>
          <span className="sr-only" role="status">
            {copied ? 'Ссылка скопирована' : ''}
          </span>
          <ErrorNotice message={copyError} />
        </section>
      )}
      {loading && <Loading />}
      <ErrorNotice message={error} retry={retry} />
      {!selected && <ErrorNotice message={actionError} />}
      {data?.items.length === 0 && (
        <EmptyState title="Приглашений пока нет">Создайте первое приглашение для друга.</EmptyState>
      )}
      <ul className="admin-list">
        {data?.items.map((invite) => (
          <li key={invite.id} className="admin-row">
            <div>
              <strong>{inviteStatus(invite)}</strong>
              <p className="muted">Создано {formatDate(invite.created_at)}</p>
              <p className="hint">Действует до {formatDate(invite.expires_at)}</p>
            </div>
            {inviteStatus(invite) === 'Активно' && (
              <Button
                variant="text"
                disabled={busy}
                onClick={() => {
                  setActionError('');
                  setSelected(invite);
                }}
              >
                Отозвать
              </Button>
            )}
          </li>
        ))}
      </ul>
      {selected && (
        <ConfirmDialog
          title="Отозвать приглашение?"
          confirmLabel="Отозвать"
          onConfirm={() => void revoke()}
          onCancel={() => setSelected(null)}
          busy={busy}
          error={actionError}
        >
          <p>Друг больше не сможет зарегистрироваться по этой ссылке.</p>
        </ConfirmDialog>
      )}
    </>
  );
}
export function UsersPage() {
  const { data, loading, error, retry, setData } = useResource<{ items: User[] }>('/admin/users');
  const [selected, setSelected] = useState<User | null>(null);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');
  async function toggle() {
    if (!selected || !data || busy) return;
    setBusy(true);
    setActionError('');
    try {
      const updated = await api<User>(
        `/admin/users/${encodeURIComponent(selected.id)}`,
        json('PATCH', { is_active: !selected.is_active }),
      );
      setData({ items: data.items.map((user) => (user.id === updated.id ? updated : user)) });
      setSelected(null);
    } catch (err) {
      setActionError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeading title="Участники" subtitle="Люди, с которыми вы делитесь моментами." />
      {loading && <Loading />}
      <ErrorNotice message={error} retry={retry} />
      <ul className="admin-list">
        {data?.items.map((user) => (
          <li key={user.id} className="admin-row">
            <div>
              <strong>{user.display_name}</strong>
              <p className="muted">
                @{user.username} ·{' '}
                {user.role === 'owner' ? 'Владелец' : user.is_active ? 'Активен' : 'Отключён'}
              </p>
              <p className="hint">В круге с {formatDate(user.created_at)}</p>
            </div>
            {user.role !== 'owner' && (
              <Button
                variant="text"
                onClick={() => {
                  setActionError('');
                  setSelected(user);
                }}
                disabled={busy}
              >
                {user.is_active ? 'Отключить' : 'Включить'}
              </Button>
            )}
          </li>
        ))}
      </ul>
      {selected && (
        <ConfirmDialog
          title={selected.is_active ? 'Отключить участника?' : 'Включить участника?'}
          confirmLabel={selected.is_active ? 'Отключить' : 'Включить'}
          onConfirm={() => void toggle()}
          onCancel={() => setSelected(null)}
          busy={busy}
          error={actionError}
        >
          <p>
            {selected.display_name}:{' '}
            {selected.is_active
              ? 'доступ будет сразу закрыт на всех устройствах. Публикации сохранятся.'
              : 'доступ вернётся после нового входа. Старые сессии останутся недействительными.'}
          </p>
        </ConfirmDialog>
      )}
    </>
  );
}

import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { api, errorMessage, json } from '../lib/api';
import { useAuth } from '../lib/auth-context';
import type { User } from '../lib/types';
import { validDisplayName, validPassword } from '../lib/validation';
import { Button, ErrorNotice, Field, PageHeading } from '../components/ui';
export function ProfilePage() {
  const { user, acceptUser, endSession } = useAuth();
  const [name, setName] = useState(user?.display_name ?? '');
  const [current, setCurrent] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState<'name' | 'password' | null>(null);
  const [nameError, setNameError] = useState('');
  const [passwordError, setPasswordError] = useState('');
  const [success, setSuccess] = useState('');
  const navigate = useNavigate();
  async function saveName(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setNameError('');
    setSuccess('');
    if (!validDisplayName(name)) {
      setNameError('Имя должно содержать от 1 до 80 символов.');
      return;
    }
    setBusy('name');
    try {
      acceptUser(await api<User>('/me', json('PATCH', { display_name: name })));
      setSuccess('Имя сохранено.');
    } catch (err) {
      setNameError(errorMessage(err));
    } finally {
      setBusy(null);
    }
  }
  async function savePassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setPasswordError('');
    if (!validPassword(password)) {
      setPasswordError('Новый пароль должен содержать от 12 до 128 символов.');
      return;
    }
    if (password !== confirm) {
      setPasswordError('Новые пароли не совпадают.');
      return;
    }
    setBusy('password');
    try {
      await api<void>(
        '/me/password',
        json('POST', { current_password: current, new_password: password }),
      );
      setCurrent('');
      setPassword('');
      setConfirm('');
      endSession('Пароль изменён. Все сессии завершены. Войдите с новым паролем.');
      navigate('/login', { replace: true });
    } catch (err) {
      setPasswordError(errorMessage(err));
    } finally {
      setBusy(null);
    }
  }
  return (
    <>
      <PageHeading title="Профиль" />
      <section className="form-section">
        <h2>О вас</h2>
        <form onSubmit={saveName}>
          <fieldset disabled={busy !== null}>
            <Field
              label="Логин"
              value={user?.username ?? ''}
              readOnly
              hint="Логин нельзя изменить."
            />
            <Field
              label="Отображаемое имя"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              autoComplete="nickname"
              hint="От 1 до 80 символов."
            />
            <ErrorNotice message={nameError} />
            {success && (
              <p role="status" className="success">
                {success}
              </p>
            )}
            <Button type="submit">{busy === 'name' ? 'Сохраняем…' : 'Сохранить имя'}</Button>
          </fieldset>
        </form>
      </section>
      <section className="form-section">
        <h2>Сменить пароль</h2>
        <p className="muted">
          После смены пароля все ваши сессии будут завершены, включая текущую.
        </p>
        <form onSubmit={savePassword}>
          <fieldset disabled={busy !== null}>
            <Field
              label="Текущий пароль"
              type="password"
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
              autoComplete="current-password"
              required
            />
            <Field
              label="Новый пароль"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
              required
              hint="12–128 символов. Пробелы учитываются."
            />
            <Field
              label="Повторите новый пароль"
              type="password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              autoComplete="new-password"
              required
            />
            <ErrorNotice message={passwordError} />
            <Button type="submit">{busy === 'password' ? 'Сохраняем…' : 'Изменить пароль'}</Button>
          </fieldset>
        </form>
      </section>
    </>
  );
}

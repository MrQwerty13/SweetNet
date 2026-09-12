import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { api, errorMessage, json } from '../lib/api';
import { useAuth } from '../lib/auth-context';
import { clearInvitationToken, getInvitationToken } from '../lib/invitation';
import type { User } from '../lib/types';
import { validDisplayName, validPassword, validUsername } from '../lib/validation';
import { Button, ErrorNotice, Field, PageHeading } from '../components/ui';
export function AuthPage({ join = false }: { join?: boolean }) {
  const [token] = useState(getInvitationToken);
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const { acceptUser, notice } = useAuth();
  const navigate = useNavigate();
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setError('');
    if (!validUsername(username)) {
      setError('Логин: от 3 до 32 латинских букв, цифр или символов _.');
      return;
    }
    if (join && (!validDisplayName(displayName) || !validPassword(password))) {
      setError('Имя: 1–80 символов. Пароль: 12–128 символов.');
      return;
    }
    setBusy(true);
    try {
      const user = await api<User>(
        join ? '/auth/register' : '/auth/login',
        json(
          'POST',
          join
            ? { token, username: username.toLowerCase(), display_name: displayName, password }
            : { username: username.toLowerCase(), password },
        ),
      );
      setPassword('');
      clearInvitationToken();
      acceptUser(user);
      navigate('/', { replace: true });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  if (join && !token)
    return (
      <>
        <PageHeading title="Нужно приглашение" />
        <p>
          Откройте полную ссылку, которую отправил владелец круга. Приглашение действует 7 суток и
          используется один раз.
        </p>
        <Link className="button button-secondary" to="/login">
          Перейти ко входу
        </Link>
      </>
    );
  return (
    <>
      <PageHeading
        title={join ? 'Добро пожаловать в круг' : 'Войти в SweetNet'}
        subtitle={join ? 'Место для вас и ваших друзей.' : 'Рады снова видеть вас.'}
      />
      {notice && !join && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}
      {join && (
        <p className="privacy-note">
          Публикации доступны всем участникам этого закрытого круга. Вы увидите и записи, созданные
          до вашего вступления.
        </p>
      )}
      <form onSubmit={submit}>
        <fieldset disabled={busy}>
          <Field
            label="Логин"
            name="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoComplete="username"
            autoCapitalize="none"
            spellCheck={false}
            hint={join ? '3–32 латинских буквы, цифры или _. Регистр не важен.' : undefined}
          />
          {join && (
            <Field
              label="Отображаемое имя"
              name="display_name"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              required
              autoComplete="nickname"
              hint="Как вас будут видеть друзья. До 80 символов."
            />
          )}
          <Field
            label="Пароль"
            type="password"
            name="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete={join ? 'new-password' : 'current-password'}
            hint={join ? 'От 12 до 128 символов. Пробелы учитываются.' : undefined}
          />
          <ErrorNotice message={error} />
          <Button type="submit" className="full-width">
            {busy ? 'Подождите…' : join ? 'Присоединиться' : 'Войти'}
          </Button>
        </fieldset>
      </form>
      <p className="auth-footer muted">
        {join ? (
          <Link to="/login">Уже есть аккаунт? Войти</Link>
        ) : (
          'Нужен аккаунт или забыли пароль? Обратитесь к владельцу круга.'
        )}
      </p>
    </>
  );
}

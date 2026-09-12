import { useState } from 'react';
import { Link, NavLink, Outlet } from 'react-router-dom';
import { useAuth } from '../lib/auth-context';
import { errorMessage } from '../lib/api';
import { Button, ErrorNotice } from './ui';
export function Shell() {
  const { user, logout } = useAuth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function handleLogout() {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      await logout();
    } catch (err) {
      setError(
        `Выход не подтверждён сервером: сессия могла остаться активной. Повторите выход. ${errorMessage(err)}`,
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <a className="skip-link" href="#main">
        К содержимому
      </a>
      <header className="site-header">
        <div className="header-inner">
          <Link to="/" className="brand" aria-label="SweetNet — лента">
            SweetNet
          </Link>
          <nav aria-label="Основная навигация">
            <NavLink to="/" end>
              Лента
            </NavLink>
            <NavLink to="/profile">Профиль</NavLink>
            {user?.role === 'owner' && (
              <>
                <NavLink to="/admin/users">Участники</NavLink>
                <NavLink to="/admin/invites">Приглашения</NavLink>
              </>
            )}
          </nav>
          <Button
            variant="text"
            className="logout"
            onClick={() => void handleLogout()}
            disabled={busy}
          >
            {busy ? 'Выходим…' : 'Выйти'}
          </Button>
        </div>
      </header>
      <main id="main" className="main-content">
        <ErrorNotice message={error} />
        <Outlet />
      </main>
    </>
  );
}
export function PublicShell() {
  return (
    <>
      <header className="site-header">
        <div className="header-inner">
          <Link to="/login" className="brand">
            SweetNet
          </Link>
        </div>
      </header>
      <main id="main" className="auth-content">
        <Outlet />
      </main>
    </>
  );
}

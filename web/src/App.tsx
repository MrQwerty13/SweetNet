import { useEffect } from 'react';
import {
  BrowserRouter,
  Link,
  Navigate,
  Outlet,
  Route,
  Routes,
  useLocation,
} from 'react-router-dom';
import { AuthProvider } from './components/AuthProvider';
import { PublicShell, Shell } from './components/Shell';
import { ErrorNotice, Loading, PageHeading } from './components/ui';
import { useAuth } from './lib/auth-context';
import { AuthPage } from './features/AuthPages';
import { FeedPage } from './features/FeedPage';
import { PostEditor, PostPage } from './features/PostPages';
import { ProfilePage } from './features/ProfilePage';
import { InvitesPage, UsersPage } from './features/AdminPages';
function RequireSession() {
  const { user, loading, error, refresh } = useAuth();
  if (loading)
    return (
      <main className="main-content">
        <Loading />
      </main>
    );
  if (error)
    return (
      <main className="auth-content">
        <PageHeading title="Не удалось загрузить SweetNet" />
        <ErrorNotice message={error} retry={() => void refresh()} />
        <Link to="/login">Перейти ко входу</Link>
      </main>
    );
  return user ? <Outlet key={user.id} /> : <Navigate to="/login" replace />;
}
function RequireOwner() {
  const { user } = useAuth();
  return user?.role === 'owner' ? (
    <Outlet />
  ) : (
    <>
      <PageHeading title="Доступ только владельцу" />
      <Link to="/">Вернуться в ленту</Link>
    </>
  );
}
function GuestOnly() {
  const { user, loading } = useAuth();
  if (loading) return <Loading />;
  return user ? <Navigate to="/" replace /> : <Outlet />;
}
function RouteFocus() {
  const { pathname } = useLocation();
  useEffect(() => {
    window.scrollTo(0, 0);
    const heading = document.querySelector<HTMLElement>('h1');
    if (heading) {
      heading.tabIndex = -1;
      heading.focus({ preventScroll: true });
      document.title = `${heading.textContent} — SweetNet`;
    }
  }, [pathname]);
  return null;
}
export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <RouteFocus />
        <Routes>
          <Route element={<PublicShell />}>
            <Route element={<GuestOnly />}>
              <Route path="/login" element={<AuthPage />} />
              <Route path="/join" element={<AuthPage key="join" join />} />
            </Route>
          </Route>
          <Route element={<RequireSession />}>
            <Route element={<Shell />}>
              <Route index element={<FeedPage />} />
              <Route path="posts/new" element={<PostEditor />} />
              <Route path="posts/:id" element={<PostPage key="view" />} />
              <Route path="posts/:id/edit" element={<PostPage key="edit" edit />} />
              <Route path="profile" element={<ProfilePage />} />
              <Route element={<RequireOwner />}>
                <Route path="admin/invites" element={<InvitesPage />} />
                <Route path="admin/users" element={<UsersPage />} />
              </Route>
              <Route
                path="*"
                element={
                  <>
                    <PageHeading title="Страница не найдена" />
                    <Link to="/">Вернуться в ленту</Link>
                  </>
                }
              />
            </Route>
          </Route>
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  );
}

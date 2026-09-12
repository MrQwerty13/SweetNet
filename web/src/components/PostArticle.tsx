import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { api, errorMessage, json } from '../lib/api';
import { useAuth } from '../lib/auth-context';
import type { Post } from '../lib/types';
import { formatDate } from '../lib/validation';
import { Button, ConfirmDialog } from './ui';
import { ProtectedImage } from './ProtectedImage';
export function PostArticle({ post, onDelete }: { post: Post; onDelete?: (id: string) => void }) {
  const { user } = useAuth();
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();
  const own = user?.id === post.author.id;
  async function remove() {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      await api<void>(`/posts/${encodeURIComponent(post.id)}`, json('DELETE'));
      setConfirm(false);
      if (onDelete) onDelete(post.id);
      else navigate('/', { replace: true });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <article className="post-article">
      <header className="post-meta">
        <div className="author-meta">
          <strong>{post.author.display_name}</strong>
          <span className="muted">@{post.author.username}</span>
          <span className="meta-dot" aria-hidden="true">
            ·
          </span>
          <Link to={`/posts/${post.id}`} className="post-date">
            <time dateTime={post.created_at}>{formatDate(post.created_at)}</time>
          </Link>
        </div>
        {(own || user?.role === 'owner') && (
          <div className="post-actions">
            {own && <Link to={`/posts/${post.id}/edit`}>Изменить</Link>}
            {own && <span aria-hidden="true">·</span>}
            <Button
              variant="text"
              onClick={() => {
                setError('');
                setConfirm(true);
              }}
            >
              Удалить
            </Button>
          </div>
        )}
      </header>
      {post.body && <p className="post-body">{post.body}</p>}
      {post.images.length > 0 && (
        <div className="post-images">
          {post.images.map((image, index) => (
            <ProtectedImage key={image.id} image={image} index={index} />
          ))}
        </div>
      )}
      {confirm && (
        <ConfirmDialog
          title="Удалить публикацию?"
          confirmLabel="Удалить"
          onConfirm={() => void remove()}
          onCancel={() => setConfirm(false)}
          busy={busy}
          error={error}
        >
          <p>Текст и фотографии будут удалены. Отменить это действие нельзя.</p>
        </ConfirmDialog>
      )}
    </article>
  );
}

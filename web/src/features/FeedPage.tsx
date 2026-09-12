import { useState } from 'react';
import { Link } from 'react-router-dom';
import { api, errorMessage, isAbort } from '../lib/api';
import type { Feed } from '../lib/types';
import { useResource } from '../lib/use-resource';
import { Button, EmptyState, ErrorNotice, Loading, PageHeading, PlusIcon } from '../components/ui';
import { PostArticle } from '../components/PostArticle';
export function FeedPage() {
  const { data, error, loading, retry, setData } = useResource<Feed>('/posts?limit=20');
  const [moreBusy, setMoreBusy] = useState(false);
  const [moreError, setMoreError] = useState('');
  async function loadMore() {
    if (!data?.next_cursor || moreBusy) return;
    setMoreBusy(true);
    setMoreError('');
    try {
      const next = await api<Feed>(
        `/posts?limit=20&cursor=${encodeURIComponent(data.next_cursor)}`,
      );
      setData((current) => {
        if (!current) return current;
        const ids = new Set(current.items.map((post) => post.id));
        return {
          items: [...current.items, ...next.items.filter((post) => !ids.has(post.id))],
          next_cursor: next.next_cursor,
        };
      });
    } catch (err) {
      if (!isAbort(err)) setMoreError(errorMessage(err));
    } finally {
      setMoreBusy(false);
    }
  }
  return (
    <>
      <PageHeading
        title="Наш круг"
        subtitle="Моменты, которыми хочется поделиться."
        action={
          <Link className="button button-primary new-post" to="/posts/new">
            <PlusIcon />
            Новый пост
          </Link>
        }
      />
      <p className="feed-privacy">Только для участников круга</p>
      <div className="feed-list">
        {loading && <Loading />}
        <ErrorNotice message={error} retry={retry} />
        {data && data.items.length === 0 && !loading && (
          <EmptyState title="Здесь пока нет публикаций">
            Поделитесь первым моментом с вашим кругом.
          </EmptyState>
        )}
        {data?.items.map((post) => (
          <PostArticle
            key={post.id}
            post={post}
            onDelete={(id) =>
              setData((current) =>
                current
                  ? { ...current, items: current.items.filter((item) => item.id !== id) }
                  : current,
              )
            }
          />
        ))}
      </div>
      <ErrorNotice message={moreError} />
      {data?.next_cursor && (
        <div className="pagination">
          <Button variant="secondary" onClick={() => void loadMore()} disabled={moreBusy}>
            {moreBusy ? 'Загрузка…' : 'Показать ещё'}
          </Button>
        </div>
      )}
    </>
  );
}

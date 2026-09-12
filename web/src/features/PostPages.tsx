import { useEffect, useRef, useState, type ChangeEvent, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api, errorMessage, json } from '../lib/api';
import { useAuth } from '../lib/auth-context';
import type { Post } from '../lib/types';
import { textLength } from '../lib/validation';
import { useResource } from '../lib/use-resource';
import { Button, ErrorNotice, Loading, PageHeading } from '../components/ui';
import { PostArticle } from '../components/PostArticle';
export function PostPage({ edit = false }: { edit?: boolean }) {
  const { id = '' } = useParams();
  const { user } = useAuth();
  const { data, loading, error, retry } = useResource<Post>(`/posts/${encodeURIComponent(id)}`);
  if (loading) return <Loading />;
  if (error) return <><Link to="/">Вернуться в ленту</Link><ErrorNotice message={error} retry={retry} /></>;
  if (!data) return null;
  if (edit) return data.author.id === user?.id ? <PostEditor key={data.id} post={data} /> : <><PageHeading title="Недостаточно прав" /><p>Изменить текст может только автор публикации.</p><Link to={`/posts/${id}`}>Вернуться к публикации</Link></>;
  return <><PageHeading title="Публикация" action={<Link className="button button-secondary" to="/">В ленту</Link>} /><div className="feed-list"><PostArticle post={data} /></div></>;
}
interface SelectedImage { id: string; file: File; url: string }
export function PostEditor({ post }: { post?: Post }) {
  const [body, setBody] = useState(post?.body ?? '');
  const [images, setImages] = useState<SelectedImage[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const urls = useRef(new Set<string>());
  const navigate = useNavigate();
  const count = textLength(body);
  const hasImages = post ? post.images.length > 0 : images.length > 0;
  useEffect(() => { const current = urls.current; return () => current.forEach(url => URL.revokeObjectURL(url)); }, []);
  function selectImages(event: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.target.files ?? []); event.target.value = ''; setError('');
    if (images.length + files.length > 4) { setError('Можно прикрепить не больше 4 фотографий.'); return; }
    if (files.some(file => !['image/jpeg', 'image/png'].includes(file.type))) { setError('Выберите фотографии в формате JPEG или PNG.'); return; }
    if (files.some(file => file.size > 10 * 1024 * 1024)) { setError('Каждый файл должен быть не больше 10 MiB.'); return; }
    setImages(previous => [...previous, ...files.map(file => { const url = URL.createObjectURL(file); urls.current.add(url); return { id: crypto.randomUUID(), file, url }; })]);
  }
  function removeImage(id: string) {
    const image = images.find(item => item.id === id);
    if (image) { URL.revokeObjectURL(image.url); urls.current.delete(image.url); }
    setImages(current => current.filter(item => item.id !== id));
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (busy) return;
    if (count > 5000 || (!body.trim() && !hasImages)) { setError(count > 5000 ? 'В публикации может быть до 5000 символов.' : 'Добавьте текст или хотя бы одну фотографию.'); return; }
    setBusy(true); setError('');
    try {
      let result: Post;
      if (post) result = await api<Post>(`/posts/${encodeURIComponent(post.id)}`, json('PATCH', { body }));
      else { const form = new FormData(); form.append('body', body); images.forEach(image => form.append('images', image.file)); result = await api<Post>('/posts', { method: 'POST', body: form }); }
      navigate(`/posts/${result.id}`, { replace: true });
    } catch (err) { setError(errorMessage(err)); } finally { setBusy(false); }
  }
  return <><PageHeading title={post ? 'Изменить публикацию' : 'Новый пост'} />
    <form onSubmit={submit}><fieldset disabled={busy}><div className="field"><label htmlFor="post-body">Текст публикации</label><textarea id="post-body" value={body} onChange={e => setBody(e.target.value)} rows={7} aria-describedby="body-count" aria-invalid={count > 5000} placeholder="Чем хотите поделиться?" /><span id="body-count" className={`hint text-counter ${count > 5000 ? 'invalid' : ''}`}>{count} / 5000 символов</span></div>
    {post ? <p className="hint">Фотографии сохранятся. После публикации можно изменить только текст.</p> : <section className="upload-section" aria-labelledby="photos-label"><h2 id="photos-label">Фотографии</h2><p className="hint" id="photos-hint">До 4 фото, JPEG или PNG, до 10 MiB каждое и до 25 мегапикселей. Можно опубликовать только фотографии.</p><label className={`button button-secondary file-picker ${images.length >= 4 ? 'disabled' : ''}`}><input type="file" accept="image/jpeg,image/png" multiple onChange={selectImages} disabled={busy || images.length >= 4} aria-label="Добавить фотографии" aria-describedby="photos-hint" />Добавить фотографии</label>
    {images.length > 0 && <div className="image-previews">{images.map((image, index) => <figure key={image.id}><img src={image.url} alt={`Предпросмотр фотографии ${index + 1}`} /><figcaption><span>{image.file.name}</span><Button variant="text" type="button" onClick={() => removeImage(image.id)} aria-label={`Убрать фотографию ${index + 1}`}>Убрать</Button></figcaption></figure>)}</div>}</section>}
    <ErrorNotice message={error} /><div className="form-actions"><Button type="submit">{busy ? 'Сохраняем…' : post ? 'Сохранить изменения' : 'Опубликовать'}</Button><Button type="button" variant="secondary" onClick={() => navigate(post ? `/posts/${post.id}` : '/')}>Отмена</Button></div>
    </fieldset></form></>;
}

import { useEffect, useState } from 'react';
import { errorMessage, isAbort, mediaBlob } from '../lib/api';
import type { PostImage } from '../lib/types';
import { Button } from './ui';
export function ProtectedImage({ image, index }: { image: PostImage; index: number }) {
  const [url, setUrl] = useState('');
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    let objectUrl = '';
    mediaBlob(image.id, controller.signal)
      .then((blob) => {
        if (!controller.signal.aborted) {
          objectUrl = URL.createObjectURL(blob);
          setUrl(objectUrl);
        }
      })
      .catch((err) => {
        if (!controller.signal.aborted && !isAbort(err)) setError(errorMessage(err));
      });
    return () => {
      controller.abort();
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [image.id, revision]);
  if (error)
    return (
      <div className="image-error" role="alert">
        <p>Не удалось загрузить фото. {error}</p>
        <Button
          variant="secondary"
          onClick={() => {
            setError('');
            setRevision((n) => n + 1);
          }}
        >
          Повторить загрузку фото
        </Button>
      </div>
    );
  return url ? (
    <a href={url} target="_blank" rel="noreferrer" aria-label={`Открыть фото ${index + 1}`}>
      <img
        className="post-image"
        src={url}
        width={image.width}
        height={image.height}
        alt={`Фотография ${index + 1} из публикации`}
      />
    </a>
  ) : (
    <div className="image-loading" role="status">
      Загрузка фотографии…
    </div>
  );
}

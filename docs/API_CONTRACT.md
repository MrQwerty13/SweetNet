# API: договор frontend/backend

Все пути из IMPLEMENTATION_PLAN.md. Успешный ответ объекта — сам объект, без `data`.

- User: `{id, username, display_name, role: 'owner'|'member', is_active, created_at}`.
- Login/register: User и Set-Cookie; GET/PATCH me: User.
- Register JSON: `{token, username, display_name, password}`. Login: `{username,password}`.
- Password JSON: `{current_password,new_password}`; ответ 204, все сессии отозваны.
- Post: `{id,author:{id,username,display_name},body,created_at,updated_at,images:[{id,url,width,height}]}`.
- Лента: `{items:Post[],next_cursor:string|null}`. POST multipart: `body` и повторяющийся `images`; ответ Post (201). PATCH JSON `{body}` возвращает Post.
- GET admin/users: `{items:User[]}`; PATCH `{is_active:boolean}` возвращает User.
- Invite: `{id,created_at,expires_at,used_at:string|null,revoked_at:string|null}`.
- GET admin/invites: `{items:Invite[]}`; POST без тела: Invite + `url` (201), секрет только в этом ответе. Ссылка `/join#token=...`. DELETE: 204.
- DELETE post/logout: 204. Ошибка: `{error:{code,message,fields?}}`.
- Cookie HttpOnly, `credentials:'same-origin'`; все mutation требуют точный Origin (браузер отправляет сам). Никакого localStorage для приватных данных.
- Для неизвестных полей JSON сервер возвращает 400. Ограничения текста считаются Unicode code points.
- 401 очищает профиль и приватные страницы; формы входа показывают полученную ошибку. Приватные фото только `/api/v1/media/{id}`.

export interface User {
  id: string; username: string; display_name: string;
  role: 'owner' | 'member'; is_active: boolean; created_at: string;
}
export interface PostImage { id: string; url: string; width: number; height: number }
export interface Post {
  id: string; author: Pick<User, 'id' | 'username' | 'display_name'>;
  body: string; created_at: string; updated_at: string; images: PostImage[];
}
export interface Feed { items: Post[]; next_cursor: string | null }
export interface Invite {
  id: string; created_at: string; expires_at: string;
  used_at: string | null; revoked_at: string | null;
}

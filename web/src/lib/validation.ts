export const textLength = (value: string) => Array.from(value).length;
export const validUsername = (value: string) => /^[a-z0-9_]{3,32}$/.test(value.toLowerCase());
export const validPassword = (value: string) => textLength(value) >= 12 && textLength(value) <= 128;
export const validDisplayName = (value: string) => value.trim().length > 0 && textLength(value) <= 80;
export const formatDate = (value: string) => new Intl.DateTimeFormat('ru-RU', {
  day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit',
}).format(new Date(value));

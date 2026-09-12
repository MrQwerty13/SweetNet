// Capture once, before React/Router. Never persist the invitation secret.
let invitationToken = '';
if (window.location.pathname === '/join') {
  invitationToken = new URLSearchParams(window.location.hash.slice(1)).get('token') ?? '';
  if (window.location.hash) window.history.replaceState(null, '', window.location.pathname + window.location.search);
}
export const getInvitationToken = () => invitationToken;
export function clearInvitationToken() { invitationToken = ''; }

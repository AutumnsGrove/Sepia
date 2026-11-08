// The new-session route is client-only: it asks the service for a session and replaces the URL with it, which a
// static build cannot do ahead of time.
export const ssr = false;
export const prerender = false;

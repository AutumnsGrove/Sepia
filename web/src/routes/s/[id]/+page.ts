// The session screen is client-only. The static build serves index.html for any extensionless path, and the app's
// router picks the id out of the URL.
export const ssr = false;
export const prerender = false;

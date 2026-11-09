// The whole app is one client-side app: every screen reads from the service on mount, so there is nothing worth
// rendering on the server, and a prerendered page would only be the loading state with the data still to come. This
// keeps the build to one fallback document, which is what the static handler serves for any route.
export const ssr = false;
export const prerender = false;

// The About page is static, keyword-rich content — the one route we prerender to
// real HTML so search engines (and no-JS crawlers) get indexable text without
// executing the SPA. Overrides the app-wide ssr=false / prerender=false defaults.
export const ssr = true;
export const prerender = true;

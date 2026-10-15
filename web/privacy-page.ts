import { type ComponentType, createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { createServer, type Plugin } from "vite";

function document(Page: ComponentType, styles: string[]) {
  const body = renderToStaticMarkup(
    createElement(
      "div",
      { className: "min-h-screen" },
      createElement(
        "header",
        { className: "border-b" },
        createElement(
          "nav",
          {
            className: "mx-auto flex max-w-5xl items-center gap-6 px-6 py-4",
            "aria-label": "Main navigation",
          },
          createElement("a", { href: "/", className: "font-semibold" }, "ADX"),
          createElement("a", { href: "/projects", className: "text-muted-foreground" }, "Projects"),
        ),
      ),
      createElement("main", { className: "mx-auto max-w-5xl px-6 py-8" }, createElement(Page)),
      createElement(
        "footer",
        { className: "border-t" },
        createElement(
          "div",
          { className: "mx-auto max-w-5xl px-6 py-4 text-sm text-muted-foreground" },
          "ADX · WokeWorks",
        ),
      ),
    ),
  );
  const links = styles.map((href) => `<link rel="stylesheet" href="${href}">`).join("");
  return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Privacy policy | ADX</title><meta name="description" content="How ADX uses connected Google, Meta and CRM data, and how to request deletion.">${links}</head><body>${body}</body></html>`;
}

// The same React page serves app navigation and crawler-readable HTML.
export function privacyPage(): Plugin {
  return {
    name: "adx-privacy-page",
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        const pathname = new URL(req.url ?? "/", "http://localhost").pathname;
        if (!["/privacy", "/privacy/", "/privacy.html"].includes(pathname)) return next();
        if (req.method !== "GET" && req.method !== "HEAD") return next();
        try {
          const { PrivacyPage } = await server.ssrLoadModule("/src/routes/privacy.tsx");
          res.setHeader("Content-Type", "text/html; charset=utf-8");
          res.end(
            req.method === "HEAD" ? undefined : document(PrivacyPage, ["/src/index.css?direct"]),
          );
        } catch (error) {
          next(error);
        }
      });
    },
    async generateBundle(_, bundle) {
      const server = await createServer({
        configFile: false,
        root: import.meta.dirname,
        cacheDir: "node_modules/.vite-privacy-render",
        optimizeDeps: { noDiscovery: true },
        server: { middlewareMode: true },
        appType: "custom",
        logLevel: "error",
      });
      try {
        const { PrivacyPage } = await server.ssrLoadModule("/src/routes/privacy.tsx");
        const styles = Object.values(bundle)
          .filter((file) => file.type === "asset" && file.fileName.endsWith(".css"))
          .map((file) => `/${file.fileName}`);
        const source = document(PrivacyPage, styles);
        this.emitFile({ type: "asset", fileName: "privacy/index.html", source });
        this.emitFile({ type: "asset", fileName: "privacy.html", source });
      } finally {
        await server.close();
      }
    },
  };
}

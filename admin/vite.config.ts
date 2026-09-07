import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";
import tailwindcss from "@tailwindcss/vite";
import { sentryVitePlugin } from "@sentry/vite-plugin";

// Source-map upload, and nothing else, is what the Sentry plugin does here.
//
// It is never a required build step: a fork, a self-host build or a local
// `pnpm build` sets none of these and the plugin is simply not in the plugin
// list, so nothing needs a Sentry account and nothing is uploaded. CI passes
// the token as a build secret only for the hosted release.
//
// Source maps are emitted only in that case too, so the shipped bundle is
// unchanged for everybody else, and `filesToDeleteAfterUpload` keeps the .map
// files out of the image once they have been sent.
const sentryAuthToken = process.env.SENTRY_AUTH_TOKEN;
const sentryOrg = process.env.SENTRY_ORG;
const sentryProject = process.env.SENTRY_PROJECT;
const uploadSourceMaps = Boolean(sentryAuthToken && sentryOrg && sentryProject);

const sentryPlugins = uploadSourceMaps
    ? [
          sentryVitePlugin({
              authToken: sentryAuthToken,
              org: sentryOrg,
              project: sentryProject,
              release: { name: process.env.VITE_SENTRY_RELEASE },
              sourcemaps: { filesToDeleteAfterUpload: ["dist/**/*.map"] },
              telemetry: false,
          }),
      ]
    : [];

export default defineConfig({
    plugins: [
        react(),
        tailwindcss(),
        ...sentryPlugins,
    ],
    build: {
        // Only when they are going to be uploaded: shipping them otherwise
        // would hand every visitor the dashboard's original sources.
        sourcemap: uploadSourceMaps,
    },
    resolve: {
        alias: {
            "@": path.resolve(__dirname, "./src"),
        },
    },
    // Pre-bundle the heaviest deps so the first request doesn't pay
    // the cold-compile cost. Same intent as web/, lighter list because
    // the admin app doesn't pull in tiptap/dnd-kit/etc.
    optimizeDeps: {
        include: [
            "react",
            "react-dom",
            "react-router-dom",
            "axios",
            "@tanstack/react-query",
            "@tanstack/react-query-devtools",
            "lucide-react",
        ],
    },
    server: {
        // Run on a non-default port so it coexists with the dashboard
        // dev server (5173) without a port collision when both are up.
        port: 5174,
        strictPort: false,
        // Permit Tailscale MagicDNS names (and extras via VITE_ALLOWED_HOSTS)
        // when exposed with --host. IPs + localhost are always allowed, so
        // this is inert for normal local dev.
        allowedHosts: [".ts.net", ...(process.env.VITE_ALLOWED_HOSTS?.split(",").filter(Boolean) ?? [])],
    },
});

import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The built app is served by the Go forms service (cmd/forms), which owns
// /f/<publicId> shells, per-form CSP headers and the same-origin /api. The
// dev server proxies /api to a locally running `make forms` service.
export default defineConfig({
    plugins: [react()],
    server: {
        port: 5175,
        proxy: {
            "/api": {
                target: process.env.FORMS_API_URL ?? "http://localhost:8090",
                changeOrigin: true,
            },
        },
    },
});

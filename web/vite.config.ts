import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { VitePWA } from "vite-plugin-pwa";

// bismuth web PWA config.
// Dev server proxies /api -> http://127.0.0.1:9000 (bismuth server).
// In prod, the SPA is served from /usr/share/bismuth/web/ behind aigoproxy.
//
// VitePWA disabled by default in dev (workbox-build can't write the
// service worker to /tmp on this host; set VITE_PWA=1 to enable).

const pwaEnabled = process.env.VITE_PWA === "1";

export default defineConfig({
  plugins: [
    react(),
    ...(pwaEnabled
      ? [
          VitePWA({
            registerType: "autoUpdate",
            manifest: {
              name: "bismuth",
              short_name: "bismuth",
              description: "Multi-agent coding team control",
              theme_color: "#0a0a0a",
              background_color: "#0a0a0a",
              display: "standalone",
              orientation: "any",
              start_url: "/",
              icons: [
                { src: "/icon-192.png", sizes: "192x192", type: "image/png" },
                { src: "/icon-512.png", sizes: "512x512", type: "image/png" },
                { src: "/icon-512-maskable.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
              ],
            },
          }),
        ]
      : []),
  ],
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:9000",
      "/v1": "http://127.0.0.1:9000",
      "/ws": { target: "ws://127.0.0.1:9000", ws: true },
    },
  },
  build: {
    outDir: "dist",
    sourcemap: true,
  },
});

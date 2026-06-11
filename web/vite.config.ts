import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
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
    tailwindcss(),
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
    rollupOptions: {
      output: {
        // P7-f code-splitting: keep the entry chunk lean. Heavy zones
        // (Terminal/Voice) are React.lazy'd, and their vendor deps get
        // dedicated chunks so they never ride along with the entry:
        //   react-vendor — react/react-dom/scheduler + zustand (eager)
        //   xterm        — @xterm/* (loaded with the Terminal zone)
        //   vad          — @ricky0123/vad-web + onnxruntime-web
        //                  (loaded when the Voice zone mounts)
        manualChunks(id: string) {
          // Vite's virtual runtime helpers (\0vite/preload-helper.js
          // etc.) are shared by the entry and every lazy chunk; pin
          // them to the eager vendor chunk, otherwise rolldown may
          // colocate them with a lazy vendor chunk (observed: "vad")
          // and drag ~420 kB into the entry's modulepreload graph.
          if (id.startsWith("\0vite/") || id.includes("vite/preload-helper")) {
            return "react-vendor";
          }
          if (!id.includes("node_modules")) return undefined;
          if (id.includes("@xterm")) return "xterm";
          if (id.includes("@ricky0123") || id.includes("onnxruntime")) return "vad";
          if (
            id.includes("/react/") ||
            id.includes("/react-dom/") ||
            id.includes("/scheduler/") ||
            id.includes("/zustand/")
          ) {
            return "react-vendor";
          }
          return undefined;
        },
      },
    },
  },
});

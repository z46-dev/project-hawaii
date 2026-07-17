import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "node:path";

export default defineConfig({
    root: "src",
    plugins: [react(), tailwindcss()],
    server: {
        port: 3000,
        proxy: {
            "/api": "http://localhost:8000"
        }
    },
    build: {
        outDir: "../dist",
        emptyOutDir: true,
        rollupOptions: {
            input: {
                app: resolve(import.meta.dirname, "src/index.html")
            }
        }
    }
});

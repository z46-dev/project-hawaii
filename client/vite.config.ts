import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const clientDirectory = fileURLToPath(new URL(".", import.meta.url));

export default defineConfig({
    root: "src",
    plugins: [react(), tailwindcss()],
    resolve: {
        alias: {
            "@": resolve(clientDirectory, "src")
        }
    },
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
                app: resolve(clientDirectory, "src/index.html")
            }
        }
    }
});

import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "node:path";

export default defineConfig({
    plugins: [react(), tailwindcss()],
    build: {
        outDir: "dist",
        emptyOutDir: true,
        rollupOptions: {
            input: {
                app: resolve(import.meta.dirname, "index.html"),
                hitboxGenerator: resolve(import.meta.dirname, "hitboxGenerator/index.html"),
                mapEditor: resolve(import.meta.dirname, "mapEditor/index.html"),
                shipEditor: resolve(import.meta.dirname, "shipEditor/index.html"),
                stats: resolve(import.meta.dirname, "stats/index.html"),
                turretVisualizer: resolve(import.meta.dirname, "turretVisualizer/index.html")
            }
        }
    }
});

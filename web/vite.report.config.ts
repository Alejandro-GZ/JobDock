import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  define: { "process.env.NODE_ENV": JSON.stringify("production") },
  resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
  build: {
    emptyOutDir: false,
    outDir: "dist/report",
    cssCodeSplit: false,
    lib: { entry: path.resolve(__dirname, "src/report-runtime.tsx"), formats: ["iife"], name: "JobDockReport", fileName: () => "report.js" },
    rollupOptions: { output: { assetFileNames: asset => asset.name?.endsWith(".css") ? "report.css" : "[name][extname]" } },
  },
});

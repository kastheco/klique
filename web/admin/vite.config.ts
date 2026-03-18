import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  base: "/admin/",
  plugins: [react()],
  server: {
    proxy: {
      "/v1": {
        target: "http://localhost:7433",
        changeOrigin: true,
      },
    },
  },
});

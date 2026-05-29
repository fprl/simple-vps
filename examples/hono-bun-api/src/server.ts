import { Hono } from "hono";
import { serve } from "bun";

const app = new Hono();

app.get("/health", (c) => c.text("ok"));
app.get("/", (c) => c.json({ app: "hono-api", status: "running" }));

serve({
  fetch: app.fetch,
  port: 3000,
  hostname: "0.0.0.0",
});

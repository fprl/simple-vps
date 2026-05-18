import { mkdtempSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { afterEach, describe, expect, test } from "bun:test";
import { main, type CommandRunner } from "../src/cli";

function fixture(): string {
  const root = mkdtempSync(join(tmpdir(), "simple-deploy-setup-test-"));
  writeFileSync(join(root, "bun.lock"), "\n");
  writeFileSync(
    join(root, "simple-deploy.toml"),
    `
name = "api"

[env.production]
server = "admin@100.x.y.z"
path = "/var/apps/api"
runtime = "bun"

[services.web]
command = "bun run src/server.ts"
port = 3000
healthcheck = "/health"

[routes.app]
host = "api.example.com"
type = "proxy"
service = "web"
`,
  );
  return root;
}

afterEach(() => {
  process.exitCode = 0;
});

describe("setup", () => {
  test("prepares the app over SSH using the Simple VPS server API", async () => {
    const root = fixture();
    const commands: string[][] = [];
    const runner: CommandRunner = {
      async run(command) {
        commands.push(command);
        return { code: 0, stdout: "", stderr: "" };
      },
    };

    await main(["setup", "production"], root, { runner });

    expect(process.exitCode).toBeUndefined();
    expect(commands).toEqual([
      ["ssh", "admin@100.x.y.z", "true"],
      ["ssh", "admin@100.x.y.z", "command -v simple-vps"],
      ["ssh", "admin@100.x.y.z", "command -v rsync"],
      ["ssh", "admin@100.x.y.z", "command -v bun"],
      ["ssh", "admin@100.x.y.z", "sudo simple-vps app create api"],
    ]);
  });

  test("points to Simple VPS install when the server API is missing", async () => {
    const root = fixture();
    const errors: string[] = [];
    const originalError = console.error;
    const runner: CommandRunner = {
      async run(command) {
        if (command[2] === "command -v simple-vps") {
          return { code: 1, stdout: "", stderr: "simple-vps not found" };
        }
        return { code: 0, stdout: "", stderr: "" };
      },
    };

    console.error = (message?: unknown) => {
      errors.push(String(message));
    };
    try {
      await main(["setup", "production"], root, { runner });
    } finally {
      console.error = originalError;
    }

    expect(process.exitCode).toBe(1);
    expect(errors.join("\n")).toContain("rerun the Simple VPS install");
  });
});

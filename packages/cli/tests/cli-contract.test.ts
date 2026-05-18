import { afterEach, describe, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { main, type CommandRunner } from "../src/cli";

function writeManifest(root: string) {
  writeFileSync(join(root, "bun.lock"), "\n");
  writeFileSync(
    join(root, "simple-vps.toml"),
    `
name = "api"

[env.production]
server = "admin@100.x.y.z"
path = "/var/apps/api"
runtime = "bun"
`,
  );
}

afterEach(() => {
  process.exitCode = 0;
});

describe("public CLI contract", () => {
  test("help advertises simple-vps commands only", async () => {
    const lines: string[] = [];
    const originalError = console.error;
    console.error = (message?: unknown) => lines.push(String(message));
    try {
      await main(["--help"]);
    } finally {
      console.error = originalError;
    }

    const output = lines.join("\n");
    expect(output).toContain("simple-vps deploy <env> [--dirty] [--include-dotenv]");
    expect(output).not.toContain("simple-deploy");
  });

  test("package exposes the simple-vps binary only", () => {
    const packageJson = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8")) as {
      name?: string;
      bin?: Record<string, string>;
    };

    expect(packageJson.name).toBe("simple-vps");
    expect(packageJson.bin).toEqual({ "simple-vps": "./src/cli.ts" });
  });

  test("check accepts an optional positional env", async () => {
    const root = mkdtempSync(join(tmpdir(), "simple-vps-contract-test-"));
    const output: string[] = [];
    const originalLog = console.log;
    writeManifest(root);

    console.log = (message?: unknown) => output.push(String(message));
    try {
      await main(["check", "production"], root);
    } finally {
      console.log = originalLog;
    }

    expect(process.exitCode).toBe(0);
    expect(output).toEqual(["simple-vps.toml OK (envs: production)"]);
  });

  test("ssh opens the selected env server with passthrough", async () => {
    const root = mkdtempSync(join(tmpdir(), "simple-vps-contract-test-"));
    const commands: Array<{ command: string[]; passthrough: boolean | undefined }> = [];
    const runner: CommandRunner = {
      async run(command, options) {
        commands.push({ command, passthrough: options?.passthrough });
        return { code: 0, stdout: "", stderr: "" };
      },
    };
    writeManifest(root);

    await main(["ssh", "production"], root, { runner });

    expect(process.exitCode).toBe(0);
    expect(commands).toEqual([{ command: ["ssh", "admin@100.x.y.z"], passthrough: true }]);
  });

  test("route list reads routes from the selected host", async () => {
    const root = mkdtempSync(join(tmpdir(), "simple-vps-contract-test-"));
    const output: string[] = [];
    const originalLog = console.log;
    const commands: string[][] = [];
    const runner: CommandRunner = {
      async run(command) {
        commands.push(command);
        return { code: 0, stdout: '{"routes":[]}\n', stderr: "" };
      },
    };
    writeManifest(root);

    console.log = (message?: unknown) => output.push(String(message));
    try {
      await main(["route", "list", "--json"], root, { runner });
    } finally {
      console.log = originalLog;
    }

    expect(process.exitCode).toBe(0);
    expect(commands).toEqual([["ssh", "admin@100.x.y.z", "sudo simple-vps route list --json"]]);
    expect(output).toEqual(['{"routes":[]}']);
  });

  test("host status reads server status from the selected host", async () => {
    const root = mkdtempSync(join(tmpdir(), "simple-vps-contract-test-"));
    const output: string[] = [];
    const originalLog = console.log;
    const commands: string[][] = [];
    const runner: CommandRunner = {
      async run(command) {
        commands.push(command);
        return { code: 0, stdout: "Simple VPS status\n", stderr: "" };
      },
    };
    writeManifest(root);

    console.log = (message?: unknown) => output.push(String(message));
    try {
      await main(["host", "status"], root, { runner });
    } finally {
      console.log = originalLog;
    }

    expect(process.exitCode).toBe(0);
    expect(commands).toEqual([["ssh", "admin@100.x.y.z", "sudo simple-vps status"]]);
    expect(output).toEqual(["Simple VPS status"]);
  });
});

#!/usr/bin/env bun
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

type Dict = Record<string, unknown>;

export type CheckResult = {
  errors: string[];
  warnings: string[];
  envs: string[];
};

export type CommandResult = {
  code: number;
  stdout: string;
  stderr: string;
};

export type CommandRunner = {
  run(command: string[]): Promise<CommandResult>;
};

export type MainOptions = {
  runner?: CommandRunner;
};

const APP_RE = /^[a-z][a-z0-9-]{1,40}$/;
const SERVICE_RE = /^[a-z][a-z0-9-]{0,30}$/;
const HOST_RE =
  /^(?=.{1,253}$)(?!-)[a-z0-9-]{1,63}(?<!-)(?:\.(?!-)[a-z0-9-]{1,63}(?<!-))*$/;
const RUNTIMES = new Set(["bun", "node", "static"]);
const ROUTE_TYPES = new Set(["proxy", "static", "redirect"]);
const RESERVED_SERVICES = new Set(["current", "releases", "shared"]);
const LOCKFILES = ["bun.lock", "bun.lockb", "pnpm-lock.yaml", "package-lock.json", "yarn.lock"];

const defaultRunner: CommandRunner = {
  async run(command) {
    const proc = Bun.spawn(command, {
      stdout: "pipe",
      stderr: "pipe",
    });
    const [stdout, stderr, code] = await Promise.all([
      new Response(proc.stdout).text(),
      new Response(proc.stderr).text(),
      proc.exited,
    ]);
    return { code, stdout, stderr };
  },
};

function isRecord(value: unknown): value is Dict {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function asBoolean(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function asNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function parseToml(path: string): Dict {
  try {
    return Bun.TOML.parse(readFileSync(path, "utf8")) as Dict;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`failed to parse ${path}: ${message}`);
  }
}

function readManifest(root: string): Dict {
  const path = join(root, "simple-deploy.toml");
  if (!existsSync(path)) {
    throw new Error("simple-deploy.toml not found");
  }
  return parseToml(path);
}

function mergeNamed(base: unknown, override: unknown): Record<string, Dict> {
  const merged: Record<string, Dict> = {};
  if (isRecord(base)) {
    for (const [name, value] of Object.entries(base)) {
      if (isRecord(value)) merged[name] = { ...value };
    }
  }
  if (isRecord(override)) {
    for (const [name, value] of Object.entries(override)) {
      if (!isRecord(value)) continue;
      merged[name] = { ...(merged[name] ?? {}), ...value };
    }
  }
  return merged;
}

function effectiveBuild(manifest: Dict, env: Dict): Dict | undefined {
  const base = isRecord(manifest.build) ? manifest.build : undefined;
  const override = isRecord(env.build) ? env.build : undefined;
  if (!base && !override) return undefined;
  return { ...(base ?? {}), ...(override ?? {}) };
}

function effectiveServices(manifest: Dict, env: Dict): Record<string, Dict> {
  return mergeNamed(manifest.services, env.services);
}

function effectiveRoutes(manifest: Dict, env: Dict): Record<string, Dict> {
  return mergeNamed(manifest.routes, env.routes);
}

function validateRelativePath(path: unknown, label: string, errors: string[]) {
  const value = asString(path);
  if (!value) {
    errors.push(`${label} must be a non-empty string`);
    return;
  }
  if (value.startsWith("/") || value.includes("..") || /[*?[\]{}]/.test(value)) {
    errors.push(`${label} must be a relative path without '..' or globs`);
  }
}

function validateBuild(build: Dict | undefined, root: string, errors: string[]) {
  if (!build) return;

  if (!asString(build.command)) errors.push("[build].command is required when [build] is present");
  validateRelativePath(build.output, "[build].output", errors);

  if (build.install !== undefined && asBoolean(build.install) === undefined) {
    errors.push("[build].install must be a boolean");
  }

  if (build.include === undefined) return;
  if (!Array.isArray(build.include)) {
    errors.push("[build].include must be an array of paths");
    return;
  }
  for (const [index, entry] of build.include.entries()) {
    const label = `[build].include[${index}]`;
    validateRelativePath(entry, label, errors);
    if (typeof entry === "string" && !existsSync(join(root, entry))) {
      errors.push(`${label} does not exist: ${entry}`);
    }
  }
}

function validateEnvBlock(name: string, env: Dict, errors: string[]) {
  if (!SERVICE_RE.test(name)) errors.push(`invalid env name: ${name}`);
  if (!asString(env.server)) errors.push(`[env.${name}].server is required`);
  const path = asString(env.path);
  if (!path) errors.push(`[env.${name}].path is required`);
  else if (!path.startsWith("/")) errors.push(`[env.${name}].path must be absolute`);

  const runtime = asString(env.runtime);
  if (!runtime) errors.push(`[env.${name}].runtime is required`);
  else if (!RUNTIMES.has(runtime)) errors.push(`[env.${name}].runtime must be bun, node, or static`);

  const keepReleases = env.keep_releases;
  if (keepReleases !== undefined) {
    const value = asNumber(keepReleases);
    if (value === undefined || !Number.isInteger(value) || value < 1) {
      errors.push(`[env.${name}].keep_releases must be a positive integer`);
    }
  }
}

function validateServices(services: Record<string, Dict>, runtime: string | undefined, errors: string[]) {
  const ports = new Map<number, string>();
  if (runtime === "static" && Object.keys(services).length > 0) {
    errors.push('runtime = "static" cannot declare services');
  }

  for (const [name, service] of Object.entries(services)) {
    if (!SERVICE_RE.test(name)) errors.push(`invalid service name: ${name}`);
    if (RESERVED_SERVICES.has(name)) errors.push(`reserved service name: ${name}`);
    if (!asString(service.command)) errors.push(`[services.${name}].command is required`);

    if (service.port !== undefined) {
      const port = asNumber(service.port);
      if (port === undefined || !Number.isInteger(port) || port < 1 || port > 65535) {
        errors.push(`[services.${name}].port must be an integer in [1, 65535]`);
      } else if (ports.has(port)) {
        errors.push(`[services.${name}].port duplicates [services.${ports.get(port)}].port`);
      } else {
        ports.set(port, name);
      }
      if (!asString(service.healthcheck)) errors.push(`[services.${name}].healthcheck is required when port is set`);
    }

    const timeout = service.healthcheck_timeout;
    if (timeout !== undefined) {
      const value = asNumber(timeout);
      if (value === undefined || value <= 0) errors.push(`[services.${name}].healthcheck_timeout must be positive`);
    }

    const status = service.healthcheck_status;
    if (status !== undefined) {
      const value = asNumber(status);
      if (value === undefined || !Number.isInteger(value) || value < 100 || value > 599) {
        errors.push(`[services.${name}].healthcheck_status must be an HTTP status code`);
      }
    }
  }
}

function validateRoutes(routes: Record<string, Dict>, services: Record<string, Dict>, errors: string[]) {
  for (const [name, route] of Object.entries(routes)) {
    if (!SERVICE_RE.test(name)) errors.push(`invalid route name: ${name}`);
    const host = asString(route.host);
    if (!host) errors.push(`[routes.${name}].host is required`);
    else if (!HOST_RE.test(host.toLowerCase().replace(/\.$/, ""))) errors.push(`[routes.${name}].host is invalid`);

    const type = asString(route.type);
    if (!type) errors.push(`[routes.${name}].type is required`);
    else if (!ROUTE_TYPES.has(type)) errors.push(`[routes.${name}].type must be proxy, static, or redirect`);

    if (type === "proxy") {
      const serviceName = asString(route.service);
      if (!serviceName) {
        errors.push(`[routes.${name}].service is required for proxy routes`);
      } else if (!services[serviceName]) {
        errors.push(`[routes.${name}].service references unknown service: ${serviceName}`);
      } else if (services[serviceName].port === undefined) {
        errors.push(`[routes.${name}].service must reference a service with a port`);
      }
    }

    if (type === "static" && route.root !== undefined) {
      errors.push(`[routes.${name}].root is not configurable in v1`);
    }

    if (type === "redirect") {
      const to = asString(route.to);
      if (!to) errors.push(`[routes.${name}].to is required for redirect routes`);
      else if (!to.startsWith("http://") && !to.startsWith("https://")) {
        errors.push(`[routes.${name}].to must start with http:// or https://`);
      }
    }
  }
}

function lockfilesIn(root: string): string[] {
  return LOCKFILES.filter((file) => existsSync(join(root, file)));
}

function installNeeded(runtime: string | undefined, build: Dict | undefined): boolean {
  if (runtime === "static") return false;
  if (!build) return true;
  return build.install !== false;
}

function requireString(value: unknown, label: string): string {
  const text = asString(value);
  if (!text) throw new Error(`${label} is required`);
  return text;
}

function resolveEnv(manifest: Dict, envName: string): Dict {
  const envs = isRecord(manifest.env) ? manifest.env : {};
  const env = envs[envName];
  if (!isRecord(env)) throw new Error(`env not found: ${envName}`);
  return env;
}

export function checkManifest(root = process.cwd(), envName?: string): CheckResult {
  const manifest = readManifest(root);
  const errors: string[] = [];
  const warnings: string[] = [];

  const name = asString(manifest.name);
  if (!name) errors.push("name is required");
  else if (!APP_RE.test(name)) errors.push("name must match ^[a-z][a-z0-9-]{1,40}$");

  const envs = isRecord(manifest.env) ? manifest.env : {};
  const envNames = Object.keys(envs);
  if (envNames.length === 0) errors.push("at least one [env.<name>] block is required");
  if (envName && !isRecord(envs[envName])) errors.push(`env not found: ${envName}`);

  const selectedEnvNames = envName ? [envName] : envNames;
  const locks = lockfilesIn(root);
  if (locks.length > 1) errors.push(`multiple lockfiles found: ${locks.join(", ")}`);

  for (const selected of selectedEnvNames) {
    const env = envs[selected];
    if (!isRecord(env)) continue;
    validateEnvBlock(selected, env, errors);

    const build = effectiveBuild(manifest, env);
    const services = effectiveServices(manifest, env);
    const routes = effectiveRoutes(manifest, env);
    const runtime = asString(env.runtime);

    validateBuild(build, root, errors);
    validateServices(services, runtime, errors);
    validateRoutes(routes, services, errors);

    if (installNeeded(runtime, build) && locks.length === 0) {
      errors.push(`no lockfile found for env ${selected}`);
    }
  }

  return { errors, warnings, envs: envNames };
}

function printCheckResult(result: CheckResult) {
  for (const warning of result.warnings) console.error(`Warning: ${warning}`);
  if (result.errors.length > 0) {
    for (const error of result.errors) console.error(`Error: ${error}`);
    process.exitCode = 1;
    return;
  }
  const envList = result.envs.length > 0 ? result.envs.join(", ") : "none";
  console.log(`simple-deploy.toml OK (envs: ${envList})`);
}

function inferPackageName(root: string): string {
  const packagePath = join(root, "package.json");
  if (!existsSync(packagePath)) return "my-app";
  try {
    const packageJson = JSON.parse(readFileSync(packagePath, "utf8")) as { name?: unknown };
    const rawName = asString(packageJson.name) ?? "my-app";
    const unscoped = rawName.split("/").pop() ?? rawName;
    return unscoped.toLowerCase().replace(/[^a-z0-9-]/g, "-").replace(/^-+|-+$/g, "") || "my-app";
  } catch {
    return "my-app";
  }
}

function runInit(root: string) {
  const manifestPath = join(root, "simple-deploy.toml");
  if (existsSync(manifestPath)) throw new Error("simple-deploy.toml already exists");
  const name = inferPackageName(root);
  const content = `name = "${name}"

[env.production]
server = "admin@100.x.y.z"
path = "/var/apps/${name}"
runtime = "bun"

[services.web]
command = "bun run src/server.ts"
port = 3000
healthcheck = "/health"

[routes.app]
host = "app.example.com"
type = "proxy"
service = "web"
`;
  writeFileSync(manifestPath, content, { encoding: "utf8", mode: 0o644 });
  console.log("Created simple-deploy.toml");
  console.log("Next:");
  console.log("1. edit simple-deploy.toml");
  console.log("2. simple-deploy setup production");
  console.log("3. simple-deploy deploy production");
}

async function runCommand(runner: CommandRunner, command: string[], failureMessage: string) {
  const result = await runner.run(command);
  if (result.code !== 0) {
    const detail = result.stderr.trim() || result.stdout.trim();
    throw new Error(detail ? `${failureMessage}: ${detail}` : failureMessage);
  }
}

async function runSetup(root: string, envName: string, runner: CommandRunner) {
  const result = checkManifest(root, envName);
  if (result.errors.length > 0) {
    throw new Error(result.errors.join("\n"));
  }

  const manifest = readManifest(root);
  const appName = requireString(manifest.name, "name");
  const env = resolveEnv(manifest, envName);
  const server = requireString(env.server, `[env.${envName}].server`);
  const runtime = requireString(env.runtime, `[env.${envName}].runtime`);

  await runCommand(runner, ["ssh", server, "true"], `SSH failed for ${server}`);
  for (const tool of ["simple-vps", "rsync", runtime]) {
    if (tool === "static") continue;
    const message =
      tool === "simple-vps"
        ? "missing Simple VPS server API; rerun the Simple VPS install"
        : `missing required server tool: ${tool}`;
    await runCommand(runner, ["ssh", server, `command -v ${tool}`], message);
  }
  await runCommand(
    runner,
    ["ssh", server, `sudo simple-vps app create ${appName}`],
    "simple-vps app create failed; rerun the Simple VPS install",
  );
  console.log(`Setup complete for ${appName} (${envName})`);
}

function usage() {
  console.error("Usage:");
  console.error("  simple-deploy init");
  console.error("  simple-deploy check [--env <name>]");
  console.error("  simple-deploy setup <env>");
}

export async function main(argv = process.argv.slice(2), root = process.cwd(), options: MainOptions = {}) {
  process.exitCode = undefined;
  const [command, ...args] = argv;
  const runner = options.runner ?? defaultRunner;
  try {
    if (command === "--help" || command === "-h") {
      usage();
      return;
    }
    if (command === "init") {
      runInit(root);
      return;
    }
    if (command === "check") {
      let env: string | undefined;
      for (let index = 0; index < args.length; index += 1) {
        const arg = args[index];
        if (arg === "--env") {
          env = args[index + 1];
          index += 1;
        } else {
          throw new Error(`unknown argument: ${arg}`);
        }
      }
      printCheckResult(checkManifest(root, env));
      return;
    }
    if (command === "setup") {
      const env = args[0];
      if (!env || args.length > 1) throw new Error("setup requires exactly one env");
      await runSetup(root, env, runner);
      return;
    }
    usage();
    process.exitCode = 1;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`Error: ${message}`);
    process.exitCode = 1;
  }
}

if (import.meta.main) {
  await main();
}

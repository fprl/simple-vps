# OpenVPS Execution Plan

## Goal

- [ ] Deliver a reliable open-source VPS baseline with `install.sh` as the main UX and Ansible as the converge engine.
- [x] Keep scope focused on base installation first; defer custom role extension system until after base stability.

## Phase 1: Foundation Alignment

- [x] Define the v1 contract (what OpenVPS guarantees after a successful run).
- [x] Confirm supported OS matrix (Ubuntu 22.04 and 24.04 for v1).
- [x] Normalize naming and entrypoints (`install.sh`, playbook names, docs consistency).
- [x] Remove or parameterize personal defaults (SSH key path, timezone, host naming).
- [x] Document required vs optional components (for example Tailscale optional in base).

## Phase 2: Installer UX (`install.sh`)

- [x] Create `install.sh` as the canonical user entrypoint.
- [x] Implement mode selection:
- [x] `remote` mode (run from laptop/workstation against remote VPS).
- [x] `local` mode (run on target VPS directly).
- [x] Add auto-detection fallback with explicit override flags.
- [x] Add interactive prompts when required inputs are missing.
- [x] Add non-interactive flags for automation (`--host`, `--user`, `--key`, `--yes`).
- [x] Ensure both modes converge through the exact same Ansible path.
- [x] Add safe preflight checks (SSH reachability, privileges, required binaries).
- [x] Add clear logs and failure messages with next-step hints.

## Phase 3: Ansible Base Hardening

- [ ] Validate bootstrap/apply role order and idempotency.
- [x] Ensure security baseline is always applied in base profile (SSH hardening, UFW, fail2ban, unattended upgrades).
- [x] Keep Tailscale opt-in for base profile.
- [ ] Verify Node.js/PM2/Caddy installation behavior across supported Ubuntu versions.
- [ ] Ensure reruns are safe and do not regress access (admin user and SSH key handling).
- [x] Add `--check` workflow guidance for dry-run usage.

## Phase 4: Configuration Model (Base)

- [ ] Define base config file (`openvps.yml`) schema for v1.
- [ ] Implement precedence rules (CLI > env > config > defaults).
- [ ] Support initial v1 keys:
- [ ] `admin_user`
- [ ] `security.tailscale`
- [ ] optional package/runtime additions (without full custom role system yet)
- [ ] Validate config with friendly errors before execution.

## Phase 5: Documentation and Onboarding

- [x] Keep README focused on the one-command experience and the two run modes.
- [x] Add docs page for remote mode quickstart.
- [x] Add docs page for local mode quickstart.
- [x] Add docs page for security modes (with and without Tailscale).
- [x] Add troubleshooting page (SSH failures, auth issues, package/network failures).
- [x] Add "what changed on your server" audit-oriented summary docs.

## Phase 6: Quality Gates and CI

- [x] Add `ansible-lint` and syntax checks in CI.
- [ ] Add idempotency check in CI (run apply twice and assert no changes on second pass).
- [ ] Add minimal integration smoke test for both installer modes.
- [x] Add shell linting (`shellcheck`) for scripts.
- [ ] Establish release checklist before tagging versions.

## Phase 7: Release and OSS Readiness

- [ ] Create versioning strategy and changelog process.
- [ ] Publish first v1 milestone with clear supported scope.
- [x] Add contribution guide focused on reproducibility and idempotency.
- [ ] Add issue templates for install failures and environment reports.

## Phase 8: Post-v1 Extensions

- [ ] Design custom role extension system after base is stable.
- [ ] Define extension safety contract (ordering, version pinning, validation).
- [ ] Add profile system (`base`, `web`, `ai`, `data`) once extension model is stable.

## Definition of Done (v1)

- [ ] A new user can provision a fresh Ubuntu VPS from either remote or local flow via `install.sh`.
- [ ] Resulting host is secure and developer-ready with documented expected state.
- [ ] Re-running the installer is safe and predictable.
- [x] Core docs match real behavior with no entrypoint ambiguity.

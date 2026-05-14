# Simple Stack

Simple Stack is an opinionated path for running small production apps on boring
VPS infrastructure.

The stack is split into focused packages:

```text
packages/simple-vps
  Harden and prepare an Ubuntu VPS for production apps.

packages/simple-deploy
  Planned native deploy tool for JS/Bun/Node apps using systemd releases.
```

Current working package:

```bash
./install.sh --mode remote --host 203.0.113.10 --ssh-key ~/.ssh/id_ed25519
```

The root installer delegates to [packages/simple-vps](packages/simple-vps).

Read [SIMPLE_STACK_PLAN.md](SIMPLE_STACK_PLAN.md) for the product direction and
[packages/simple-vps/SPEC.md](packages/simple-vps/SPEC.md) for the current VPS
implementation.

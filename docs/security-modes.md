# Security Modes

OpenVPS currently exposes two practical security modes.

## Base Security (default)

- SSH hardening
- UFW with SSH/HTTP/HTTPS allow rules
- fail2ban for SSH
- unattended upgrades

Enabled by default through bootstrap/apply roles.

## Base + Tailscale (opt-in)

Enable with installer:

```bash
./install.sh --mode remote --host 203.0.113.10 --tailscale
```

Or direct Ansible:

```bash
ansible-playbook -i inventory/hosts.ini playbooks/vps-apply.yml \
  -e "target=vps security_enable_tailscale=true"
```

## Recommendation

Keep Tailscale optional in default OSS flow, and enforce it in stricter profiles later.

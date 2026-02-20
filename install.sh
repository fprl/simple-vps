#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

MODE="auto"
TARGET_HOST=""
BOOTSTRAP_USER="root"
SSH_KEY=""
SSH_PUBLIC_KEY_FILE=""
ADMIN_USER="admin"
TIMEZONE="UTC"
LOCALE="en_US.UTF-8"
TAILSCALE="false"
CHECK_MODE="false"
ASSUME_YES="false"
PASSTHROUGH_ARGS=()

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

usage() {
  cat <<USAGE
OpenVPS installer

Usage:
  ./install.sh [options]

Options:
  --mode <local|remote|auto>     Execution mode (default: auto)
  --host <ip-or-hostname>        Target VPS host (required for remote mode)
  --ip <ip-or-hostname>          Alias for --host
  --bootstrap-user <user>        SSH user for bootstrap phase in remote mode (default: root)
  --ssh-key <path>               SSH private key for remote mode
  --ssh-public-key-file <path>   Explicit public key to add for admin user
  --admin-user <name>            Admin user to create/configure (default: admin)
  --timezone <tz>                Server timezone (default: UTC)
  --locale <locale>              Server locale (default: en_US.UTF-8)
  --tailscale                    Enable Tailscale setup
  --no-tailscale                 Disable Tailscale setup (default)
  --check                        Run Ansible in check mode
  --yes                          Non-interactive mode (fail if required values are missing)
  -h, --help                     Show help

Examples:
  ./install.sh --mode remote --host 203.0.113.10 --ssh-key ~/.ssh/id_ed25519 --admin-user dev
  ./install.sh --mode local --admin-user dev --tailscale
USAGE
}

err() {
  echo -e "${RED}Error:${NC} $*" >&2
}

warn() {
  echo -e "${YELLOW}Warning:${NC} $*"
}

info() {
  echo -e "${GREEN}==>${NC} $*"
}

prepare_ansible_env() {
  local ansible_tmp_dir

  if [[ -f "$SCRIPT_DIR/ansible.cfg" ]]; then
    export ANSIBLE_CONFIG="$SCRIPT_DIR/ansible.cfg"
  fi

  ansible_tmp_dir="${ANSIBLE_LOCAL_TEMP:-${TMPDIR:-/tmp}/openvps-ansible-tmp}"
  mkdir -p "$ansible_tmp_dir"
  export ANSIBLE_LOCAL_TEMP="$ansible_tmp_dir"
}

ensure_openvps_layout() {
  local required_files=(
    "$SCRIPT_DIR/playbooks/vps-bootstrap.yml"
    "$SCRIPT_DIR/playbooks/vps-apply.yml"
    "$SCRIPT_DIR/roles/system/tasks/main.yml"
  )
  local file

  for file in "${required_files[@]}"; do
    if [[ ! -f "$file" ]]; then
      err "Required OpenVPS file not found: $file"
      err "Run install.sh from an OpenVPS checkout that includes playbooks and roles."
      exit 1
    fi
  done
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    err "Required command not found: $cmd"
    exit 1
  fi
}

confirm_or_prompt() {
  local var_name="$1"
  local prompt="$2"
  local default_value="${3:-}"
  local current_value="${!var_name:-}"

  if [[ -n "$current_value" ]]; then
    return
  fi

  if [[ "$ASSUME_YES" == "true" ]]; then
    err "$var_name is required in non-interactive mode."
    exit 1
  fi

  if [[ -n "$default_value" ]]; then
    read -r -p "$prompt [$default_value]: " current_value
    current_value="${current_value:-$default_value}"
  else
    read -r -p "$prompt: " current_value
  fi

  if [[ -z "$current_value" ]]; then
    err "$var_name cannot be empty"
    exit 1
  fi

  printf -v "$var_name" '%s' "$current_value"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --mode)
        MODE="${2:-}"
        shift 2
        ;;
      --host|--ip)
        TARGET_HOST="${2:-}"
        shift 2
        ;;
      --bootstrap-user)
        BOOTSTRAP_USER="${2:-}"
        shift 2
        ;;
      --ssh-key)
        SSH_KEY="${2:-}"
        shift 2
        ;;
      --ssh-public-key-file)
        SSH_PUBLIC_KEY_FILE="${2:-}"
        shift 2
        ;;
      --admin-user)
        ADMIN_USER="${2:-}"
        shift 2
        ;;
      --timezone)
        TIMEZONE="${2:-}"
        shift 2
        ;;
      --locale)
        LOCALE="${2:-}"
        shift 2
        ;;
      --tailscale)
        TAILSCALE="true"
        shift
        ;;
      --no-tailscale)
        TAILSCALE="false"
        shift
        ;;
      --check)
        CHECK_MODE="true"
        shift
        ;;
      --yes)
        ASSUME_YES="true"
        shift
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      --)
        shift
        PASSTHROUGH_ARGS=("$@")
        break
        ;;
      *)
        err "Unknown option: $1"
        usage
        exit 1
        ;;
    esac
  done
}

auto_detect_mode() {
  if [[ "$MODE" != "auto" ]]; then
    return
  fi

  if [[ -n "$TARGET_HOST" ]]; then
    MODE="remote"
    return
  fi

  if [[ "$(id -u)" -eq 0 ]] && [[ -f /etc/os-release ]]; then
    MODE="local"
  else
    MODE="remote"
  fi
}

validate_mode() {
  case "$MODE" in
    local|remote)
      ;;
    *)
      err "Invalid mode: $MODE (expected local, remote, or auto)"
      exit 1
      ;;
  esac
}

ensure_ansible_local() {
  require_cmd ansible-playbook
}

ensure_ansible_inplace() {
  if command -v ansible-playbook >/dev/null 2>&1; then
    return
  fi

  require_cmd apt-get
  info "Ansible not found. Installing with apt-get..."
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y
  apt-get install -y ansible
}

pick_default_public_key_from_private_key() {
  if [[ -n "$SSH_PUBLIC_KEY_FILE" ]]; then
    return
  fi

  if [[ -n "$SSH_KEY" ]] && [[ -f "${SSH_KEY}.pub" ]]; then
    SSH_PUBLIC_KEY_FILE="${SSH_KEY}.pub"
  fi
}

read_public_key_file() {
  local out_var="$1"
  local key_contents=""

  if [[ -z "$SSH_PUBLIC_KEY_FILE" ]]; then
    printf -v "$out_var" '%s' ""
    return
  fi

  if [[ ! -f "$SSH_PUBLIC_KEY_FILE" ]]; then
    err "SSH public key file not found: $SSH_PUBLIC_KEY_FILE"
    exit 1
  fi

  key_contents="$(tr -d '\r' < "$SSH_PUBLIC_KEY_FILE" | sed '/^\s*$/d' | head -n 1)"
  if [[ -z "$key_contents" ]]; then
    err "SSH public key file is empty: $SSH_PUBLIC_KEY_FILE"
    exit 1
  fi

  printf -v "$out_var" '%s' "$key_contents"
}

write_extra_vars_file() {
  local file_path="$1"
  local ssh_public_key_value="$2"

  {
    printf 'openvps_admin_user: "%s"\n' "$ADMIN_USER"
    printf 'openvps_timezone: "%s"\n' "$TIMEZONE"
    printf 'openvps_locale: "%s"\n' "$LOCALE"
    printf 'security_enable_tailscale: %s\n' "$TAILSCALE"

    if [[ -n "$ssh_public_key_value" ]]; then
      local escaped_key="${ssh_public_key_value//\'/\'\"\'\"\'}"
      printf 'openvps_ssh_public_keys:\n'
      printf "  - '%s'\n" "$escaped_key"
    else
      printf 'openvps_ssh_public_keys: []\n'
    fi
  } > "$file_path"
}

preflight_ssh() {
  local user="$1"
  local host="$2"
  local key="$3"

  local cmd=(ssh -o BatchMode=yes -o ConnectTimeout=7)
  if [[ -n "$key" ]]; then
    cmd+=( -i "$key" )
  fi

  if ! "${cmd[@]}" "${user}@${host}" "echo connected" >/dev/null 2>&1; then
    err "SSH preflight failed for ${user}@${host}."
    err "Check host, credentials, and key access."
    exit 1
  fi
}

run_local() {
  ensure_ansible_local

  confirm_or_prompt TARGET_HOST "Target VPS host (IP or DNS name)"
  confirm_or_prompt BOOTSTRAP_USER "Bootstrap SSH user" "root"

  if [[ -n "$SSH_KEY" ]] && [[ ! -f "$SSH_KEY" ]]; then
    err "SSH key file not found: $SSH_KEY"
    exit 1
  fi

  pick_default_public_key_from_private_key

  local ssh_public_key_value=""
  read_public_key_file ssh_public_key_value

  info "Running in remote mode against ${TARGET_HOST}"
  preflight_ssh "$BOOTSTRAP_USER" "$TARGET_HOST" "$SSH_KEY"

  local tmp_inventory
  local tmp_vars
  tmp_inventory="$(mktemp)"
  tmp_vars="$(mktemp)"

  trap 'rm -f "$tmp_inventory" "$tmp_vars"' EXIT

  cat > "$tmp_inventory" <<INVENTORY
[target]
target ansible_host=${TARGET_HOST} ansible_python_interpreter=/usr/bin/python3
INVENTORY

  write_extra_vars_file "$tmp_vars" "$ssh_public_key_value"

  local common_args=(
    -i "$tmp_inventory"
    -e "target=target"
    -e "@$tmp_vars"
  )

  local bootstrap_ssh_args=( -u "$BOOTSTRAP_USER" )
  local apply_ssh_args=( -u "$ADMIN_USER" )
  if [[ -n "$SSH_KEY" ]]; then
    bootstrap_ssh_args+=( --private-key "$SSH_KEY" )
    apply_ssh_args+=( --private-key "$SSH_KEY" )
  fi

  local check_args=()
  if [[ "$CHECK_MODE" == "true" ]]; then
    check_args+=( --check )
  fi

  info "Phase 1/2: bootstrap"
  ansible-playbook "${common_args[@]}" "${bootstrap_ssh_args[@]}" "${check_args[@]}" \
    "${PASSTHROUGH_ARGS[@]}" "$SCRIPT_DIR/playbooks/vps-bootstrap.yml"

  info "Phase 2/2: apply"
  if ! ansible-playbook "${common_args[@]}" "${apply_ssh_args[@]}" "${check_args[@]}" \
      "${PASSTHROUGH_ARGS[@]}" "$SCRIPT_DIR/playbooks/vps-apply.yml"; then
    warn "Apply phase as '${ADMIN_USER}' failed; retrying as '${BOOTSTRAP_USER}'."
    ansible-playbook "${common_args[@]}" "${bootstrap_ssh_args[@]}" "${check_args[@]}" \
      "${PASSTHROUGH_ARGS[@]}" "$SCRIPT_DIR/playbooks/vps-apply.yml"
  fi
}

run_inplace() {
  if [[ "$(id -u)" -ne 0 ]]; then
    err "Local mode must run as root."
    exit 1
  fi

  ensure_ansible_inplace

  local ssh_public_key_value=""
  read_public_key_file ssh_public_key_value

  info "Running in local mode on localhost"

  local tmp_inventory
  local tmp_vars
  tmp_inventory="$(mktemp)"
  tmp_vars="$(mktemp)"

  trap 'rm -f "$tmp_inventory" "$tmp_vars"' EXIT

  cat > "$tmp_inventory" <<INVENTORY
[target]
localhost ansible_connection=local ansible_python_interpreter=/usr/bin/python3
INVENTORY

  write_extra_vars_file "$tmp_vars" "$ssh_public_key_value"

  local common_args=(
    -i "$tmp_inventory"
    -e "target=target"
    -e "@$tmp_vars"
  )

  local check_args=()
  if [[ "$CHECK_MODE" == "true" ]]; then
    check_args+=( --check )
  fi

  info "Phase 1/2: bootstrap"
  ansible-playbook "${common_args[@]}" "${check_args[@]}" \
    "${PASSTHROUGH_ARGS[@]}" "$SCRIPT_DIR/playbooks/vps-bootstrap.yml"

  info "Phase 2/2: apply"
  ansible-playbook "${common_args[@]}" "${check_args[@]}" \
    "${PASSTHROUGH_ARGS[@]}" "$SCRIPT_DIR/playbooks/vps-apply.yml"
}

main() {
  parse_args "$@"
  auto_detect_mode
  validate_mode
  prepare_ansible_env
  ensure_openvps_layout

  info "OpenVPS installer starting"
  info "Mode: $MODE"
  info "Admin user: $ADMIN_USER"
  info "Timezone: $TIMEZONE"
  info "Tailscale: $TAILSCALE"

  case "$MODE" in
    remote)
      run_local
      ;;
    local)
      run_inplace
      ;;
  esac

  info "Provisioning complete"
}

main "$@"

Below is a **full requirements/spec doc** for `sshm` as a **dynamic multi-environment session manager** (SSH + Terraform-resolved + AWS SSM + Docker + Kubernetes). It’s written so you can implement it as a package (Go/Rust/Python) and still preserve your current CLI behavior where it makes sense.

---

# SSHM v2 — Dynamic Session Manager

**Requirements & Specification**

## 1. Overview

### 1.1 Purpose

`sshm` is a cross-environment session launcher and inventory manager that allows DevOps engineers to:

* open interactive shells into many target types (VMs, cloud instances via SSM, containers, pods)
* quickly discover targets via an interactive TUI (fzf-like)
* store and share target definitions via inventory files
* support dynamic resolution (Terraform → IP, SSM selectors → instance-id, Kube selectors → pod/container)

### 1.2 Goals

* **One unified “open” workflow** for different environments.
* **Fast interactive selection** (TUI).
* **Strong passthrough** support so advanced users aren’t blocked.
* **Dynamic resolution** that maps “logical identifiers” to real endpoints at runtime.
* **Portable packaging** as a CLI tool.

### 1.3 Non-goals (initially)

* Full secret management (integrate later with OS keychain/Vault/etc.)
* Universal file transfer across all environments (provider-specific only)
* Replacing `ssh_config` entirely
* Full infrastructure discovery (inventory remains the “index”)

---

## 2. Terminology

* **Entry**: an inventory record describing a target.
* **Provider**: adapter implementing a target type (`ssh`, `tf`, `ssm`, `docker`, `kube`).
* **Resolver**: logic that converts a logical target into an actionable one.
* **ExecPlan**: a computed command invocation (argv + env + cwd + tty mode).
* **Meta**: provider-specific key-value settings attached to an entry.

---

## 3. Inventory Specification

### 3.1 Storage layout

* Directory: `~/.ssh/inventory.d`
  Override: `SSHM_INV_DIR`.
* Inventory files: `*.inv`
* Default filebase: `default`
  Override: `SSHM_DEFAULT_FILEBASE`.

### 3.2 Entry line format (canonical)

Tab-separated fields (recommended; whitespace fallback allowed but discouraged):

```
alias<TAB>type<TAB>target<TAB>user?<TAB>port?<TAB>key?<TAB>workdir?<TAB>tags?<TAB>meta?
```

**Fields**

* `alias` (required): unique identifier, regex `^[a-zA-Z0-9._-]+$`
* `type` (required): one of `ssh|tf|ssm|docker|kube`
* `target` (required): provider-specific target string
* `user` (optional): default user (ssh/tf), optional hint for others
* `port` (optional): default port (ssh/tf); for SSM may be used in PF profiles
* `key` (optional/required depending on type)
* `workdir` (optional): used for `cd` and provider context (tf root, kube manifests, etc.)
* `tags` (optional): freeform string (commonly comma-separated)
* `meta` (optional): semicolon-delimited `k=v` pairs (no unescaped semicolons)

**Meta format**

* `k=v;k=v;k=v`
* Keys are `[a-zA-Z0-9_.-]+`
* Values may be URL-escaped if needed (implementation choice). Minimum requirement: values may contain `:` `,` `.` `-` `_` and alphanumerics.

### 3.3 Backwards compatibility (v1 lines)

If a line has **7 fields** matching v1 format:

```
alias key host user? port? workdir? tags?
```

Then v2 parser must interpret as:

* `alias = f1`
* `type = ssh`
* `target = f3`
* `key = f2`
* `user = f4`
* `port = f5`
* `workdir = f6`
* `tags = f7`
* `meta = ""`

### 3.4 Comments and blanks

* Lines beginning with `#` ignored
* Empty lines ignored
* Trailing whitespace ignored

### 3.5 Path handling

* `~` expands to home for `key` and `workdir`
* Relative paths resolved to absolute at time of writing via `add` (recommended)

### 3.6 Alias uniqueness

* Aliases are globally unique across all `*.inv` files.
* On `add`, if alias exists anywhere, it must be removed from all files before writing new entry to the selected filebase.

---

## 4. Provider Types and Semantics

## 4.1 Provider: `ssh`

### 4.1.1 Required fields

* `key` required
* `target` must be host-like: `host`, `ip`, `user@host`, `host:port`, `[ipv6]`, etc.

### 4.1.2 Meta keys (ssh)

* `jump=<alias|host>`: bastion jump; if alias, must resolve to an SSH entry
* `strict=<yes|no|accept-new>`: hostkey checking policy
* `sshopt=<csv>`: extra ssh args (raw tokens), e.g. `-oServerAliveInterval=30,-oIdentitiesOnly=yes`
* `knownhosts=<path>`: override known_hosts path
* `proxycmd=<...>`: optional ProxyCommand string

### 4.1.3 Resolution

* No external resolution required by default.
* If `jump` is alias, resolve it to `user@host` or `host` and apply `-J`.

---

## 4.2 Provider: `tf` (Terraform-backed SSH)

### 4.2.1 Required fields

* `workdir` required (terraform root)
* `key` required
* `target` format:

  * `<terraform_address>:<public|private>`
  * Example: `module.web.aws_instance.app:public`

### 4.2.2 Resolution

* Must resolve to an IP via terraform state:

  * call `terraform -chdir=<workdir> state show -no-color <terraform_address>`
  * parse `public_ip`, `private_ip`
* Mode selects preferred IP:

  * `public` prefers public_ip then private_ip
  * `private` prefers private_ip then public_ip
* If neither exists: hard error

### 4.2.3 Caching

* Resolution results cached by `workdir:address:mode`
* TTL default: 300 seconds, override `SSHM_TF_CACHE_TTL`

### 4.2.4 Meta keys (tf)

* inherits ssh meta keys (`jump`, `strict`, etc.)

---

## 4.3 Provider: `ssm` (AWS Systems Manager)

### 4.3.1 Required fields

* `target` is one of:

  * instance-id: `i-...`
  * selector: `tag:Key=Value` (minimum selector support)

### 4.3.2 Meta keys (ssm)

* `profile=<aws_profile>` (optional)
* `region=<aws_region>` (optional)
* `role=<role_arn>` (optional; see auth requirements)
* `doc=<document_name>` (optional)
* `pf=<localPort:remoteHost:remotePort>` (optional port forward profile)
* `params=<k=v,k=v>` (optional extra document params)

### 4.3.3 Session modes

`sshm open` must support at least:

* **Interactive shell**:

  * `aws ssm start-session --target <instance-id> [--profile] [--region]`
* **Port forward to remote host/port**:

  * uses SSM doc `AWS-StartPortForwardingSessionToRemoteHost` (or equivalent)
  * must accept `pf` format:

    * localPort, remoteHost, remotePort

### 4.3.4 Selector resolution

If target begins with `tag:`:

* resolve instance-id via:

  * `aws ec2 describe-instances` using tag filter
* If multiple instances match:

  * must prompt user via TUI selection (or error in non-interactive unless `--first` is passed)

### 4.3.5 Authentication requirements

* If `profile` provided: pass `--profile`
* If `region` provided: pass `--region`
* If `role` provided: tool must support one of:

  * (A) assuming role internally and exporting temporary env creds for the aws invocation
  * (B) using a named profile that already assumes role (acceptable for MVP)
    **MVP acceptable:** require role handling via profiles; document limitation.

---

## 4.4 Provider: `docker`

### 4.4.1 Required fields

* `target`: container id/name

### 4.4.2 Meta keys

* `ctx=<docker_context>` (optional)
* `sh=<shell>` (default `sh`)
* `user=<user>` (optional)
* `w=<path>` working directory inside container (optional)

### 4.4.3 Behavior

* Interactive shell:

  * `docker [--context ctx] exec -it [--user user] [--workdir w] <container> <shell>`
* If `:: cmd...` provided:

  * run command rather than shell:

    * `docker exec -it ... <container> <cmd...>`

---

## 4.5 Provider: `kube`

### 4.5.1 Required fields

* `target` is one of:

  * explicit pod: `pod/<name>` or `<name>`
  * selector: `label:key=value`

### 4.5.2 Meta keys

* `ctx=<kube_context>` (optional)
* `ns=<namespace>` default `default`
* `c=<container>` (optional)
* `sh=<shell>` default `bash`, fallback `sh`

### 4.5.3 Resolution

If selector:

* resolve pod via:

  * `kubectl get pods -n <ns> -l <label> -o name`
* If multiple pods match:

  * must prompt user via selection UI (or error in non-interactive)

### 4.5.4 Behavior

* Interactive shell:

  * `kubectl [--context ctx] exec -it -n <ns> <pod> [-c container] -- <shell>`
  * must fallback shell if bash not available (implementation detail: test by running `command -v bash` or try bash then sh)
* If `:: cmd...` provided:

  * run command instead of shell

---

## 5. Unified CLI Specification

## 5.1 Commands

### 5.1.1 `sshm open`

**Syntax**

```
sshm open <alias> [target_override?] [tokens...] [-- passthrough...] [:: cmd...]
```

**Behavior**

1. Load entry by alias
2. Parse tokens and passthrough
3. Resolve entry (provider-specific)
4. Build ExecPlan
5. If dry-run mode: print, do not execute
6. Execute with proper TTY forwarding

**Exit codes**

* `0`: success
* `1`: operational error (not found, resolve fail, command fail)
* `2`: usage error
* `3`: validation error (e.g., ssh key missing)

### 5.1.2 `sshm ui [filter]`

* Launch interactive list of entries; selecting an entry triggers actions.

### 5.1.3 `sshm ls [filter] [--tag TAG]`

* List entries (show type, target summary, tags, origin file).

### 5.1.4 `sshm show <alias>`

* Display full entry and resolved target if applicable (best-effort).

### 5.1.5 `sshm add`

Two modes:

* v1-compatible shorthand for ssh:

  ```
  sshm add <alias> <key> <host> [user] [port] [workdir] [tags] [filebase]
  ```
* v2 explicit:

  ```
  sshm add --type <type> --target <target> [--user ...] [--port ...] [--key ...] [--workdir ...] [--tags ...] [--meta ...] [--filebase ...]
  ```

### 5.1.6 `sshm rm <alias>`

* Remove entry from all inventory files.

### 5.1.7 `sshm edit [filebase]`

* Open inv dir or a specific filebase in `$EDITOR`.

### 5.1.8 `sshm cd <alias>`

* If workdir exists, cd there; else cd to key dir for ssh/tf; else no-op or error for others (design choice: no-op with message recommended).

### 5.1.9 `sshm history [N]`

* Show last N opens (alias + timestamp).

### 5.1.10 Provider convenience commands (optional)

* `sshm ssh <alias> ...` → forces ssh provider semantics
* `sshm tf ...`, `sshm ssm ...`, `sshm docker ...`, `sshm kube ...`

---

## 6. Token System (Friendly Flags)

### 6.1 General token grammar

Tokens are parsed left-to-right until `--` or `::`.

### 6.2 Global tokens

* `dry` → print ExecPlan, do not execute
* `wdir=<path>` → override workdir (if provider uses it)
* `tags=...` → informational only (UI filter use)

### 6.3 SSH/TF tokens (must support)

* `L=...` → `-L ...`
* `R=...` → `-R ...`
* `D=...` → `-D ...`
* `J=...` → `-J ...`
* `p=...` / `P=...` → `-p ...`
* `user=...` → user override
* `A|agent` → `-A`
* `a|noagent` → `-a`
* `t|tty` → `-t`
* `T|notty` → `-T`
* `v|v2|v3` → verbosity
* `strict=...` → host key checking policy override
* All unknown tokens become raw ssh args (before dest)

### 6.4 SSM tokens (must support)

* `profile=...`
* `region=...`
* `pf=local:remoteHost:remotePort`
* `doc=...`

### 6.5 Docker tokens (must support)

* `sh=...`
* `user=...`
* `w=...`
* `ctx=...`

### 6.6 Kube tokens (must support)

* `ctx=...`
* `ns=...`
* `c=...`
* `sh=...`

### 6.7 Passthrough semantics

* `--` begins raw passthrough to underlying provider command.
* `::` begins “remote command” payload for providers that support it:

  * ssh: appended after dest
  * docker: becomes exec cmd
  * kube: becomes exec cmd
  * ssm: allowed only if implemented via `AWS-StartInteractiveCommand` (optional); otherwise error with message

---

## 7. UI (TUI) Requirements

### 7.1 Required UI behaviors

* `sshm ui` uses fzf-like fuzzy search and selection.
* List must display:

  * type icon
  * alias
  * target summary
  * key/user/namespace/profile summary (where relevant)
  * tags
  * source filebase

### 7.2 Key bindings (minimum)

* `Enter` → open
* `Ctrl-s` → show details (including resolved endpoint)
* `Ctrl-e` → edit source inventory file
* `Ctrl-d` → cd (workdir if exists)
* `Ctrl-p` → copy full command (dry-run build)
* `Ctrl-h` → copy “primary identifier”:

  * ssh/tf: host/ip
  * ssm: instance-id
  * docker: container id/name
  * kube: pod name
* `Ctrl-t` → “test” (provider-dependent quick validation)
* `?` → toggle preview

### 7.3 Clipboard support

* macOS: `pbcopy`
* Wayland: `wl-copy`
* X11: `xclip`
* Windows: `clip.exe`
  If none: print value to stdout.

---

## 8. Testing & Validation Requirements

### 8.1 Validation on add/open

* alias valid format
* required fields present based on type
* key exists for ssh/tf
* workdir exists for tf (warning allowed on add, error on open)

### 8.2 `test` behavior per provider

* ssh/tf: attempt batchmode connect with timeout
* ssm: verify aws identity or instance reachable:

  * `aws sts get-caller-identity` (profile/region)
  * optionally `aws ssm describe-instance-information --filters Key=InstanceIds,Values=...`
* docker: `docker inspect <container>`
* kube: `kubectl get pod ...`

---

## 9. Logging, Output, and UX

### 9.1 Command echoing

* Before executing, print:

  * `→ <rendered command>`
* Must support `dry` mode that prints without execution.

### 9.2 Errors

* Errors must be:

  * explicit about which provider failed
  * include actionable next steps (e.g., “terraform not installed”, “kubectl context missing”, “aws profile not found”)

### 9.3 Performance requirements

* `ui` list must not resolve dynamic endpoints during listing (no terraform/aws/kube calls)
* Resolution occurs only on:

  * open
  * show (best-effort)
  * copy host (optional best-effort)

---

## 10. Packaging Requirements

### 10.1 Installability

* Provide a single CLI executable `sshm`
* Support:

  * Linux
  * macOS
  * Windows (at least WSL-friendly; native optional)

### 10.2 External dependencies

`sshm` may call external CLIs:

* ssh/scp
* terraform (for tf entries)
* aws (for ssm entries)
* docker (for docker entries)
* kubectl (for kube entries)
* fzf (optional if you implement your own TUI; required if you choose to depend on it)

Tool must:

* detect missing dependency at runtime
* fail with clear message

### 10.3 Config/env vars

* `SSHM_INV_DIR`
* `SSHM_DEFAULT_FILEBASE`
* `SSHM_TF_CACHE_TTL`
* `SSHM_FZF_*` (if using fzf)

---

## 11. Security Requirements

* Never store or print private key contents.
* Avoid defaulting to insecure SSH host key policies:

  * ssh provider default should not disable strict checking
  * test may relax checking (as diagnostic only), clearly indicated.
* Do not encourage embedding credentials in inventory meta.
* If printing commands, avoid printing sensitive env by default.

---

## 12. Definition of Done (MVP)

An MVP release of sshm v2 must include:

1. Inventory parsing with v1 compatibility
2. Providers: `ssh`, `tf`, `ssm` (shell + pf), `docker`, `kube`
3. `open`, `ui`, `ls`, `show`, `add`, `rm`, `edit`, `cd`, `history`
4. Friendly tokens + passthrough `--` + `::` where supported
5. Terraform resolution + TTL cache
6. Selector resolution for `ssm tag:` and `kube label:` with interactive disambiguation in UI
7. Clear provider-specific errors and dependency checks

---

## 13. Future Extensions (explicitly out-of-scope for MVP but designed for)

* Plugin providers (user-defined)
* Team-shared inventory sources (git repo overlays)
* SSM file transfer (`push/pull`) via S3 workflows
* Teleport integration
* Vault/1Password integration for keys
* Encrypted inventory fields
* Rich preview panes (show tags, meta, last used, resolve status)

---

If you want, I can also produce:

* a **data model** (structs/interfaces) for Go or Rust that matches this spec,
* a **command matrix** (type × action) showing exactly what is supported,
* and a **migration plan** from your current bash function to packaged CLI while keeping your `.inv` files intact.

Yeah — the spec you have is already strong, but if you want an AI (or any implementer) to build this cleanly without “guessing”, the missing pieces are mostly **edge cases, precedence rules, provider quirks, UX decisions, and testability**. Here’s the extra detail I’d add.

---

## 1) Decide the exact precedence rules (everywhere)

Right now you’ve got “override > tokens > entry defaults” in spirit, but for v2 you want it explicit for *each field* and *each provider*, otherwise implementers make inconsistent choices.

### Global precedence template

For any parameter `P`:

1. CLI explicit flags (if you add them later)
2. `target_override` (where applicable)
3. friendly tokens (`user=`, `p=`, `profile=`, etc.)
4. entry field (`user`, `port`, etc.)
5. entry meta (`meta` defaults) *(or swap 4/5, but choose one)*
6. provider default

Also specify merging rules:

* **ssh options**: append/merge (don’t replace)
* **port**: replace
* **profile/region**: replace
* **workdir**: replace
* **tags**: never change at runtime

---

## 2) Formalize the inventory parsing and escaping rules

An AI will otherwise make assumptions and break you.

### Must specify:

* Tabs are canonical; whitespace fallback is best-effort.
* Meta is `k=v;k=v`:

  * Define whether values can contain `;` and how to escape (e.g., percent-encoding).
  * Define whether quoting is supported (`k="a;b"`).
* How to handle unknown fields:

  * ignore? preserve on rewrite? error?

### Recommend (simple + robust):

* In MVP: **no escaping**, just document: values cannot contain `;`.
* Later: allow `%3B` etc.

---

## 3) Define how “selectors” disambiguate in non-interactive mode

For `ssm tag:` and `kube label:` you need deterministic behavior when *not* in UI.

Add flags:

* `--first` : take first match (stable ordering required)
* `--fail-on-multi` (default): error if multiple matches
* `--pick` : force interactive selection even from CLI (if TTY)

Also define ordering:

* For AWS: order by instance-id or Name tag ascending
* For Kube: order by pod creation timestamp newest/oldest or alphabetical

---

## 4) TTY / subprocess behavior: the most common failure point

If you package this, the hardest bug class is: “it works in my terminal but not in CI / in Windows / in non-TTY”.

Specify:

* `open` requires TTY for interactive shells; otherwise:

  * if `:: cmd` provided, allow non-TTY execution
  * if no cmd and no TTY, error: “interactive session requires a TTY”
* Ensure correct streaming:

  * stdin/stdout/stderr attach directly
  * return provider exit code
* `ui` requires TTY always

---

## 5) Provider-specific quirks the builder must account for

### SSH

* If `target` already has `user@`, that should override entry default user unless `user=` token passed.
* IPv6 parsing rules: support `[addr]:port`.
* Jump host resolution:

  * If `jump=<alias>`, resolve alias to SSH-style destination
  * If jump entry is `tf`, resolve it first.
* Known hosts policies:

  * Decide default (`strict=yes` recommended)
  * Support `accept-new` explicitly (OpenSSH option differs by version; might require `StrictHostKeyChecking=accept-new`)

### Terraform

* Define what resources are supported in MVP:

  * only `aws_instance`? or any resource with `public_ip/private_ip`?
* If tf state is remote and slow, cache is crucial—define:

  * cache file naming
  * cache invalidation (`sshm cache clear`?)
* What happens if terraform isn’t installed:

  * error with message

### AWS SSM

Key details an implementer needs:

* Requires `session-manager-plugin` installed (for `aws ssm start-session` to work).
* Port forwarding uses documents:

  * `AWS-StartPortForwardingSessionToRemoteHost`
  * or `AWS-StartPortForwardingSession`
* Parameter names are exact; define them:

  * `localPortNumber`, `portNumber`, `host` (depending doc)
* If you allow `:: cmd` for SSM, specify approach:

  * either out-of-scope MVP
  * or use `AWS-StartInteractiveCommand` and pass `command=["bash","-lc","..."]`

Also define how to resolve `tag:Key=Value`:

* whether Value is exact match or wildcard
* whether to require `InstanceState=running`

### Docker

* If container not running:

  * fail with message, or offer `docker start`? (MVP: fail)
* If shell missing:

  * try `bash`, fallback `sh` if configured to “auto”

### Kubernetes

* Fallback shell behavior:

  * try `bash -lc`? or `bash`? then fallback `sh`
* Handling multiple containers:

  * if `c` not set and pod has >1 container:

    * error in non-interactive
    * UI should prompt container list

---

## 6) “Command preview” rendering rules (copy/paste UX)

In UI you copy full command. Define:

* how to quote arguments (POSIX shell quoting)
* whether to include env vars in copy output
* whether sensitive values should be redacted (probably yes if you later support secrets)

Also define `dry` output format:

* Always prints **argv array rendered safely** (not just a joined string that breaks on spaces).

---

## 7) Explicit capability matrix (type × action)

This prevents someone implementing unsupported operations incorrectly.

Example:

| Action          | ssh              | tf | ssm                   | docker      | kube                              |
| --------------- | ---------------- | -- | --------------------- | ----------- | --------------------------------- |
| open shell      | ✅                | ✅  | ✅                     | ✅           | ✅                                 |
| open cmd (`::`) | ✅                | ✅  | 🚫 (MVP)              | ✅           | ✅                                 |
| port forward    | ✅ (ssh -L/-R/-D) | ✅  | ✅                     | 🚫 (MVP)    | ✅ (kubectl port-forward) optional |
| scp             | ✅                | ✅  | 🚫                    | 🚫          | ⚠️ (kubectl cp optional)          |
| test            | ✅                | ✅  | ✅ (identity/describe) | ✅ (inspect) | ✅ (get pod)                       |
| show resolved   | ✅                | ✅  | ✅                     | ✅           | ✅                                 |

That table alone saves days.

---

## 8) Config layering (important for real-world use)

You’ll want:

* global config file (optional): `~/.config/sshm/config.(yaml|toml)`
* inventory entries (per-target)
* CLI overrides

Define:

* where config lives per OS
* precedence: CLI > env > config > entry > defaults
* which settings belong globally (e.g., default region, default kube context)

---

## 9) Error taxonomy + messages (so it feels “pro”)

Define error categories with consistent exit codes and messages:

* `E_USAGE` (2): bad args
* `E_NOTFOUND` (10): alias not found
* `E_DEPS` (11): missing dependency (terraform/aws/kubectl/session-manager-plugin)
* `E_RESOLVE` (12): selector resolution failed / ambiguous
* `E_VALIDATE` (13): key missing, invalid entry
* `E_EXEC` (14): underlying command failed

You don’t *need* many codes, but having predictable ones helps scripting.

---

## 10) Testing strategy requirements

An AI builder should know how to test without real AWS/Kube infra.

Require:

* “command building” must be pure and testable (no side effects)
* resolvers are interface-driven so they can be mocked
* golden tests for:

  * parsing lines
  * meta parsing
  * precedence merges
  * exec plan rendering
  * selector ambiguity logic

For integration tests:

* docker can be tested locally (spawn busybox/alpine)
* kube can be tested with kind/minikube if available (optional)
* AWS/terraform can be mocked by injecting command runners

---

## 11) Plugin/provider architecture constraints (if you want extensibility)

If you want “many different environment types” long-term, explicitly specify:

### Provider interface

* `Name()`
* `Supports(action)`
* `Resolve(entry, runtimeOpts) -> Resolved`
* `Build(action, entry, resolved, runtimeOpts) -> ExecPlan`

### Optional external providers

Allow “provider as executable”:

* `sshm-provider-<name>` in PATH
* communicates via JSON over stdin/stdout
* returns ExecPlan JSON

This makes it future-proof without bloating core.

---

## 12) Data hygiene & portability decisions

Decide whether inventory is:

* purely local user config, or
* shareable in repos.

If shareable:

* paths like `~/.ssh/key.pem` aren’t portable.
  Add spec support for:
* `key=@agent` (use agent)
* `key=@env:KEYPATH`
* `workdir=@repo` (resolve relative to git root)

Even if you don’t implement it now, stating it avoids painting yourself into a corner.

---

## 13) A few “small but important” UX rules

* `sshm open <alias>` should be the default action on `<alias>` with no verb (optional).
* `sshm ui` should never block on network calls until the user selects something.
* Always show **what** it’s about to do:

  * `type:ssm profile:prod region:eu-west-1 target:i-...`
* Add `--json` output mode for `ls/show` to enable scripting.

---

Yeah — switching history to JSON (or JSONL) is a good move. It makes it *much* easier to add features later (last-used sorting, per-provider stats, “recent in last 24h”, “most used”, query by tag, etc.) without brittle `awk` parsing.

There are two solid approaches:

## Option A: JSONL event log (recommended)

Append-only file where **each line is one JSON object**.

**Pros**

* Fast, simple, robust (no “rewrite the whole file”)
* Easy to tail/grep
* Works well with rotation
* Low corruption risk (worst case: last line partial)

**Cons**

* Aggregations require reading multiple lines (but you can keep a tiny cache/index if you want)

### File

* `~/.ssh/inventory.d/history.jsonl` (or `~/.config/sshm/history.jsonl` if you move config later)

### Event schema (per line)

```json
{
  "ts": 1739465332,
  "alias": "prod-web",
  "type": "ssh",
  "action": "open",
  "resolved": {
    "target": "1.2.3.4",
    "user": "ubuntu",
    "port": 22
  },
  "meta": {
    "filebase": "terraform",
    "tags": "prod,web",
    "workdir": "/home/seb/repo/infra"
  },
  "cmd": {
    "kind": "ssh",
    "argv": ["ssh","-i","/home/.../prod.pem","ubuntu@1.2.3.4"]
  },
  "status": "ok",
  "exit_code": 0,
  "duration_ms": 842
}
```

**Notes**

* `resolved` is *crucial* for tf/ssm/kube: you log what it resolved to at runtime.
* `cmd.argv` is optional but extremely useful for debugging; you can disable it with a privacy setting.

### Rotation / retention

Instead of “keep last N lines” by `tail` (which becomes expensive as file grows), do:

* Keep `SSHM_HISTORY_MAX_EVENTS` (e.g. 2000)
* Or keep `SSHM_HISTORY_MAX_DAYS` (e.g. 30)
* On each write, *occasionally* compact (e.g. 1% of runs) to avoid constantly rewriting.

Compaction strategy:

* Read file, keep only newest N events, write to temp, atomic rename.

### Reading “recent aliases”

Just parse last chunk:

* `tail -n 500 history.jsonl | jq -r 'select(.action=="open")|.alias' | sort -u`
  (Implement in-code without jq, obviously.)

---

## Option B: Single JSON document (not recommended for append-only)

One file containing a list, e.g. `history.json`:

```json
{ "version": 1, "events": [ ... ] }
```

**Pros**

* Easy to load into memory and query

**Cons**

* You have to rewrite the whole file on every update (or implement locking/atomic writes)
* Higher chance of corruption if interrupted mid-write

If you do choose this, you *must* do atomic writes:

* write `history.json.tmp`
* `fsync`
* rename over original

---

## What I’d add to your spec (history requirements)

### Config

* `SSHM_HISTORY_FILE` default `~/.ssh/inventory.d/history.jsonl`
* `SSHM_HISTORY_MAX_EVENTS` default `2000`
* `SSHM_HISTORY_RECORD_CMD` default `false` (privacy-first)
* `SSHM_HISTORY_RECORD_RESOLVED` default `true`

### Must log on these actions

* `open` (and provider-specific opens like `ssh`, `ssm`, `docker`, `kube`)
* `scp` (for ssh/tf)
* `test`
* `ui` selection (optional; could be noisy)

### Must record

* timestamp (`ts`, unix seconds)
* alias
* type/provider
* action
* status + exit_code (if executed)
* duration_ms (nice for diagnosing slow terraform/ssm)
* resolved target for dynamic providers

### Optional record

* argv (disabled by default)
* environment hints (profile/region/context) — not secrets

---

## Extra nice thing JSON enables

You can compute a better “rank” for UI:

* weight recency (last 24h)
* weight frequency (last 30 days)
* optionally weight “success rate” (avoid dead hosts)

So the UI feels “smart” without guessing.


Old version

# ============================== sshm (SSH Manager) ============================== # Inventory-driven SSH/SCP manager with fzf TUI, key-directory jump, and # friendly ssh-arg shorthands + full passthrough via --. # # Inventory directory: # ~/.ssh/inventory.d/*.inv # # Inventory line format (TAB-separated recommended): # alias<TAB>key_path<TAB>host_or_ip<TAB>default_user(optional)<TAB>default_port(optional)<TAB>workdir(optional)<TAB>tags(optional) # # Terraform host token format (stored in host_or_ip field): # tf:<terraform_address>:<public|private> # Example: # mybox ~/.ssh/key.pem tf:module.web.aws_instance.app:public ubuntu 22 ~/repo/infra prod,web # NOTE: Use tabs, not spaces, if key paths contain spaces. # sshm add ... writes proper tab-separated lines automatically. # # Terraform mode: # sshm tf [tfdir] [filebase] # - reads terraform state for aws_instance resources # - lets you pick an instance + pick whether to use public/private IP # - lets you pick which PEM/key file to use # - writes an inventory entry where workdir=tfdir, so sshm cd <alias> cds to the terraform dir # ============================================================================== sshm() { # Constants local INV_DIR="${SSHM_INV_DIR:-$HOME/.ssh/inventory.d}" local SEP=$'\x1f' # non-whitespace delimiter so empty fields are preserved local DEFAULT_FILEBASE="${SSHM_DEFAULT_FILEBASE:-default}" local TF_CACHE_TTL="${SSHM_TF_CACHE_TTL:-300}" # 5 minutes local HISTORY_FILE="$INV_DIR/.history" local HISTORY_MAX="${SSHM_HISTORY_MAX:-100}" # UI Configuration local FZF_HEIGHT="${SSHM_FZF_HEIGHT:-85%}" local FZF_PROMPT="${SSHM_FZF_PROMPT:-sm> }" local FZF_PREVIEW="${SSHM_FZF_PREVIEW:-true}" local FZF_PREVIEW_WINDOW="${SSHM_FZF_PREVIEW_WINDOW:-right:40%:wrap}" local FZF_HEADER="${SSHM_FZF_HEADER:-Enter=ssh | Ctrl-d=cd | Ctrl-h=host | Ctrl-k=key | Ctrl-u=user@host | Ctrl-p=cmd | Ctrl-s=show | Ctrl-e=edit | Ctrl-t=test | ?=help}" mkdir -p "$INV_DIR" # ============================================================================ # Utility Functions # ============================================================================ _sshm_has() { command -v "$1" >/dev/null 2>&1 } _sshm_error() { echo "sshm: $*" >&2 return 1 } _sshm_warn() { echo "sshm: warning: $*" >&2 } _sshm_clip_copy() { local text="$1" # macOS if _sshm_has pbcopy; then printf "%s" "$text" | pbcopy return 0 fi # Wayland if _sshm_has wl-copy; then printf "%s" "$text" | wl-copy return 0 fi # X11 if _sshm_has xclip; then printf "%s" "$text" | xclip -selection clipboard return 0 fi # Windows if _sshm_has clip.exe; then printf "%s" "$text" | clip.exe return 0 fi return 1 } _sshm_inv_file() { local base="${1:-$DEFAULT_FILEBASE}" echo "$INV_DIR/$base.inv" } _sshm_record_usage() { local alias="$1" [[ -n "$alias" ]] || return 0 echo "$(date +%s) $alias" >> "$HISTORY_FILE" 2>/dev/null || return 0 # Keep only last N entries if [[ -f "$HISTORY_FILE" ]]; then tail -n "$HISTORY_MAX" "$HISTORY_FILE" > "${HISTORY_FILE}.tmp" 2>/dev/null && \ mv "${HISTORY_FILE}.tmp" "$HISTORY_FILE" 2>/dev/null || true fi } _sshm_get_recent_aliases() { # Returns list of aliases used in last 24 hours local cutoff=$(($(date +%s) - 86400)) if [[ -f "$HISTORY_FILE" ]]; then awk -v cutoff="$cutoff" '$1 >= cutoff {print $2}' "$HISTORY_FILE" | sort -u fi } _sshm_get_usage_count() { local alias="$1" [[ -f "$HISTORY_FILE" ]] && grep -c "^[0-9]* $alias$" "$HISTORY_FILE" 2>/dev/null || echo "0" } _sshm_get_last_used() { local alias="$1" if [[ -f "$HISTORY_FILE" ]]; then local ts ts=$(grep " $alias$" "$HISTORY_FILE" 2>/dev/null | tail -n1 | awk '{print $1}') if [[ -n "$ts" ]]; then local now=$(date +%s) local diff=$((now - ts)) if [[ $diff -lt 3600 ]]; then echo "$((diff / 60))m ago" elif [[ $diff -lt 86400 ]]; then echo "$((diff / 3600))h ago" else echo "$((diff / 86400))d ago" fi fi fi } # ============================================================================ # Validation Functions # ============================================================================ _sshm_sanitize_alias() { local alias="$1" # Only allow alphanumeric, dash, underscore, dot [[ "$alias" =~ ^[a-zA-Z0-9._-]+$ ]] || return 1 return 0 } _sshm_validate_key() { local key="$1" [[ -z "$key" ]] && { _sshm_error "key path is required" return 1 } [[ -f "$key" ]] || { _sshm_error "key file not found: $key" return 1 } [[ -r "$key" ]] || { _sshm_error "key file not readable: $key" return 1 } # Check key permissions (warn only) if _sshm_has stat; then local perms if [[ "$OSTYPE" == "darwin"* ]]; then perms=$(stat -f %A "$key" 2>/dev/null) else perms=$(stat -c %a "$key" 2>/dev/null) fi if [[ -n "$perms" && "$perms" != "600" && "$perms" != "400" && "$perms" != "644" ]]; then _sshm_warn "key file should have permissions 600 or 400 (current: $perms)" fi fi return 0 } _sshm_validate_entry() { local alias="$1" local key="$2" local host="$3" [[ -z "$alias" ]] && { _sshm_error "alias is required" return 1 } _sshm_sanitize_alias "$alias" || { _sshm_error "invalid alias format (only alphanumeric, dash, underscore, dot allowed)" return 1 } _sshm_validate_key "$key" || return 1 [[ -z "$host" ]] && { _sshm_error "host is required" return 1 } # Warn if alias already exists if _sshm_find "$alias" >/dev/null 2>&1; then _sshm_warn "alias '$alias' already exists (will be overwritten)" fi return 0 } # ============================================================================ # Path Resolution Functions # ============================================================================ _sshm_expand_home() { local path="$1" [[ "$path" == ~* ]] && path="${path/#\~/$HOME}" echo "$path" } _sshm_resolve_path() { local path="$1" path="$(_sshm_expand_home "$path")" # If relative path, make it absolute if [[ "$path" != /* ]]; then if [[ "$path" != */* ]]; then # No slashes - assume current directory path="$(pwd -P)/$path" else # Relative path with slashes path="$(pwd -P)/$path" fi fi echo "$path" } # ============================================================================ # Inventory Functions # ============================================================================ _sshm_collect() { # Output (SEP-separated, 8 fields): # alias SEP key SEP host SEP user SEP port SEP workdir SEP tags SEP file # # Inventory line format (TAB-separated recommended): # alias<TAB>key_path<TAB>host_or_ip_or_tf<TAB>default_user(optional)<TAB>default_port(optional)<TAB>workdir(optional)<TAB>tags(optional) # # Terraform host token stored in host field: # tf:<terraform_state_address>:<public|private> awk -v sep='\037' ' function expand_home(p) { if (p ~ /^~\//) sub(/^~\//, ENVIRON["HOME"] "/", p) else if (p == "~") p = ENVIRON["HOME"] return p } function split_fields(line, out, n) { if (index(line, "\t") > 0) n = split(line, out, "\t") else n = split(line, out, /[[:space:]]+/) return n } FNR==1 { file=FILENAME } /^[[:space:]]*#/ { next } /^[[:space:]]*$/ { next } { n = split_fields($0, f) alias=f[1]; key=f[2]; host=f[3]; user=f[4]; port=f[5]; workdir=f[6]; tags=f[7] if (alias=="" || key=="" || host=="") next key = expand_home(key) workdir = expand_home(workdir) printf "%s%c%s%c%s%c%s%c%s%c%s%c%s%c%s\n", alias,sep,key,sep,host,sep,user,sep,port,sep,workdir,sep,tags,sep,file } ' "$INV_DIR"/*.inv 2>/dev/null } _sshm_find() { local alias="$1" _sshm_collect | awk -v a="$alias" -F'\037' '$1==a {print; exit}' } _sshm_delete_alias_everywhere() { local alias="$1" local f tmp for f in "$INV_DIR"/*.inv; do [[ -f "$f" ]] || continue tmp="${f}.sshm.tmp.$$" awk -v a="$alias" ' /^[[:space:]]*#/ { print; next } /^[[:space:]]*$/ { print; next } { line=$0 if (index(line, "\t")>0) split(line, f, "\t") else split(line, f, /[[:space:]]+/) if (f[1]==a) next print } ' "$f" >"$tmp" && mv "$tmp" "$f" 2>/dev/null || rm -f "$tmp" 2>/dev/null done } # ============================================================================ # Terraform Functions # ============================================================================ _sshm_tf_cache_file() { local key="$1" local cache_dir="${TMPDIR:-/tmp}" # Create hash of key for cache filename if _sshm_has md5sum; then echo "$cache_dir/sshm-tf-$(echo -n "$key" | md5sum | cut -d' ' -f1)" elif _sshm_has md5; then echo "$cache_dir/sshm-tf-$(echo -n "$key" | md5 | cut -d' ' -f1)" else # Fallback: use simple hash echo "$cache_dir/sshm-tf-$(echo -n "$key" | shasum -a 256 2>/dev/null | cut -d' ' -f1 | cut -c1-16)" fi } _sshm_tf_find_root() { local d="${1:-$PWD}" while [[ -n "$d" && "$d" != "/" ]]; do if compgen -G "$d/*.tf" >/dev/null 2>&1 || [[ -f "$d/terraform.tfstate" ]] || [[ -d "$d/.terraform" ]]; then echo "$d" return 0 fi d="$(dirname "$d")" done return 1 } _sshm_tf_list_resources() { # List available aws_instance resources from terraform state local tfdir="${1:-$PWD}" [[ -d "$tfdir" ]] || return 1 _sshm_has terraform || return 1 terraform -chdir="$tfdir" state list 2>/dev/null | grep -E '\.aws_instance\.[^.]*$' || return 1 } _sshm_tf_get_resource_info() { # Get detailed info about a terraform resource local tfdir="$1" local tfaddr="$2" local mode="${3:-public}" [[ -d "$tfdir" ]] || return 1 _sshm_has terraform || return 1 local state_output state_output="$(terraform -chdir="$tfdir" state show -no-color "$tfaddr" 2>/dev/null)" || return 1 local pub_ip priv_ip instance_id instance_type pub_ip="$(awk -F'=' '/^[[:space:]]*public_ip[[:space:]]*=/ {gsub(/[[:space:]]*/,"",$2); gsub(/"/,"",$2); print $2; exit}' <<<"$state_output")" priv_ip="$(awk -F'=' '/^[[:space:]]*private_ip[[:space:]]*=/ {gsub(/[[:space:]]*/,"",$2); gsub(/"/,"",$2); print $2; exit}' <<<"$state_output")" instance_id="$(awk -F'=' '/^[[:space:]]*id[[:space:]]*=/ {gsub(/[[:space:]]*/,"",$2); gsub(/"/,"",$2); print $2; exit}' <<<"$state_output")" instance_type="$(awk -F'=' '/^[[:space:]]*instance_type[[:space:]]*=/ {gsub(/[[:space:]]*/,"",$2); gsub(/"/,"",$2); print $2; exit}' <<<"$state_output")" local ip if [[ "$mode" == "private" ]]; then ip="${priv_ip:-$pub_ip}" else ip="${pub_ip:-$priv_ip}" fi printf "%s|%s|%s|%s" "$ip" "$instance_id" "$instance_type" "$pub_ip" } _sshm_tf_list_detailed() { # List resources with detailed information local tfdir="${1:-$PWD}" local show_details="${2:-false}" [[ -d "$tfdir" ]] || return 1 _sshm_has terraform || return 1 local resources resources="$(_sshm_tf_list_resources "$tfdir")" [[ -z "$resources" ]] && return 1 # Check which resources are already in inventory local existing_aliases existing_aliases="$(_sshm_collect | awk -F'\037' '$3 ~ /^tf:/ {print $3}' | sed 's/^tf://; s/:.*$//')" echo "$resources" | while IFS= read -r resource; do local in_inv="" if echo "$existing_aliases" | grep -q "^${resource}$"; then in_inv="✓" fi if [[ "$show_details" == "true" ]]; then local info info="$(_sshm_tf_get_resource_info "$tfdir" "$resource" "public" 2>/dev/null)" if [[ -n "$info" ]]; then IFS='|' read -r ip instance_id instance_type pub_ip <<<"$info" printf "%-3s %-50s %-15s %-20s %-15s\n" \ "$in_inv" "$resource" "${ip:-<pending>}" "${instance_id:-<unknown>}" "${instance_type:-<unknown>}" else printf "%-3s %-50s %-15s\n" "$in_inv" "$resource" "<no info>" fi else printf "%-3s %s\n" "$in_inv" "$resource" fi done } _sshm_tf_interactive_add() { # Interactive mode for terraform add local tfdir="${1:-}" # Find terraform directory if [[ -z "$tfdir" ]]; then tfdir="$(_sshm_tf_find_root "$PWD")" if [[ -z "$tfdir" ]]; then echo "Terraform directory not found. Please specify:" read -r tfdir [[ -z "$tfdir" ]] && { _sshm_error "Terraform directory required" return 1 } else echo "Found terraform directory: $tfdir" fi fi tfdir="$(_sshm_resolve_path "$tfdir")" # List available resources with details echo "" echo "Available Terraform resources:" local resources resources="$(_sshm_tf_list_resources "$tfdir")" if [[ -z "$resources" ]]; then _sshm_error "No aws_instance resources found in terraform state" echo "Please specify the resource address manually:" read -r tfaddr [[ -z "$tfaddr" ]] && { _sshm_error "Resource address required" return 1 } else # Use fzf if available for better selection with details if _sshm_has fzf; then echo "Legend: ✓ = already in inventory" echo "" tfaddr="$(_sshm_tf_list_detailed "$tfdir" true | fzf \ --height=60% \ --border \ --header="Select terraform resource (✓ = already in inventory)" \ --with-nth=2.. \ --preview="echo 'Resource: {2}'" \ | awk '{print $2}')" [[ -z "$tfaddr" ]] && { echo "Cancelled" return 0 } else echo "Legend: ✓ = already in inventory" echo "" printf "%-3s %-50s %-15s\n" "INV" "RESOURCE" "PUBLIC_IP" echo "────────────────────────────────────────────────────────────" _sshm_tf_list_detailed "$tfdir" true | awk '{printf "%-3s %-50s %-15s\n", $1, $2, $3}' echo "" echo "Select resource number (or enter address manually):" read -r selection if [[ "$selection" =~ ^[0-9]+$ ]]; then tfaddr="$(echo "$resources" | sed -n "${selection}p")" [[ -z "$tfaddr" ]] && { _sshm_error "Invalid selection" return 1 } else tfaddr="$selection" fi fi fi # Prompt for alias (with suggestion from resource name) echo "" local suggested_alias suggested_alias="$(echo "$tfaddr" | awk -F'.' '{print $NF}')" echo "Enter alias name${suggested_alias:+ (suggested: $suggested_alias)}:" read -r alias alias="${alias:-$suggested_alias}" [[ -z "$alias" ]] && { _sshm_error "Alias required" return 1 } _sshm_sanitize_alias "$alias" || { _sshm_error "Invalid alias format (only alphanumeric, dash, underscore, dot allowed)" return 1 } # Check if alias already exists if _sshm_find "$alias" >/dev/null 2>&1; then _sshm_warn "Alias '$alias' already exists" echo "Overwrite? [y/N]" read -r confirm [[ ! "$confirm" =~ ^[Yy]$ ]] && { echo "Cancelled" return 0 } fi # Prompt for key with common locations echo "" echo "Enter SSH key path (e.g., ~/.ssh/key.pem):" local common_keys common_keys="$(find ~/.ssh -name "*.pem" -o -name "id_*" -type f 2>/dev/null | head -5)" if [[ -n "$common_keys" ]]; then echo "Common keys found:" echo "$common_keys" | nl -w2 -s'. ' echo " 0. Enter custom path" echo "" echo "Select key number (or enter path manually):" read -r key_sel if [[ "$key_sel" =~ ^[0-9]+$ && "$key_sel" -gt 0 ]]; then key="$(echo "$common_keys" | sed -n "${key_sel}p")" else read -r key fi else read -r key fi [[ -z "$key" ]] && { _sshm_error "Key path required" return 1 } _sshm_validate_key "$key" || return $? key="$(_sshm_resolve_path "$key")" # Prompt for IP mode echo "" echo "IP mode [public/private] (default: public):" read -r mode mode="${mode:-public}" [[ "$mode" != "public" && "$mode" != "private" ]] && mode="public" # Prompt for user (with common defaults) echo "" echo "SSH user (common: ubuntu, ec2-user, admin, root) (optional, press Enter to skip):" read -r user # Prompt for port echo "" echo "SSH port (default: 22) (optional, press Enter to skip):" read -r port # Prompt for tags echo "" echo "Tags (comma-separated, e.g., prod,web,terraform) (optional, press Enter to skip):" read -r tags # Prompt for filebase echo "" echo "Inventory filebase (default: terraform):" read -r filebase filebase="${filebase:-terraform}" # Show summary echo "" echo "Summary:" echo " Alias : $alias" echo " Key : $key" echo " Resource : $tfaddr" echo " Mode : $mode" echo " User : ${user:-<none>}" echo " Port : ${port:-<none>}" echo " Tags : ${tags:-<none>}" echo " Filebase : $filebase" echo " TF Dir : $tfdir" echo "" echo "Add this entry? [y/N]" read -r confirm [[ ! "$confirm" =~ ^[Yy]$ ]] && { echo "Cancelled" return 0 } # Validate terraform state if ! terraform -chdir="$tfdir" state show "$tfaddr" >/dev/null 2>&1; then _sshm_warn "terraform state show failed for: $tfaddr" echo "Continue anyway? [y/N]" read -r confirm [[ ! "$confirm" =~ ^[Yy]$ ]] && { echo "Cancelled" return 0 } fi # Add entry local f="$(_sshm_inv_file "$filebase")" touch "$f" 2>/dev/null || { _sshm_error "cannot write: $f" return 1 } _sshm_delete_alias_everywhere "$alias" local host="tf:${tfaddr}:${mode}" printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\n" "$alias" "$key" "$host" "$user" "$port" "$tfdir" "$tags" >>"$f" echo "" echo "✓ Added TF entry: $alias → $host (dir: $tfdir) → $f" return 0 } _sshm_tf_resolve_ip() { # args: tfdir tfaddr mode(public|private) local tfdir="$1" local tfaddr="$2" local mode="$3" local cache_key="${tfdir}:${tfaddr}:${mode}" local cache_file="$(_sshm_tf_cache_file "$cache_key")" # Check cache if [[ -f "$cache_file" ]]; then local cache_age if [[ "$OSTYPE" == "darwin"* ]]; then cache_age=$(($(date +%s) - $(stat -f %m "$cache_file" 2>/dev/null || echo 0))) else cache_age=$(($(date +%s) - $(stat -c %Y "$cache_file" 2>/dev/null || echo 0))) fi if [[ $cache_age -lt $TF_CACHE_TTL ]]; then cat "$cache_file" return 0 fi fi _sshm_has terraform || { _sshm_error "terraform not found (needed to resolve tf: hosts)" return 1 } [[ -n "$tfdir" ]] || { _sshm_error "terraform workdir not set for this alias (6th column)" return 1 } [[ -d "$tfdir" ]] || { _sshm_error "terraform dir not found: $tfdir" return 1 } local out out="$(terraform -chdir="$tfdir" state show -no-color "$tfaddr" 2>/dev/null)" || { _sshm_error "terraform state show failed for: $tfaddr (dir: $tfdir)" return 1 } local pub priv pub="$(awk -F'=' '/^[[:space:]]*public_ip[[:space:]]*=/ {gsub(/[[:space:]]*/,"",$2); gsub(/"/,"",$2); print $2; exit}' <<<"$out")" priv="$(awk -F'=' '/^[[:space:]]*private_ip[[:space:]]*=/ {gsub(/[[:space:]]*/,"",$2); gsub(/"/,"",$2); print $2; exit}' <<<"$out")" local chosen="" if [[ "$mode" == "private" ]]; then chosen="${priv:-$pub}" else chosen="${pub:-$priv}" fi [[ -n "$chosen" ]] || { _sshm_error "no public/private IP found in state for $tfaddr" return 1 } # Cache the result echo "$chosen" > "$cache_file" 2>/dev/null || true printf "%s" "$chosen" } _sshm_resolve_inventory_host() { # args: inv_host inv_workdir local inv_host="$1" local inv_workdir="$2" if [[ "$inv_host" == tf:* ]]; then local rest="${inv_host#tf:}" local tfaddr="${rest%:*}" local mode="${rest##*:}" [[ "$mode" == "public" || "$mode" == "private" ]] || mode="public" _sshm_tf_resolve_ip "$inv_workdir" "$tfaddr" "$mode" || return 1 return 0 fi printf "%s" "$inv_host" } # ============================================================================ # SSH/SCP Functions # ============================================================================ _sshm_split_target() { # Input: [user@]host[:port] or [user@][ipv6]:port local in="$1" local user="" local host="" local port="" if [[ "$in" == *@* ]]; then user="${in%@*}" in="${in#*@}" fi if [[ "$in" =~ ^\[(.+)\]:(.+)$ ]]; then host="${BASH_REMATCH[1]}" port="${BASH_REMATCH[2]}" elif [[ "$in" =~ ^([^:]+):([0-9]+)$ ]]; then host="${BASH_REMATCH[1]}" port="${BASH_REMATCH[2]}" else host="$in" fi printf "%s%s%s" "$user" "$SEP" "$host$SEP$port" } _sshm_norm_ssh_parts() { # Uses function-scoped variables with unique names to avoid conflicts # Variables are set in caller's scope via eval for compatibility local __sshm_opts_var="$1" local __sshm_cmd_var="$2" local __sshm_user_var="$3" shift 3 local __sshm_opts=() local __sshm_cmd=() local __sshm_user="" local tok local mode="opts" while (($#)); do tok="$1" shift if [[ "$tok" == "--" ]]; then mode="pass" continue fi if [[ "$tok" == "::" ]]; then mode="cmd" continue fi if [[ "$mode" == "cmd" ]]; then __sshm_cmd+=("$tok") continue fi if [[ "$mode" == "pass" ]]; then __sshm_opts+=("$tok") continue fi case "$tok" in L=*|l=*) __sshm_opts+=(-L "${tok#*=}") ;; R=*|r=*) __sshm_opts+=(-R "${tok#*=}") ;; D=*|d=*) __sshm_opts+=(-D "${tok#*=}") ;; J=*|j=*) __sshm_opts+=(-J "${tok#*=}") ;; p=*|P=*) __sshm_opts+=(-p "${tok#*=}") ;; user=*) __sshm_user="${tok#*=}" ;; A|agent) __sshm_opts+=(-A) ;; a|noagent) __sshm_opts+=(-a) ;; t|tty) __sshm_opts+=(-t) ;; T|notty) __sshm_opts+=(-T) ;; v) __sshm_opts+=(-v) ;; v2) __sshm_opts+=(-vv) ;; v3) __sshm_opts+=(-vvv) ;; *) __sshm_opts+=("$tok") ;; esac done # Export to caller's scope eval "${__sshm_opts_var}=(\"\${__sshm_opts[@]}\")" eval "${__sshm_cmd_var}=(\"\${__sshm_cmd[@]}\")" eval "${__sshm_user_var}=\"\$__sshm_user\"" } _sshm_managed_ssh() { # key inv_host inv_user inv_port inv_workdir [target_override] [friendly] [-- raw] [:: cmd...] local key="$1" local inv_host="$2" local inv_user="$3" local inv_port="$4" local inv_workdir="$5" shift 5 _sshm_validate_key "$key" || return 3 local target="$inv_host" if [[ $# -gt 0 ]]; then case "$1" in -*|L=*|R=*|D=*|J=*|p=*|user=*|A|agent|a|noagent|t|tty|T|notty|v|v2|v3|--|::) ;; *) target="$1" shift ;; esac fi # If target wasn't overridden and inventory host is tf:..., resolve it now if [[ "$target" == "$inv_host" && "$inv_host" == tf:* ]]; then target="$(_sshm_resolve_inventory_host "$inv_host" "$inv_workdir")" || return $? fi local t_user t_host t_port IFS="$SEP" read -r t_user t_host t_port < <(_sshm_split_target "$target") local parsed_opts parsed_cmd user_override _sshm_norm_ssh_parts parsed_opts parsed_cmd user_override "$@" local final_user="" local final_port="" [[ -n "$t_user" ]] && final_user="$t_user" [[ -n "$user_override" ]] && final_user="$user_override" [[ -z "$final_user" ]] && final_user="$inv_user" [[ -n "$t_port" ]] && final_port="$t_port" [[ -z "$final_port" ]] && final_port="$inv_port" local dest="$t_host" [[ -n "$final_user" ]] && dest="$final_user@$t_host" local -a cmd=(ssh -i "$key" -o IdentitiesOnly=yes) [[ -n "$final_port" ]] && cmd+=(-p "$final_port") cmd+=("${parsed_opts[@]}") cmd+=("$dest") cmd+=("${parsed_cmd[@]}") echo "→ ${cmd[*]}" "${cmd[@]}" } _sshm_managed_scp() { # key host user port workdir src dst [-- raw scp args...] local key="$1" local host="$2" local user="$3" local port="$4" local workdir="$5" local src="$6" local dst="$7" shift 7 _sshm_validate_key "$key" || return 3 # Resolve terraform token host at runtime if [[ "$host" == tf:* ]]; then host="$(_sshm_resolve_inventory_host "$host" "$workdir")" || return $? fi local -a opts=() if [[ "$1" == "--" ]]; then shift opts+=("$@") fi local remote_prefix="$host" [[ -n "$user" ]] && remote_prefix="$user@$host" local src_is_remote=0 local dst_is_remote=0 [[ "$src" == :* ]] && src_is_remote=1 [[ "$dst" == :* ]] && dst_is_remote=1 if [[ $src_is_remote -eq 0 && $dst_is_remote -eq 0 ]]; then if [[ -e "$src" ]]; then dst_is_remote=1 elif [[ -e "$dst" ]]; then src_is_remote=1 else dst_is_remote=1 fi fi [[ $src_is_remote -eq 1 ]] && src="${remote_prefix}:${src#:}" [[ $dst_is_remote -eq 1 ]] && dst="${remote_prefix}:${dst#:}" local -a cmd=(scp -i "$key") [[ -n "$port" ]] && cmd+=(-P "$port") cmd+=("${opts[@]}") cmd+=("$src" "$dst") echo "→ ${cmd[*]}" "${cmd[@]}" } _sshm_test_connection() { # key host user port workdir local key="$1" local host="$2" local user="$3" local port="$4" local workdir="$5" _sshm_validate_key "$key" || return 3 # Resolve terraform token host at runtime if [[ "$host" == tf:* ]]; then host="$(_sshm_resolve_inventory_host "$host" "$workdir")" || return $? fi local dest="$host" [[ -n "$user" ]] && dest="$user@$host" local -a cmd=(ssh -i "$key" -o IdentitiesOnly=yes -o ConnectTimeout=5 -o BatchMode=yes -o StrictHostKeyChecking=no) [[ -n "$port" ]] && cmd+=(-p "$port") cmd+=("$dest" "echo 'OK'") echo "Testing connection to $dest..." if "${cmd[@]}" >/dev/null 2>&1; then echo "✓ Connection successful" return 0 else _sshm_error "Connection failed" return 1 fi } # ============================================================================ # Help & Documentation # ============================================================================ _sshm_help() { cat <<'EOF' sshm - inventory-based SSH/SCP manager Inventory line format (TAB-separated): alias<TAB>key<TAB>host_or_ip_or_tf<TAB>user(optional)<TAB>port(optional)<TAB>workdir(optional)<TAB>tags(optional) Terraform host token (stored in host field): tf:<terraform_state_address>:<public|private> Commands: sshm ui [filter] sshm ls [filter] [--tag TAG] sshm add <alias> <key> <host> [user] [port] [workdir] [tags] [filebase] sshm rm <alias> sshm edit [filebase] sshm cd <alias> sshm show <alias> sshm test <alias> sshm export <file> sshm import <file> sshm history [N] Terraform integration: sshm tf - Interactive terraform add wizard sshm tf list [tfdir] [--details] [--fzf] - List terraform resources --details, -d Show detailed info (IPs, instance IDs, types) --fzf, -f Use fzf for selection sshm tf add - Interactive add wizard sshm tf add <alias> <key> <tfaddr> [public|private] [user] [port] [tfdir] [tags] [filebase] Managed SSH: sshm ssh <alias> [target_override] [friendly tokens...] [-- raw ssh args...] [:: remote cmd...] Managed SCP: sshm scp <alias> <src> <dst> [-- raw scp args...] Environment Variables: SSHM_INV_DIR - Inventory directory (default: ~/.ssh/inventory.d) SSHM_DEFAULT_FILEBASE - Default inventory filebase (default: default) SSHM_TF_CACHE_TTL - Terraform cache TTL in seconds (default: 300) SSHM_FZF_HEIGHT - fzf height (default: 85%) SSHM_FZF_PROMPT - fzf prompt (default: sm> ) SSHM_HISTORY_MAX - Max history entries (default: 100) Examples: sshm add prod-web ~/.ssh/prod.pem 1.2.3.4 ubuntu 22 "" prod,web sshm tf add prod-web ~/.ssh/prod.pem module.web.aws_instance.app public ubuntu 22 ~/infra prod terraform sshm ssh prod-web sshm test prod-web sshm cd prod-web sshm ls --tag prod sshm export ~/backup/sshm-inventory.tar.gz EOF } # ============================================================================ # Main Command Dispatcher # ============================================================================ local sub="${1:-ui}" case "$sub" in help|-h|--help) _sshm_help return 0 ;; ls) local filter="${2:-}" local tag_filter="" [[ "$2" == "--tag" ]] && { tag_filter="$3" filter="" } [[ "$3" == "--tag" ]] && tag_filter="$4" local output output="$(_sshm_collect | awk -v FS='\037' -v filter="$filter" -v tag_filter="$tag_filter" ' BEGIN{print "ALIAS\tHOST\tUSER\tPORT\tKEY\tWORKDIR\tTAGS\tFILE"} { if (filter != "" && tolower($1) !~ tolower(filter) && tolower($3) !~ tolower(filter)) next if (tag_filter != "" && tolower($7) !~ tolower(tag_filter)) next u=($4==""?"-":$4); p=($5==""?"-":$5); w=($6==""?"-":$6); t=($7==""?"-":$7); printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",$1,$3,u,p,$2,w,t,$8 } ')" echo "$output" | column -t -s $'\t' return 0 ;; add) # sshm add <alias> <key> <host> [user] [port] [workdir] [tags] [filebase] local alias="$2" local key="$3" local host="$4" local user="$5" local port="$6" local workdir="$7" local tags="$8" local filebase="${9:-$DEFAULT_FILEBASE}" [[ -z "$alias" || -z "$key" || -z "$host" ]] && { _sshm_error "Usage: sshm add <alias> <key> <host> [user] [port] [workdir] [tags] [filebase]" return 2 } _sshm_validate_entry "$alias" "$key" "$host" || return $? # Resolve paths key="$(_sshm_resolve_path "$key")" [[ -n "$workdir" ]] && workdir="$(_sshm_resolve_path "$workdir")" local f="$(_sshm_inv_file "$filebase")" touch "$f" 2>/dev/null || { _sshm_error "cannot write: $f" return 1 } _sshm_delete_alias_everywhere "$alias" printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\n" "$alias" "$key" "$host" "$user" "$port" "$workdir" "$tags" >>"$f" echo "Added: $alias → $f" return 0 ;; tf) # sshm tf [list|add] [args...] local action="${2:-}" # If no action, show help or start interactive mode if [[ -z "$action" ]]; then echo "Terraform integration for sshm" echo "" echo "Commands:" echo " sshm tf list [tfdir] [--details] [--fzf] - List terraform resources" echo " --details, -d Show detailed info (IPs, instance IDs, types)" echo " --fzf, -f Use fzf for selection (returns selected resource)" echo " sshm tf add [args...] - Add terraform resource (interactive if args missing)" echo " sshm tf add <alias> <key> <tfaddr> [public|private] [user] [port] [tfdir] [tags] [filebase]" echo "" echo "Examples:" echo " sshm tf - Interactive add wizard" echo " sshm tf list - List resources (simple)" echo " sshm tf list --details - List with IPs and details" echo " sshm tf list --fzf - Select resource with fzf" echo " sshm tf add - Interactive add wizard" echo " sshm tf add my-server ~/.ssh/key.pem module.web.aws_instance.app" return 0 fi case "$action" in list) local tfdir="${3:-}" local show_details=false local use_fzf=false # Parse options shift 2 while [[ $# -gt 0 ]]; do case "$1" in --details|-d) show_details=true shift ;; --fzf|-f) use_fzf=true shift ;; *) if [[ -z "$tfdir" ]]; then tfdir="$1" fi shift ;; esac done if [[ -z "$tfdir" ]]; then tfdir="$(_sshm_tf_find_root "$PWD")" if [[ -z "$tfdir" ]]; then _sshm_error "Could not find terraform root. Specify directory: sshm tf list [tfdir] [--details] [--fzf]" return 1 fi fi tfdir="$(_sshm_resolve_path "$tfdir")" echo "Terraform resources in: $tfdir" echo "" local resources resources="$(_sshm_tf_list_resources "$tfdir")" if [[ -z "$resources" ]]; then echo "No aws_instance resources found" return 0 fi # Use fzf for selection if requested if [[ "$use_fzf" == "true" ]] && _sshm_has fzf; then local selected if [[ "$show_details" == "true" ]]; then selected="$(_sshm_tf_list_detailed "$tfdir" true | fzf --height=60% --border --header="Select resource (✓ = already in inventory)" --with-nth=2..)" else selected="$(_sshm_tf_list_detailed "$tfdir" false | fzf --height=40% --border --header="Select resource (✓ = already in inventory)" --with-nth=2..)" fi if [[ -n "$selected" ]]; then # Extract resource address (second column) echo "$selected" | awk '{print $2}' fi return 0 fi # Regular listing if [[ "$show_details" == "true" ]]; then echo "Legend: ✓ = already in inventory" echo "" printf "%-3s %-50s %-15s %-20s %-15s\n" "INV" "RESOURCE" "PUBLIC_IP" "INSTANCE_ID" "TYPE" echo "────────────────────────────────────────────────────────────────────────────────────────────" _sshm_tf_list_detailed "$tfdir" true else echo "Legend: ✓ = already in inventory" echo "" printf "%-3s %s\n" "INV" "RESOURCE" echo "────────────────────────────────────────────────────────────" _sshm_tf_list_detailed "$tfdir" false fi return 0 ;; add) local alias="$3" local key="$4" local tfaddr="$5" # If required args missing, start interactive mode if [[ -z "$alias" || -z "$key" || -z "$tfaddr" ]]; then echo "Starting interactive terraform add wizard..." echo "" _sshm_tf_interactive_add "${9:-}" # tfdir is 9th arg return $? fi # Non-interactive mode with all args local mode="${6:-public}" local user="$7" local port="$8" local tfdir="$9" local tags="${10}" local filebase="${11:-terraform}" [[ "$mode" == "public" || "$mode" == "private" ]] || mode="public" # Validate key _sshm_validate_key "$key" || return $? key="$(_sshm_resolve_path "$key")" # Default tfdir: detect from CWD if [[ -z "$tfdir" ]]; then tfdir="$(_sshm_tf_find_root "$PWD")" || { _sshm_error "could not find terraform root (pass tfdir explicitly or use interactive mode)" return 1 } fi tfdir="$(_sshm_resolve_path "$tfdir")" # Validate terraform state exists if ! terraform -chdir="$tfdir" state show "$tfaddr" >/dev/null 2>&1; then _sshm_warn "terraform state show failed for: $tfaddr (dir: $tfdir)" _sshm_warn "entry will be added but IP resolution may fail" fi local f="$(_sshm_inv_file "$filebase")" touch "$f" 2>/dev/null || { _sshm_error "cannot write: $f" return 1 } _sshm_delete_alias_everywhere "$alias" local host="tf:${tfaddr}:${mode}" printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\n" "$alias" "$key" "$host" "$user" "$port" "$tfdir" "$tags" >>"$f" echo "Added TF: $alias → $host (dir: $tfdir) → $f" return 0 ;; *) _sshm_error "Unknown terraform command: $action" echo "Usage: sshm tf [list|add] [args...]" echo "Run 'sshm tf' for interactive mode" return 2 ;; esac ;; rm) local alias="$2" [[ -z "$alias" ]] && { _sshm_error "Usage: sshm rm <alias>" return 2 } _sshm_delete_alias_everywhere "$alias" echo "Removed: $alias" return 0 ;; edit) local filebase="${2:-}" if [[ -z "$filebase" ]]; then "${EDITOR:-vi}" "$INV_DIR" else "${EDITOR:-vi}" "$(_sshm_inv_file "$filebase")" fi return $? ;; cd) local alias="$2" [[ -z "$alias" ]] && { _sshm_error "Usage: sshm cd <alias>" return 2 } local row="$(_sshm_find "$alias")" [[ -z "$row" ]] && { _sshm_error "alias not found: $alias" return 1 } local a key host user port workdir tags file IFS="$SEP" read -r a key host user port workdir tags file <<<"$row" if [[ -n "$workdir" && -d "$workdir" ]]; then cd "$workdir" || return $? pwd return 0 fi local dir dir="$(dirname "$key")" [[ -d "$dir" ]] || { _sshm_error "key directory not found: $dir" return 1 } cd "$dir" || return $? pwd return 0 ;; show) local alias="$2" [[ -z "$alias" ]] && { _sshm_error "Usage: sshm show <alias>" return 2 } local row="$(_sshm_find "$alias")" [[ -z "$row" ]] && { _sshm_error "alias not found: $alias" return 1 } local a key host user port workdir tags file IFS="$SEP" read -r a key host user port workdir tags file <<<"$row" echo "Alias : $a" echo "Host : $host" echo "User : ${user:-<none>}" echo "Port : ${port:-<none>}" echo "Key : $key" echo "Workdir : ${workdir:-<none>}" echo "Tags : ${tags:-<none>}" echo "File : $file" return 0 ;; test) local alias="$2" [[ -z "$alias" ]] && { _sshm_error "Usage: sshm test <alias>" return 2 } local row="$(_sshm_find "$alias")" [[ -z "$row" ]] && { _sshm_error "alias not found: $alias" return 1 } local a key host user port workdir tags file IFS="$SEP" read -r a key host user port workdir tags file <<<"$row" _sshm_test_connection "$key" "$host" "$user" "$port" "$workdir" return $? ;; ssh) local alias="$2" shift 2 [[ -z "$alias" ]] && { _sshm_error "Usage: sshm ssh <alias> ..." return 2 } local row="$(_sshm_find "$alias")" [[ -z "$row" ]] && { _sshm_error "alias not found: $alias" return 1 } local a key host user port workdir tags file IFS="$SEP" read -r a key host user port workdir tags file <<<"$row" _sshm_record_usage "$alias" _sshm_managed_ssh "$key" "$host" "$user" "$port" "$workdir" "$@" return $? ;; scp) local alias="$2" local src="$3" local dst="$4" shift 4 [[ -z "$alias" || -z "$src" || -z "$dst" ]] && { _sshm_error "Usage: sshm scp <alias> <src> <dst> [-- raw scp args...]" return 2 } local row="$(_sshm_find "$alias")" [[ -z "$row" ]] && { _sshm_error "alias not found: $alias" return 1 } local a key host user port workdir tags file IFS="$SEP" read -r a key host user port workdir tags file <<<"$row" _sshm_managed_scp "$key" "$host" "$user" "$port" "$workdir" "$src" "$dst" "$@" return $? ;; export) local output_file="${2:-sshm-inventory-$(date +%Y%m%d-%H%M%S).tar.gz}" [[ -d "$INV_DIR" ]] || { _sshm_error "inventory directory not found: $INV_DIR" return 1 } if _sshm_has tar; then tar -czf "$output_file" -C "$(dirname "$INV_DIR")" "$(basename "$INV_DIR")"/*.inv 2>/dev/null && { echo "Exported inventory to: $output_file" return 0 } || { _sshm_error "export failed" return 1 } else _sshm_error "tar not found (needed for export)" return 1 fi ;; import) local input_file="$2" [[ -z "$input_file" ]] && { _sshm_error "Usage: sshm import <file>" return 2 } [[ -f "$input_file" ]] || { _sshm_error "file not found: $input_file" return 1 } if _sshm_has tar; then tar -xzf "$input_file" -C "$(dirname "$INV_DIR")" 2>/dev/null && { echo "Imported inventory from: $input_file" return 0 } || { _sshm_error "import failed (check file format)" return 1 } else _sshm_error "tar not found (needed for import)" return 1 fi ;; history) local count="${2:-10}" [[ "$count" =~ ^[0-9]+$ ]] || count=10 if [[ -f "$HISTORY_FILE" ]]; then tail -n "$count" "$HISTORY_FILE" | awk '{ ts=$1; alias=$2; cmd="date -r " ts " 2>/dev/null || date -d @" ts " 2>/dev/null" cmd | getline dt close(cmd) print dt " " alias }' | tac else echo "No history found" fi return 0 ;; ui|"") _sshm_has fzf || { _sshm_error "fzf not found. Use: sshm ls / sshm ssh / sshm add" return 1 } local filter="${2:-}" local rows="$(_sshm_collect)" [[ -z "$rows" ]] && { _sshm_error "no inventory entries found in $INV_DIR" return 1 } # Get recent aliases for sorting and indicators local recent_aliases recent_aliases="$(_sshm_get_recent_aliases)" # Build: alias<SEP>prettyline<SEP>sort_key local out keypress sel alias row out="$(printf "%s\n" "$rows" | awk -v FS='\037' -v OFS='\037' -v filter="$filter" -v recent="$recent_aliases" ' BEGIN { reset="\033[0m" dim="\033[2m" bright="\033[1m" tfc="\033[35m" # magenta ipc="\033[36m" # cyan dnc="\033[32m" # green star="\033[33m⭐\033[0m" # yellow star # Build recent set split(recent, recent_arr, "\n") for (i in recent_arr) recent_set[recent_arr[i]] = 1 } { alias=$1; key=$2; host=$3; user=$4; port=$5; workdir=$6; tags=$7; file=$8; if (filter != "" && tolower(alias) !~ tolower(filter) && tolower(host) !~ tolower(filter) && tolower(tags) !~ tolower(filter)) next # Check if recently used is_recent = (alias in recent_set) ? 1 : 0 # Type + icon if (host ~ /^tf:/) { icon=tfc "⛏" reset; typ="TF"; } else if (host ~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/) { icon=ipc "●" reset; typ="IP"; } else { icon=dnc "◌" reset; typ="DNS"; } # Add star indicator for recently used if (is_recent) { icon = star " " icon } # key base n=split(key, a, "/"); kb=a[n]; # workdir base wdb="-"; if (workdir != "") { m=split(workdir, b, "/"); wdb=b[m]; } # filebase (category) fb=file; sub(/.*\//,"",fb); sub(/\.inv$/,"",fb); # user/port printable u=(user==""?"-":user); p=(port==""?"-":port); # tags t=(tags==""?"-":tags); # shorten host display (don'\''t resolve tf here; keep fast) h=host; if (length(h) > 44) h=substr(h,1,41) "…"; # One aligned pretty string (single column) # icon alias host user port key workdir tags [category] pretty=sprintf("%s %-22s %-46s u:%-10s p:%-5s %-16s %-14s %-12s %s[%s]%s", icon, alias, h, u, p, kb, wdb, t, dim, fb, reset); # Sort key: recent first (0), then alphabetical sort_key = sprintf("%d%030s", (1 - is_recent), alias) print alias, pretty, sort_key; } ' | sort -t"$SEP" -k3 | cut -d"$SEP" -f1,2 | fzf \ --ansi \ --delimiter="$SEP" \ --with-nth=2 \ --height="$FZF_HEIGHT" --layout=reverse --border \ --prompt="$FZF_PROMPT" \ --header="$FZF_HEADER" \ $([ "$FZF_PREVIEW" = "true" ] && echo "--preview-window=$FZF_PREVIEW_WINDOW --preview='echo \"Use Ctrl-s to show full details\"'") \ --bind '?:toggle-preview' \ --expect=enter,ctrl-d,ctrl-h,ctrl-k,ctrl-u,ctrl-p,ctrl-s,ctrl-e,ctrl-t,ctrl-w)" keypress="$(sed -n '1p' <<<"$out")" sel="$(sed -n '2p' <<<"$out")" [[ -z "$sel" ]] && return 0 IFS="$SEP" read -r alias _pretty <<<"$sel" row="$(_sshm_find "$alias")" [[ -z "$row" ]] && { _sshm_error "alias not found: $alias" return 1 } local a key host user port workdir tags file IFS="$SEP" read -r a key host user port workdir tags file <<<"$row" # Resolve host for display/copy operations local display_host="$host" if [[ "$host" == tf:* ]]; then display_host="$(_sshm_resolve_inventory_host "$host" "$workdir" 2>/dev/null || echo "$host")" fi case "$keypress" in ctrl-d) # Prefer workdir (terraform dir) if present if [[ -n "$workdir" && -d "$workdir" ]]; then cd "$workdir" || return $? pwd return 0 fi local dir dir="$(dirname "$key")" [[ -d "$dir" ]] || { _sshm_error "key directory not found: $dir" return 1 } cd "$dir" || return $? pwd return 0 ;; ctrl-h) # Copy resolved IP for tf: entries; otherwise copy host as stored if _sshm_clip_copy "$display_host"; then echo "Copied: $display_host" else echo "Host/IP: $display_host (no clipboard tool found)" fi return 0 ;; ctrl-k) if _sshm_clip_copy "$key"; then echo "Copied key: $key" else echo "Key: $key (no clipboard tool found)" fi return 0 ;; ctrl-u) # Copy user@host local user_host="$display_host" [[ -n "$user" ]] && user_host="$user@$display_host" if _sshm_clip_copy "$user_host"; then echo "Copied: $user_host" else echo "User@Host: $user_host (no clipboard tool found)" fi return 0 ;; ctrl-p) # Copy full SSH command local ssh_cmd="ssh -i \"$key\"" [[ -n "$port" ]] && ssh_cmd+=" -p $port" local dest="$display_host" [[ -n "$user" ]] && dest="$user@$display_host" ssh_cmd+=" $dest" if _sshm_clip_copy "$ssh_cmd"; then echo "Copied command: $ssh_cmd" else echo "Command: $ssh_cmd (no clipboard tool found)" fi return 0 ;; ctrl-s) # Show full details (like 'show' command) echo "Alias : $a" echo "Host : $host" [[ "$host" == tf:* ]] && echo "Resolved : $display_host" echo "User : ${user:-<none>}" echo "Port : ${port:-<none>}" echo "Key : $key" echo "Workdir : ${workdir:-<none>}" echo "Tags : ${tags:-<none>}" echo "File : $file" local last_used="$(_sshm_get_last_used "$alias")" [[ -n "$last_used" ]] && echo "Last used: $last_used" echo "" echo "SSH Command:" local ssh_cmd="ssh -i \"$key\"" [[ -n "$port" ]] && ssh_cmd+=" -p $port" local dest="$display_host" [[ -n "$user" ]] && dest="$user@$display_host" ssh_cmd+=" $dest" echo " $ssh_cmd" return 0 ;; ctrl-w) # Copy workdir path if [[ -n "$workdir" ]]; then if _sshm_clip_copy "$workdir"; then echo "Copied workdir: $workdir" else echo "Workdir: $workdir (no clipboard tool found)" fi else echo "No workdir set for this alias" fi return 0 ;; ctrl-e) "${EDITOR:-vi}" "$file" return $? ;; ctrl-t) _sshm_test_connection "$key" "$host" "$user" "$port" "$workdir" return $? ;; enter|"") _sshm_record_usage "$alias" _sshm_managed_ssh "$key" "$host" "$user" "$port" "$workdir" "$@" return $? ;; esac return 0 ;; *) _sshm_error "unknown command: $sub" _sshm_help return 2 ;; esac } alias sm=sshm # ============================ end sshm =======================================


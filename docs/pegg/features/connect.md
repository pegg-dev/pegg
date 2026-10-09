---
icon: lucide/monitor-smartphone
---

# Connect

Connect pairs the Pegg app running on **your machine** with the Pegg website, so
you can chat with your coding agent from a browser, browse its sessions, and manage
its settings — while every prompt, file read, and shell command still happens locally.

The website never executes anything on your machine. It relays messages over an
encrypted WebSocket; your agent answers using the providers, tools, and permissions
you already configured. **Nothing is stored on the website**: sessions and
transcripts live in `~/.pegg/sessions.db` on your machine and are streamed to the
browser on demand — when the agent is offline, its history simply is not available
rather than being served from someone else's server.

## Quick start

1. **Generate a pairing code on the website.** Open **Connect → Connect a device**
   (or **Agents → Connect a device**), type a label for the machine, and press
   **Generate code**. The code is single-use and expires in about 30 minutes.
2. **Paste it on the machine that runs the agent:**

    ```bash
    pegg connect B3GBpJznsxn2KASWM3mgDtvkDTAGTp0M
    ```

   The token is saved the first time, so afterwards you only need:

    ```bash
    pegg connect
    ```

3. **Watch it come online.** The terminal prints your device ID and starts dialing.
   In the website the agent appears under **Agents** — normally immediately as
   *Online* (pairing binds it to your team), or as *Pending approval* when the
   website asks for confirmation first.

That's it. The next time you open the website, your agent is already connected.

## What you can do from the website

| Page | What it gives you |
|---|---|
| **Connect** | Chat with the agent — sessions, streaming answers, reasoning blocks, tool cards, diffs, and a composer that can stop a run |
| **Agents** | Every machine connected to your team: online/offline state, workspace, rename, granted scopes, revoke, re-enable, delete |
| **Agent settings** | Change the provider, model, reasoning effort, permissions, compaction, memory, MCP servers, skills, rules, plugins — the same settings as the TUI, saved on the machine |
| **Plan & Billing** | How many agents your plan allows, what is connected right now, and one-click **Add agents** |

Model and reasoning effort are chosen in **Agent settings → General** and applied to
every message you send from the website; everything else follows the agent's own
configuration.

## Managing agents

Each machine keeps its own identity, so reconnecting never asks for a new pairing
round as long as the agent stays registered.

| State | Meaning | What you can do |
|---|---|---|
| **Online** | Connected right now | Chat with it, rename it, change its scopes |
| **Offline** | Registered, but not connected | Its sessions are unavailable until it returns; it reconnects automatically |
| **Pending** | Connected but not bound to a team yet | Approve it (requires the `connect.devices.approve` permission) |
| **Revoked** | Disabled; it can no longer connect | **Enable** it again (same identity, no re-pairing) or **Delete** it from the registry |

- **Revoke** disconnects the agent and keeps its record — useful for a laptop you
  won't use for a while.
- **Enable** puts it back with the same identity, so it reconnects on its next
  attempt.
- **Delete** removes the record permanently and frees an agent slot; the machine
  will come back as *Pending* if it tries to connect again without a new code.

## Agent limits

Your plan decides how many coding agents a team may keep connected:

| Plan | Agents |
|---|---|
| Free | 1 |
| Hobby | 3 |
| Pro | 20 |
| Enterprise | Unlimited |

Ran out? The **Add agents** dialog (on Connect, Agents, or Plan & Billing) sells
extra agents as a monthly add-on, and the plan page lists everything you own. When
the allowance is full, approving or enabling an agent says *agent limit reached*
and offers to add capacity instead.

## Commands

Everything about Connect is a subcommand of `pegg connect`:

```bash
pegg connect <token>     # first run: connect and remember the token
pegg connect             # every run after that
pegg connect status      # relay, saved token (masked), device identity
pegg connect login       # save a token without connecting (asks if omitted)
pegg connect logout      # forget the saved token (--all also resets the device)
pegg connect whoami      # this machine's device ID and public key
pegg connect methods     # everything the website may call on this machine
```

The relay endpoint is built into the app, so you never paste a URL — just a token.
If you self-host the website, point `connect.relay_url` in `~/.peggco/pegg.json`
at your deployment.

## How safe is this

- **Least privilege per agent.** Each pairing grants a set of scopes (`read`, `chat`,
  `sessions:manage`, `settings:read`, and so on) that you can trim from the Agents
  page. `pegg connect methods` lists exactly what the website can call.
- **You approve sensitive actions.** Unless you turn on `auto-approve`, shell
  commands and other sensitive tools raise the same confirmation prompts they do in
  the terminal — including the permission judge and your per-tool rules.
- **The device has an identity, not a password.** A local Ed25519 key is generated on
  first run and stored at `~/.pegg/connect/device.json`; the website only ever
  receives its public key, and any mismatch is rejected.
- **Pairing codes are single-use** and short-lived, and the token is never printed
  again — `pegg connect status` shows it masked.
- **No transcripts leave your machine.** The website receives messages over the relay
  and keeps none of them.

Avoid `--auto-approve` unless the machine is unattended: it removes the local
confirmation step for anything the website asks for.

## Troubleshooting

| Symptom | What it means | What to do |
|---|---|---|
| `no pairing token — run pegg connect <token>` | No token saved yet | Generate a code on the website and run `pegg connect <token>`, or save one with `pegg connect login` |
| The agent stays **Pending** | It connected but was not approved yet | Approve it on the Agents page (needs `connect.devices.approve`) |
| `device revoked` in the terminal | The agent was disabled from the website | **Enable** it on the Agents page, or revoke the record and pair again |
| `agent limit reached (…)` | Your plan is full | **Add agents** on the plan page, or delete an agent you no longer use |
| The agent shows **Offline** | The daemon is not running or lost its network | Start `pegg connect` again — it reconnects and backfills the session list |
| `permission denied` on a website action | The pairing granted fewer scopes than the page needs | Edit the agent's scopes, or pair again with a broader approval |
| Token expired | Codes last about 30 minutes and are single-use | Generate a new code on the website |

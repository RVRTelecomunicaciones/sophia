# Soak window automation — `scripts/soak-daily.sh` + launchd

Two-system setup running the v0.2.0-rc.1 → v0.2.0 daily smoke
during the 7-day soak window (2026-05-08 → 2026-05-15).

## Layers

| Layer | Tool | Trigger | What it does |
|---|---|---|---|
| **A. Interactive** | Claude `CronCreate` | 09:13 local daily, while Claude is open | Runs gates, updates `docs/release/v0.2.0-soak-matrix.md`, commits + pushes both repos. Triages 🔴 in-session. **Session-only** — dies when Claude exits. |
| **B. Durable** | macOS `launchd` | 09:13 local daily, regardless of Claude | Runs gates only (read-only). Writes `~/.local/state/sophia/soak/YYYY-MM-DD.log` + `latest-summary.txt`. Survives reboots. **Doesn't update the matrix or commit** — that's interactive Claude's job. |

When both fire on the same day, the interactive cron reads the
launchd-written log first and skips re-running the gates if `RESULT:
all 6 gates GREEN` is present. Saves ~90 seconds per day.

## Files

```
scripts/soak-daily.sh                       — the gate-running script
scripts/promote-v0.2.0.sh                   — the Day 7 promotion script
~/Library/LaunchAgents/com.sophia.soak.plist — launchd job (NOT in repo)
~/.local/state/sophia/soak/YYYY-MM-DD.log    — per-day log
~/.local/state/sophia/soak/latest-summary.txt — most recent summary
~/.local/state/sophia/soak/launchd.out|err   — launchd-level stdout/stderr
```

## Install the launchd plist (one-time)

The plist lives outside the repo because it's user- and OS-specific.
Recreate it manually:

```bash
mkdir -p ~/Library/LaunchAgents ~/.local/state/sophia/soak

cat > ~/Library/LaunchAgents/com.sophia.soak.plist <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>com.sophia.soak</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>-c</string>
        <string>exec /Users/russell/Documents/2026/sophia-cli/scripts/soak-daily.sh</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict>
        <key>Hour</key><integer>9</integer>
        <key>Minute</key><integer>13</integer>
    </dict>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key><string>/usr/local/go/bin:/opt/homebrew/bin:/usr/bin:/bin</string>
        <key>HOME</key><string>/Users/russell</string>
    </dict>
    <key>StandardOutPath</key><string>/Users/russell/.local/state/sophia/soak/launchd.out</string>
    <key>StandardErrorPath</key><string>/Users/russell/.local/state/sophia/soak/launchd.err</string>
    <key>RunAtLoad</key><false/>
</dict>
</plist>
PLIST

plutil -lint ~/Library/LaunchAgents/com.sophia.soak.plist
launchctl unload ~/Library/LaunchAgents/com.sophia.soak.plist 2>/dev/null
launchctl load   ~/Library/LaunchAgents/com.sophia.soak.plist
launchctl list | grep com.sophia.soak
```

Adjust the path under `ProgramArguments` and `HOME` if your repo
checkout lives elsewhere.

## Daily ops

- **Manual probe** (any time): `bash scripts/soak-daily.sh; cat ~/.local/state/sophia/soak/latest-summary.txt`
- **Force-fire the launchd job** (test): `launchctl kickstart -k gui/$(id -u)/com.sophia.soak`
- **Inspect the schedule:** `launchctl print gui/$(id -u)/com.sophia.soak | head`

## End of soak (Day 7+)

Once `v0.2.0` is tagged and pushed:

```bash
launchctl unload ~/Library/LaunchAgents/com.sophia.soak.plist
rm ~/Library/LaunchAgents/com.sophia.soak.plist
# state logs under ~/.local/state/sophia/soak can stay as audit trail.
```

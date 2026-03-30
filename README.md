# opi-thermald

Tiny Go daemon for Orange Pi and similar SBCs. It tracks CPU temperature, CPU usage, load, memory usage, and disk usage with very low overhead, stores local samples in JSONL, and can send Telegram alerts and daily/weekly summaries.

Repository: `https://github.com/horoshi10v/opi-thermald`
Go module: `github.com/horoshi10v/opi-thermald`

## Why this design

- One binary, no external services
- Standard library only
- No Docker required
- Runs well as a `systemd` service
- Writes compact local history to disk

## Features

- Polls `/sys` and `/proc` every N seconds
- Tracks:
  - CPU temperature
  - CPU usage
  - load average
  - memory usage
  - root filesystem usage
- Sends Telegram alerts with hysteresis and cooldown
- Exports daily and weekly CSV snapshots
- Sends daily and weekly summaries as Telegram photos with a caption
- Keeps up to 8 days of raw samples

## Project layout

```text
opi-thermald/
  cmd/opi-thermald/main.go
  config/config.example.json
  deploy/opi-thermald.service
  internal/
    collector/
    config/
    service/
    storage/
    telegram/
```

## Local build

```bash
go build -o bin/opi-thermald ./cmd/opi-thermald
```

## Deploy from GitHub to Orange Pi

### 1. Install Go on Orange Pi

```bash
sudo apt update
sudo apt install -y golang-go git
```

### 2. Clone the repository

```bash
cd /opt
sudo git clone https://github.com/horoshi10v/opi-thermald.git
cd /opt/opi-thermald
```

If this daemon lives inside a larger monorepo, clone that repo instead and `cd` into the `opi-thermald` folder.

### 3. Build the binary

```bash
go build -o opi-thermald ./cmd/opi-thermald
sudo install -Dm755 ./opi-thermald /usr/local/bin/opi-thermald
```

### 4. Install config

```bash
sudo mkdir -p /etc/opi-thermald /var/lib/opi-thermald
sudo cp ./config/config.example.json /etc/opi-thermald/config.json
sudo nano /etc/opi-thermald/config.json
```

Update:

- `telegram_bot_token`
- `telegram_chat_id`
- `host_alias`
- temperature thresholds

### 5. Install systemd unit

```bash
sudo cp ./deploy/opi-thermald.service /etc/systemd/system/opi-thermald.service
sudo systemctl daemon-reload
sudo systemctl enable --now opi-thermald
```

### 6. Verify service health

```bash
systemctl status opi-thermald
journalctl -u opi-thermald -n 100 --no-pager
```

### 7. Update later from GitHub

```bash
cd /opt/opi-thermald
sudo git pull
go build -o opi-thermald ./cmd/opi-thermald
sudo install -Dm755 ./opi-thermald /usr/local/bin/opi-thermald
sudo systemctl restart opi-thermald
```

## Telegram bot setup

1. Create a bot with BotFather
2. Get the bot token
3. Send at least one message to the bot from your Telegram account
4. Obtain your `chat_id`
5. Put both values into `/etc/opi-thermald/config.json`

## Telegram commands

- `/temp` returns the current CPU temperature
- `/status` returns the current temperature, CPU, load, memory, and disk usage
- `/summary` sends the current daily summary immediately
- `/weekly` sends the current weekly summary immediately

## Runtime files

- Config: `/etc/opi-thermald/config.json`
- Data: `/var/lib/opi-thermald/`
- Samples: `/var/lib/opi-thermald/samples.jsonl`
- State: `/var/lib/opi-thermald/state.json`
- CSV exports: `/var/lib/opi-thermald/exports/daily-latest.csv` and `/var/lib/opi-thermald/exports/weekly-latest.csv`
- Summary charts are generated in memory and sent to Telegram as PNG photos

## Summary format

Daily and weekly summaries are sent as PNG images to Telegram with a text caption that contains the main stats.

```text
orangepi-ultra daily summary
Temp min/avg/max: 46.1/58.3/76.4C
CPU avg/max: 8.22/61.44%
RAM avg/max: 22.40/31.85%
Load1 avg/max: 0.42/2.31
Samples above warn: 12/2880
```

## Resource profile

Expected runtime footprint is small:

- low CPU because it mostly sleeps
- modest RAM because there is no web UI and no database server
- disk writes are tiny and append-only, plus periodic pruning

## Notes

- This daemon is designed for passive-cooled boards and alerting, not for fancy dashboards
- If later you want graphs, you can export `samples.jsonl` into another tool

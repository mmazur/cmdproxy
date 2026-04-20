# Clipboard over SSH with cmdproxy socket mode

Forward your local Wayland clipboard (`wl-paste`) to a remote SSH
system so that programs there can read your desktop clipboard.

## How it works

```
remote server                         local desktop (Wayland)
─────────────                         ──────────────────────
wl-paste
  → cmdproxy-shim
    → connect to forwarded Unix socket
                                      → systemd accepts connection
                                        → cmdproxy-server --socket
                                          → policy check → allowed
                                          → exec wl-paste, capture output
                                          → JSON response with clipboard contents
  ← stdout = clipboard contents
```

No reverse SSH needed — the socket is forwarded over your existing outbound SSH
connection using `RemoteForward`.

## Setup

### 1. Build and install cmdproxy (both systems)

```
make && make install    # installs to ~/.local/bin
```

### 2. Local desktop — systemd socket activation

Copy the systemd units:

```
cp server/cmdproxy.socket  ~/.config/systemd/user/
cp server/cmdproxy@.service ~/.config/systemd/user/
```

Enable the socket:

```
systemctl --user daemon-reload
systemctl --user enable --now cmdproxy.socket
```

### 3. Local desktop — server policy

```
mkdir -p ~/.config/cmdproxy/profiles
cp server/default.toml ~/.config/cmdproxy/profiles/default.toml
```

### 4. Local desktop — SSH config

Add the `RemoteForward` block to `~/.ssh/config` (see `server/ssh_config`).

### 5. Remote server — shim config

```
mkdir -p ~/.config/cmdproxy
cp shim/shim.toml ~/.config/cmdproxy/shim.toml
```

### 6. Remote server — create symlinks

```
ln -s ~/.local/bin/cmdproxy-shim ~/.local/bin/wl-paste
```

Make sure `~/.local/bin` is in your `$PATH`.

### 7. Test

SSH into the remote server (the `RemoteForward` sets up the socket automatically),
then run:

```
wl-paste
```

You should see your desktop clipboard contents.

## File overview

```
shim/
  shim.toml              → remote: ~/.config/cmdproxy/shim.toml

server/
  default.toml           → local:  ~/.config/cmdproxy/profiles/default.toml
  cmdproxy.socket         → local:  ~/.config/systemd/user/cmdproxy.socket
  cmdproxy@.service       → local:  ~/.config/systemd/user/cmdproxy@.service
  ssh_config             → local:  add to ~/.ssh/config
```

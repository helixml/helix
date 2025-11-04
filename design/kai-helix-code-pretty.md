## ags

The library to make a nice wrapper around Wayland

https://github.com/Aylur/ags
https://aylur.github.io/ags/

## sway

The wayland compositor.

https://swaywm.org/

## booting

we need to run `sudo modprobe vgem` before running `./stack start` inside Helix.

## dev machine

```
ssh kai@dev.code.helix.ml
```

From local:

```
ssh -L 8080:localhost:8080 -L 8081:localhost:8081 kai@dev.code.helix.ml
```

## build stack

install rust:

```
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

clone repositories:

```
git clone git@github.com:helixml/helix.git
(cd helix && git checkout feature/helix-code)
git clone git@github.com:helixml/moonlight-web-stream.git
(cd moonlight-web-stream && git checkout feature/kickoff)
git clone git@github.com:helixml/wolf.git
(cd wolf && git checkout wolf-ui-working)
git clone https://github.com/helixml/zed.git
(cd zed && git checkout feature/external-thread-sync)
```

resolve submodules:

```
(cd moonlight-web-stream && git submodule update --init --recursive)
```

we need to put the `.env` file in the root of the helix directory.

```
sudo apt-get install libx11-dev pkg-config cmake libasound2-dev libx11-xcb-dev libxkbcommon-dev libxkbcommon-x11-dev
```

```
cd helix
./stack start
```

## files

`Dockerfile.sway-helix` = the container that is spawned by wolf - i.e. what we put in here is what the desktop can use.

`wolf/sway-config` = the config for sway - this ends up in `/cfg/sway/custom-cfg` inside the container.

`wolf/sway-config/startup-app.sh` = desktop container logical entrypoint - this is where waybar lives and where we need to replace it with something better

`./stack build-sway` = builds the container

`api/pkg/external-agent/wolf_executor.go` = the code that spawns the container.


# PlatformCraft Video Loader

## Build

```shell
go build
```

## Run

```shell
platformcraft_video_loader <URL> <RES> [<ATTEMPTS>] [<ROUTINES>] > output.mp4
```

Where `<URL>` - url of page with video, `<RES>` - resolution,
`<ATTEMPTS>` - attempts count for retry, `<ROUTINES>` - parralel working routines count,
`<output.mp4>` - filename for save result file

By default `ATTEMPTS` is 10, `ROUTINES` is 1

Example:

```shell
platformcraft_video_loader \
    http://video.platformcraft.ru/embed/60506db30e47cf1a472041b4 \
    1280x720 > output.mp4
```

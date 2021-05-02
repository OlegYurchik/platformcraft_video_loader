# PlatformCraft Video Loader

## Build

```shell
go build
```

## Usage

```
usage: platformcraft_video_loader [-h|--help] -u|--url "<value>"
                                  -r|--resolution "<value>" [-g|--goroutines
                                  <integer>] [-a|--attempts <integer>]

                                  Video loader from PlatformCraft

Arguments:

  -h  --help        Print help information
  -u  --url         Video URL for downloading
  -r  --resolution  Set video resolution
  -g  --goroutines  Max count of parallel working goroutines. Default: 1
  -a  --attempts    Count of retry attempts if downloaded is failed. Default: 3
```

Example:

```shell
platformcraft_video_loader \
    -u http://video.platformcraft.ru/embed/60506db30e47cf1a472041b4 \
    -r 1280x720 > output.mp4
```

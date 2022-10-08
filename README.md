# Sound Brick

## About

By default, the hotkey is `\`. This can be changed in the settings.

## Features

-   [x] Hotkey toggle
-   [x] Renamable outputs
-   [x] Disable inputs
-   [ ] Optional alert sound on switch

## Build

```sh
go build -ldflags -H=windowsgui
```

## Generate Icon Bytes

```sh
C:/Users/kyles/go/bin/go2array -prefix Data -package icon iconwin.ico icon.png

C:/Users/kyles/go/bin/go2array -prefix Data -package check checkwin.ico check.png

C:/Users/kyles/go/bin/go2array -prefix Data -package blank blankwin.ico blank.png
```

## Generate Icon rsrc

```sh
%GOPATH%\bin\rsrc.exe -ico assets\icon\iconwin.ico
```

## Credits

<a href="https://www.flaticon.com/free-icons/input" title="input icons">Input
icons created by Pixelmeetup - Flaticon</a>

<a href="https://www.flaticon.com/free-icons/check" title="check icons">Check
icons created by Maxim Basinski Premium - Flaticon</a>

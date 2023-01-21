# Sound Brick - Desktop

## About

> The desktop program for Sound Brick

## Features

- [x] Hotkey toggle (Default: `\`)
- [x] Renamable outputs
- [x] Disable inputs
- [x] Settings Menu
- [ ] Optional alert sound on switch
- [ ] MacOS & Linux binaries

## Build

```sh
go build -ldflags -H=windowsgui
```

## Generate Icon Bytes

```sh
go install github.com/parvit/go2array

%GOPATH%/bin/go2array -prefix Data -package icon iconwin.ico icon.png
%GOPATH%/bin/go2array -prefix Data -package check checkwin.ico check.png
%GOPATH%/bin/go2array -prefix Data -package blank blankwin.ico blank.png
```

## Generate Icon rsrc

```sh
go install github.com/akavel/rsrc

%GOPATH%\bin\rsrc.exe -ico assets\icon\iconwin.ico
```

## Credits

<a href="https://www.flaticon.com/free-icons/input" title="input icons">Input
icons created by Pixelmeetup - Flaticon</a>

<a href="https://www.flaticon.com/free-icons/check" title="check icons">Check
icons created by Maxim Basinski Premium - Flaticon</a>

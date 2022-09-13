package utils

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"

	"kyleschwartz/soundbrick/assets/icon"

	"github.com/electricbubble/go-toast"
)

func OpenLink(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func Alert(title string, content string) {
	go toast.Push(content,
		toast.WithTitle(title),
		toast.WithAppID("Sound Brick"),
		toast.WithAudio(toast.Default),
		toast.WithShortDuration(),
		toast.WithIconRaw(icon.Data),
	)
}

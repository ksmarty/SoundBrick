package utils

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"kyleschwartz/soundbrick/assets/icon"

	"github.com/electricbubble/go-toast"
	"gopkg.in/ini.v1"
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
	Prod(func() {
		go toast.Push(content,
			toast.WithTitle(title),
			toast.WithAppID("Sound Brick"),
			toast.WithAudio(toast.Default),
			toast.WithShortDuration(),
			toast.WithIconRaw(icon.Data),
		)
	})
}

func Load() *ini.File {
	dir, _ := os.UserConfigDir()
	configPath := filepath.Clean(fmt.Sprintf("%s/SoundBrick", dir))
	configFile := filepath.Clean(fmt.Sprintf("%s/config.ini", configPath))

	// Check if config exists, if not create it
	if _, err := os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
		os.MkdirAll(configPath, 0644)
		os.Create(configFile)
	}

	file, _ := ini.InsensitiveLoad(configFile)

	return file
}

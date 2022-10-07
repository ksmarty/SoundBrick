package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"
	"golang.design/x/hotkey"
	"golang.design/x/hotkey/mainthread"
	"golang.org/x/exp/slices"
	"gopkg.in/ini.v1"

	"kyleschwartz/soundbrick/assets/blank"
	"kyleschwartz/soundbrick/assets/check"
	"kyleschwartz/soundbrick/assets/icon"
	"kyleschwartz/soundbrick/utils"

	"github.com/gen2brain/iup-go/iup"
)

type Switcher struct {
	UDP        *net.UDPConn
	prevOutput int
	config     *ini.File
	updated    map[string]chan string
}

func (switcher *Switcher) cycleOutput() {
	conf := switcher.config.Section("").Key
	enabled := conf("enabled").Strings(",")

	// Do nothing if all inputs are disabled
	if !slices.Contains(enabled, "ON") {
		return
	}

	x, _ := conf("current_output").Int()

	if x == 4 {
		x = switcher.prevOutput
	}

	// Find next available input
	for do := true; do; do = (enabled[x] != "ON") {
		x = (x + 1) % 4
	}

	switcher.sendUDP(x)
}

func (switcher *Switcher) muteToggle() {
	if switcher.UDP == nil {
		return
	}

	cur, _ := switcher.config.Section("").Key("current_output").Int()

	if cur != 4 {
		switcher.prevOutput = cur
	}

	switcher.sendUDP(4)
}

func (switcher *Switcher) noConn() {
	utils.Alert("Error!", "Could not connect to device! Please change IP in settings.")
	switcher.UDP = nil
}

func (switcher *Switcher) connect() {
	sec := switcher.config.Section("")

	userIP := sec.Key("ip").String()

	s, _ := net.ResolveUDPAddr("udp4", userIP)
	c, err := net.DialUDP("udp4", nil, s)

	if err != nil || userIP != c.RemoteAddr().String() {
		switcher.noConn()
		return
	}

	switcher.UDP = c

	// Test connection
	x, err := sec.Key("current_output").Int()
	// If current is mute, request current output
	if err != nil || x == 4 {
		x = -2
	}

	switcher.sendUDP(x)

	if userIP != c.RemoteAddr().String() {
		switcher.noConn()
		return
	}

	if switcher.UDP != nil {
		fmt.Printf("The UDP server is %s\n", c.RemoteAddr().String())

		utils.Alert("Connected!", "Successfully connected to device!")
	}

}

func (switcher *Switcher) setupHotkeys() {
	mainthread.Init(func() {
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()

			k, _ := switcher.config.Section("").Key("hotkey").Int()
			hk := hotkey.New([]hotkey.Modifier{}, hotkey.Key(k))

			defer hk.Unregister()
			defer fmt.Printf("Hotkey %v is unregistered\n", hk)

			err := hk.Register()
			if err != nil {
				fmt.Printf("Error: %s", err.Error())
				return
			}

			for {
				select {
				case <-hk.Keydown():
					switcher.cycleOutput()
				case <-switcher.updated["new_hotkey"]:
					hk.Unregister()
					fmt.Printf("Hotkey %v is unregistered\n", hk)
					defer switcher.setupHotkeys()
					return
				}
			}
		}()
		wg.Wait()
	})
}

func (switcher *Switcher) sendUDP(command int) {
	if switcher.UDP == nil {
		return
	}

	fmt.Printf("Sending packet: %d\n", command)

	_, err := switcher.UDP.Write([]byte(strconv.Itoa(command)))

	if err != nil {
		fmt.Println(err)
	}

	timeout, _ := time.ParseDuration("1s")
	switcher.UDP.SetReadDeadline(time.Now().Add(timeout))

	buffer := make([]byte, 512)
	n, _, err := switcher.UDP.ReadFromUDP(buffer)

	if err != nil {
		println(err.Error())
		switcher.noConn()
		return
	}

	result, _ := strconv.Atoi(string(buffer[0:n]))

	println(result)

	if result == -1 {
		utils.Alert("Oops!", "The system is currently muted. Please unmute to change outputs.")
		return
	}

	switcher.updated["current_output"] <- strconv.Itoa(result)
}

func (switcher *Switcher) save() error {
	dir, _ := os.UserConfigDir()
	if utils.IsDev() {
		return switcher.config.SaveTo("./config.ini")
	} else {
		return switcher.config.SaveTo(fmt.Sprintf("%s/soundbrick/config.ini", dir))
	}
}

func importConfig(switcher *Switcher) {
	go func() {
		update := func(key string, value string) {
			switcher.config.Section("").Key(key).SetValue(value)
			switcher.updated["refresh_tray"] <- key
		}

		notif := func(command string) {
			value, _ := strconv.Atoi(command)

			if value > -1 && value < 4 {
				utils.Alert(
					"Output Changed!",
					fmt.Sprintf("Current output: %s", switcher.config.Section("").Key(fmt.Sprintf("output%d", value+1)).String()),
				)
			} else if value == 4 {
				utils.Alert("Muted!", "Output has been muted.")
			} else if value == -2 {
				// Request current
			} else {
				utils.Alert("Error!", "That's not a valid command! How'd you do that??")
			}
		}

		for {
			select {
			case v := <-switcher.updated["output1"]:
				update("output1", v)
			case v := <-switcher.updated["output2"]:
				update("output2", v)
			case v := <-switcher.updated["output3"]:
				update("output3", v)
			case v := <-switcher.updated["output4"]:
				update("output4", v)
			case v := <-switcher.updated["enabled"]:
				update("enabled", v)

			case v := <-switcher.updated["ip"]:
				update("ip", v)

			case v := <-switcher.updated["hotkey"]:
				update("hotkey", v)

			case v := <-switcher.updated["current_output"]:
				update("current_output", v)
				notif(v)
			}
		}
	}()
}

func (switcher *Switcher) setupConfig() {
	switcher.updated = make(map[string]chan string)

	switcher.updated["output1"] = make(chan string)
	switcher.updated["output2"] = make(chan string)
	switcher.updated["output3"] = make(chan string)
	switcher.updated["output4"] = make(chan string)
	switcher.updated["enabled"] = make(chan string)

	switcher.updated["ip"] = make(chan string)

	switcher.updated["hotkey"] = make(chan string)

	switcher.updated["new_hotkey"] = make(chan string)

	switcher.updated["current_output"] = make(chan string)

	switcher.updated["refresh_tray"] = make(chan string)

	switcher.updated["mute"] = make(chan string)

	cfg := utils.Load()

	sec, _ := cfg.GetSection("")

	// Keys and their default values
	keys := map[string]string{
		"output1":        "Output 1",
		"output2":        "Output 2",
		"output3":        "Output 3",
		"output4":        "Output 4",
		"enabled":        "ON, ON, ON, ON",
		"current_output": "0",
		"ip":             "",
		"hotkey":         "220",
	}

	for k, v := range keys {
		if !sec.HasKey(k) {
			sec.NewKey(k, v)
		}
	}

	switcher.config = cfg

	importConfig(switcher)
}

func (switcher *Switcher) openSettings() {
	iup.Open()
	defer iup.Close()

	const (
		LABEL = iota
		CONNECTION
		CONTROL
	)

	conf := switcher.config.Section("").Key

	darkTheme := iup.User().SetAttributes(`BGCOLOR="#282a36", FGCOLOR="#f8f8f2"`)
	iup.SetHandle("darkTheme", darkTheme)
	iup.SetGlobal("DEFAULTTHEME", "darkTheme")

	inputAction := func(ih iup.Ihandle) int {
		Type, _ := strconv.Atoi(ih.GetAttribute("TYPE"))
		value := ih.GetAttribute("VALUE")
		switcher.updated[ih.GetAttribute("TITLE")] <- strings.TrimSpace(value)

		if Type == CONNECTION && value != conf("ip").String() {
			go switcher.connect()
		} else if Type == CONTROL && value != conf("hotkey").String() {
			switcher.updated["new_hotkey"] <- ""
		}

		return iup.DEFAULT
	}

	enabledAction := func(ih iup.Ihandle, state int) int {
		// Update state
		index, _ := strconv.Atoi(ih.GetAttribute("INDEX"))
		arr := conf("enabled").Strings(",")
		arr[index] = []string{"OFF", "ON"}[state]
		switcher.updated["enabled"] <- strings.Join(arr, ", ")

		// Change colour
		label := iup.GetHandle(fmt.Sprintf("enabled%d", index))
		label.SetAttribute("FGCOLOR", []string{"#ffb86c", "#50fa7b"}[state])
		label.SetAttribute("TITLE", []string{"Disabled", "Enabled"}[state])

		return iup.DEFAULT
	}

	inputGen := func(label string, Type int, confKey string) iup.Ihandle {
		input := iup.Text()
		input.SetAttributes(`CANFOCUS=NO, EXPAND="HORIZONTAL", PADDING=3, FGCOLOR="#D8D8D8"`)
		input.SetAttribute("TITLE", confKey)
		input.SetAttribute("TYPE", Type)
		input.SetAttribute("VALUE", conf(confKey).String())
		input.SetCallback("KILLFOCUS_CB", iup.KillFocusFunc(inputAction))

		var custom iup.Ihandle

		switch Type {
		case LABEL:
			index, _ := strconv.Atoi(label[len(label)-1:])
			index--
			isEnabled := conf("enabled").Strings(",")[index]

			toggle := iup.Toggle("").SetAttribute("VALUE", isEnabled)
			toggle.SetAttribute("INDEX", index)
			toggle.SetCallback("ACTION", iup.ToggleActionFunc(enabledAction))

			state := 0
			if isEnabled == "ON" {
				state = 1
			}
			label := iup.Label([]string{"Disabled", "Enabled"}[state])
			label.SetAttribute("FGCOLOR", []string{"#ffb86c", "#50fa7b"}[state])
			label.SetAttribute("SIZE", "30")
			label.SetHandle(fmt.Sprintf("enabled%d", index))

			custom = iup.Hbox(
				toggle,
				label,
			)
		case CONNECTION:
		case CONTROL:
			custom = iup.FlatButton("Find keycodes")
			custom.SetAttributes(`PADDING=5, BGCOLOR="#50fa7b", FGCOLOR="#000000", HLCOLOR="#48d06d", PSCOLOR, BORDERWIDTH=0, FOCUSFEEDBACK="NO", EXPAND="VERTICAL"`)
			custom.SetCallback("FLAT_ACTION", iup.FlatActionFunc(func(ih iup.Ihandle) int {
				utils.OpenLink("https://www.toptal.com/developers/keycode")
				return iup.DEFAULT
			}))
		}

		container := iup.Hbox(
			iup.Label(label).SetAttributes(`FGCOLOR="#bd93f9"`),
			input,
			custom,
		).SetAttributes("SIZE=200, ALIGNMENT=ACENTER")

		return container
	}

	frameGen := func(title string, components ...iup.Ihandle) iup.Ihandle {
		return iup.Frame(
			iup.Vbox(
				append(components, iup.Space())...,
			).SetAttributes("GAP=10, NMARGIN=10x5"),
		).SetAttribute("TITLE", title)
	}

	labelsFrame := frameGen("Labels",
		inputGen("Output 1", LABEL, "output1"),
		inputGen("Output 2", LABEL, "output2"),
		inputGen("Output 3", LABEL, "output3"),
		inputGen("Output 4", LABEL, "output4"),
	)

	connectionFrame := frameGen("Connection",
		inputGen("IP Address", CONNECTION, "ip"),
	)

	controlsFrame := frameGen("Controls",
		inputGen("Hotkey", CONTROL, "hotkey"),
	)

	title := iup.Label("Sound Brick").SetAttributes(`FONTSIZE=24, FGCOLOR="#bd93f9"`)

	mainContainer := iup.Vbox(
		title,
		labelsFrame,
		connectionFrame,
		controlsFrame,
	).SetAttributes(`ALIGNMENT=ALEFT, NMARGIN=15x10, NGAP=10`)

	iup.Show(iup.Dialog(mainContainer).SetAttribute("TITLE", title.GetAttribute("TITLE")))
	iup.MainLoop()
}

func (switcher *Switcher) setupTray() {
	go systray.Run(func() {
		systray.SetIcon(icon.Data)

		title := "Sound Brick"
		systray.SetTitle(title)
		systray.SetTooltip(title)

		systray.AddMenuItem(title, title)
		systray.AddSeparator()
		mSelect := systray.AddMenuItem("Select Output", "Select output")
		mMute := systray.AddMenuItem("Mute", "Mute devices")
		mSettings := systray.AddMenuItem("Settings", "Open settings")
		mReload := systray.AddMenuItem("Reload Connection", "Reload connection")
		mQuit := systray.AddMenuItem("Quit", "Quit")

		outs := [4]*systray.MenuItem{}
		for i := range outs {
			str := switcher.config.Section("").Key(fmt.Sprintf("output%d", i+1)).String()
			outs[i] = mSelect.AddSubMenuItem(str, str)
		}

		// Add check icon to selected input
		setChecks := func(item int) {
			for _, v := range outs {
				v.SetIcon(blank.Data)
			}
			if item > -1 && item < 4 {
				outs[item].SetIcon(check.Data)
			}
		}

		key := switcher.config.Section("").Key

		// Set initial icon
		cur := func() int {
			x, err := key("current_output").Int()

			if err != nil {
				fmt.Println(err.Error())
				return 5
			}

			return x
		}

		setChecks(cur())

		for {

			select {
			case <-mMute.ClickedCh:
				switcher.muteToggle()

			case <-outs[0].ClickedCh:
				switcher.sendUDP(0)
				setChecks(0)
			case <-outs[1].ClickedCh:
				switcher.sendUDP(1)
				setChecks(1)
			case <-outs[2].ClickedCh:
				switcher.sendUDP(2)
				setChecks(2)
			case <-outs[3].ClickedCh:
				switcher.sendUDP(3)
				setChecks(3)

			case <-mSettings.ClickedCh:
				go switcher.openSettings()

			case <-mReload.ClickedCh:
				switcher.connect()

			case <-mQuit.ClickedCh:
				systray.Quit()
				return

			case v := <-switcher.updated["refresh_tray"]:
				switch v {
				case "output1":
					outs[0].SetTitle(key(v).String())
				case "output2":
					outs[1].SetTitle(key(v).String())
				case "output3":
					outs[2].SetTitle(key(v).String())
				case "output4":
					outs[3].SetTitle(key(v).String())
				case "current_output":
					if cur() != 4 {
						setChecks(cur())
						mMute.SetTitle("Mute")
					} else {
						mMute.SetTitle("Unmute")
					}
				}
			}
		}
	}, func() {
		switcher.exit()
	})
}

func (switcher *Switcher) exit() {
	fmt.Println("Closing...")

	switcher.save()
	if switcher.UDP != nil {
		switcher.UDP.Close()
	}

	os.Exit(0)
}

func main() {
	utils.SetupFlags()

	client := &Switcher{}

	// client.setupSettings()
	client.setupConfig()

	client.connect()

	client.setupTray()
	client.setupHotkeys()
}

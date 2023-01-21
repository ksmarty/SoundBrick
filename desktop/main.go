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

	"github.com/brotherpowers/ipsubnet"
)

const SEND_PORT = ":4210"
const REC_PORT = ":4211"

type Switcher struct {
	prevOutput int
	config     *ini.File
	updated    map[string]chan string
	IP         *net.UDPAddr
	settings   iup.Ihandle
}

const (
	OUT1         = 0
	OUT2         = 1
	OUT3         = 2
	OUT4         = 3
	MUTED        = 4
	ERROR        = -1
	CLIENT_CHECK = -2
)

func (switcher *Switcher) cycleOutput() {
	conf := switcher.config.Section("").Key
	enabled := conf("enabled").Strings(",")

	// Do nothing if all inputs are disabled
	if !slices.Contains(enabled, "ON") {
		return
	}

	x, _ := conf("current_output").Int()

	if x == MUTED {
		x = switcher.prevOutput
	}

	// Find next available input
	for do := true; do; do = (enabled[x] != "ON") {
		x = (x + 1) % 4
	}

	switcher.sendUDP(x)
}

func (switcher *Switcher) muteToggle() {
	cur, _ := switcher.config.Section("").Key("current_output").Int()

	if cur != MUTED {
		switcher.prevOutput = cur
	}

	switcher.sendUDP(MUTED)
}

func (switcher *Switcher) noConn() {
	utils.Alert("Error!", "Could not connect to device! Please change IP in settings.", 2)
}

func (switcher *Switcher) discover() {
	conn, err := net.Dial("udp4", "1.1.1.1:80")
	if err != nil {
		return
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	size, _ := localAddr.IP.DefaultMask().Size()

	cidr := ipsubnet.SubnetCalculator(localAddr.IP.String(), size).GetBroadcastAddress() + SEND_PORT

	switcher.IP, _ = net.ResolveUDPAddr("udp4", cidr)

	switcher.sendUDP(CLIENT_CHECK)
}

func (switcher *Switcher) connect() {
	Key := switcher.config.Section("").Key

	if Key("ip").String() == "" {
		switcher.discover()
		return
	}

	switcher.IP, _ = net.ResolveUDPAddr("udp4", Key("ip").String()+SEND_PORT)

	x, err := Key("current_output").Int()
	if err != nil || x == MUTED {
		x = CLIENT_CHECK
	}

	if switcher.sendUDP(x) {
		utils.Alert("Connected!", "Successfully connected to device!", 2)
	}
}

func (switcher *Switcher) sendUDP(command int) bool {
	pc, err := net.ListenPacket("udp4", REC_PORT)
	if err != nil {
		utils.Alert("Error!", "Another program on your computer is using port 4211!", 2)
		panic(err)
	}
	pc.SetDeadline(time.Now().Add(time.Second))
	defer pc.Close()

	pc.WriteTo([]byte(strconv.Itoa(command)), switcher.IP)

	buffer := make([]byte, 512)
	n, recAddr, err := pc.ReadFrom(buffer)

	if err != nil {
		fmt.Println(err)
		switcher.noConn()
		return false
	}

	if switcher.IP.String() != recAddr.String() {
		ip, _, _ := net.SplitHostPort(recAddr.String())
		switcher.updated["ip"] <- ip
		switcher.save()
		go switcher.connect()
		return false
	}

	result, _ := strconv.Atoi(string(buffer[0:n]))

	if result == ERROR {
		utils.Alert("Oops!", "The system is currently muted. Please unmute to change outputs.", 1)
		return false
	}

	switcher.updated["current_output"] <- strconv.Itoa(result)

	return true
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

func (switcher *Switcher) save() error {
	if utils.IsDev() {
		return switcher.config.SaveTo("./config.ini")
	}

	dir, _ := os.UserConfigDir()
	return switcher.config.SaveTo(fmt.Sprintf("%s/soundbrick/config.ini", dir))
}

func importConfig(switcher *Switcher) {
	go func() {
		Key := switcher.config.Section("").Key

		update := func(key string, value string) {
			Key(key).SetValue(value)
			switcher.updated["refresh_tray"] <- key
		}

		notif := func(command string) {
			value, _ := strconv.Atoi(command)

			if value >= OUT1 && value <= OUT4 {
				utils.Alert(
					"Output Changed!",
					fmt.Sprintf("Current output: %s", Key(fmt.Sprintf("output%d", value+1)).String()),
					1,
				)
			} else if value == MUTED {
				utils.Alert("Muted!", "Output has been muted.", 1)
			} else if value == CLIENT_CHECK {
				// Client check
			} else {
				utils.Alert("Error!", "That's not a valid command! How'd you do that??", 1)
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
			custom = iup.FlatButton("Auto Connect")
			custom.SetAttributes(`PADDING=5, BGCOLOR="#50fa7b", FGCOLOR="#000000", HLCOLOR="#48d06d", PSCOLOR, BORDERWIDTH=0, FOCUSFEEDBACK="NO", EXPAND="VERTICAL"`)
			custom.SetCallback("FLAT_ACTION", iup.FlatActionFunc(func(ih iup.Ihandle) int {
				switcher.discover()
				switcher.openSettings()
				return iup.DEFAULT
			}))
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

	content := iup.Dialog(mainContainer).SetAttribute("TITLE", title.GetAttribute("TITLE"))
	iup.Show(content)
	iup.Hide(switcher.settings)
	switcher.settings = content
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
			if item >= OUT1 && item <= OUT4 {
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

			case <-outs[OUT1].ClickedCh:
				switcher.sendUDP(OUT1)
				setChecks(OUT1)
			case <-outs[OUT2].ClickedCh:
				switcher.sendUDP(OUT2)
				setChecks(OUT2)
			case <-outs[OUT3].ClickedCh:
				switcher.sendUDP(OUT3)
				setChecks(OUT3)
			case <-outs[OUT4].ClickedCh:
				switcher.sendUDP(OUT4)
				setChecks(OUT4)

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
					outs[OUT1].SetTitle(key(v).String())
				case "output2":
					outs[OUT2].SetTitle(key(v).String())
				case "output3":
					outs[OUT3].SetTitle(key(v).String())
				case "output4":
					outs[OUT4].SetTitle(key(v).String())
				case "current_output":
					if cur() != MUTED {
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

	os.Exit(0)
}

func main() {
	utils.SetupFlags()

	client := &Switcher{}

	client.setupConfig()

	client.connect()

	client.setupTray()
	client.setupHotkeys()
}

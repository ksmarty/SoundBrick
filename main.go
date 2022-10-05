package main

import (
	"fmt"
	"image/color"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"
	"golang.design/x/hotkey"
	"golang.design/x/hotkey/mainthread"
	"gopkg.in/ini.v1"

	"kyleschwartz/soundbrick/assets/blank"
	"kyleschwartz/soundbrick/assets/check"
	"kyleschwartz/soundbrick/assets/icon"
	"kyleschwartz/soundbrick/utils"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Switcher struct {
	UDP        *net.UDPConn
	prevOutput int
	settings   *app.Window
	config     *ini.File
	updated    map[string]chan string
}

func (switcher *Switcher) cycleOutput() {
	x, _ := switcher.config.Section("").Key("current_output").Int()
	if x == 4 {
		x = switcher.prevOutput
	}

	switcher.sendUDP((x + 1) % 4)
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
	return switcher.config.SaveTo(fmt.Sprintf("%s/soundbrick/config.ini", dir))
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

func (switcher *Switcher) setupSettings() {
	go func() {
		switcher.settings = app.NewWindow(
			app.Title("Sound Brick"),
			app.Size(unit.Dp(400), unit.Dp(435)),
		)
	}()
}

func (switcher *Switcher) openSettings() {
	err := func() error {
		th := material.NewTheme(gofont.Collection())

		var ops op.Ops

		outputNames := [4]widget.Editor{}
		ipAddr := widget.Editor{}
		key := widget.Editor{}
		keyButton := widget.Clickable{}

		for e := range switcher.settings.Events() {
			switch e := e.(type) {
			case system.DestroyEvent:
				// Save settings on close
				err := switcher.save()
				if err != nil {
					fmt.Println(err)
				}

				// Attempt UDP connection when IP changed
				if switcher.UDP == nil || switcher.UDP.RemoteAddr().String() != ipAddr.Text() {
					switcher.connect()
				}

				switcher.updated["new_hotkey"] <- ""

				// Create new window when old one is destroyed
				switcher.setupSettings()

				return e.Err
			case system.FrameEvent:

				// Layout Context
				gtx := layout.NewContext(&ops, e)

				Colors := struct {
					bg        color.NRGBA
					focused   color.NRGBA
					unfocused color.NRGBA
					primary   color.NRGBA
					secondary color.NRGBA
					tertiary  color.NRGBA
					hint      color.NRGBA
				}{
					bg:        color.NRGBA{0x16, 0x16, 0x1a, 0xff},
					focused:   color.NRGBA{0x7f, 0x5a, 0xf0, 0xff},
					unfocused: color.NRGBA{0x01, 0x01, 0x01, 0xff},
					primary:   color.NRGBA{0xff, 0xff, 0xfe, 0xff},
					secondary: color.NRGBA{0x94, 0xa1, 0xb2, 0xff},
					tertiary:  color.NRGBA{0x2c, 0xb6, 0x7d, 0xff},
					hint:      color.NRGBA{0xa7, 0xa9, 0xbe, 0x30},
				}

				// Background ---------------------------------------

				paint.Fill(&ops, Colors.bg)

				// Events -------------------------------------------

				if keyButton.Clicked() {
					utils.OpenLink("https://www.toptal.com/developers/keycode")
				}

				// Generators ---------------------------------------

				spacer := func(size int, hw string) layout.FlexChild {
					var x layout.Spacer
					if hw == "H" {
						x.Height = unit.Dp(size)
					} else {
						x.Width = unit.Dp(size)
					}
					return layout.Rigid(x.Layout)
				}

				margins := layout.Inset{
					Top:    unit.Dp(5),
					Bottom: unit.Dp(5),
					Right:  unit.Dp(7),
					Left:   unit.Dp(7),
				}

				marginLayout := func(content layout.Widget) layout.FlexChild {
					return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return margins.Layout(gtx, content)
					})
				}

				inputGen := func(we *widget.Editor, conf *ini.Key, channel chan string, label string, widgets ...layout.FlexChild) layout.FlexChild {
					return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						var borderColor color.NRGBA

						if we.Focused() {
							borderColor = Colors.focused
						} else {
							borderColor = Colors.unfocused
						}

						border := widget.Border{
							Color:        borderColor,
							CornerRadius: unit.Dp(3),
							Width:        unit.Dp(2),
						}

						elems := []layout.FlexChild{
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								title := material.H6(th, label)
								title.Color = Colors.secondary
								return title.Layout(gtx)
							}),

							spacer(5, "W"),

							layout.Flexed(100, func(gtx layout.Context) layout.Dimensions {
								ed := material.Editor(th, we, label)
								ed.Color = Colors.secondary
								ed.HintColor = Colors.hint
								we.SingleLine = true

								content := we.Text()

								// Only update when text has changed
								if content != conf.String() && content != "" {
									channel <- strings.TrimSpace(content)
								}
								// Set initial value
								if content == "" && !we.Focused() {
									we.SetText(conf.String())
								}

								return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return margins.Layout(gtx, ed.Layout)
								})
							}),
						}

						combined := append(elems, widgets...)

						return margins.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{
									Axis:      layout.Horizontal,
									Spacing:   layout.SpaceEnd,
									Alignment: layout.Middle,
									WeightSum: 0,
								}.Layout(gtx,
									combined...,
								)
							},
						)
					})
				}

				titleGen := func(label string, size func(th *material.Theme, txt string) material.LabelStyle) layout.FlexChild {
					return marginLayout(func(gtx layout.Context) layout.Dimensions {
						title := size(th, label)
						title.Color = Colors.primary
						return title.Layout(gtx)
					})
				}

				conf := switcher.config.Section("").Key

				// Layout -------------------------------------------

				layout.Flex{
					Axis:    layout.Vertical,
					Spacing: layout.SpaceEnd,
				}.Layout(gtx,

					titleGen("Sound Brick", material.H4),

					spacer(10, "H"),

					titleGen("Labels", material.H6),
					inputGen(&outputNames[0], conf("output1"), switcher.updated["output1"], "Output 1"),
					inputGen(&outputNames[1], conf("output2"), switcher.updated["output2"], "Output 2"),
					inputGen(&outputNames[2], conf("output3"), switcher.updated["output3"], "Output 3"),
					inputGen(&outputNames[3], conf("output4"), switcher.updated["output4"], "Output 4"),

					spacer(20, "H"),

					titleGen("Connection", material.H6),
					inputGen(&ipAddr, conf("ip"), switcher.updated["ip"], "IP Address"),

					spacer(20, "H"),

					titleGen("Controls", material.H6),
					inputGen(&key, conf("hotkey"), switcher.updated["hotkey"], "Hotkey",
						spacer(5, "W"),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btn := material.Button(th, &keyButton, "Find keycodes")
							btn.Background = Colors.tertiary
							btn.Inset = margins

							return btn.Layout(gtx)
						}),
					),
				)

				e.Frame(gtx.Ops)
			}
		}
		return nil
	}()

	if err != nil {
		fmt.Println(err)
	}
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

	client.setupSettings()
	client.setupConfig()

	client.connect()

	client.setupTray()
	client.setupHotkeys()
}

package main

import (
	"errors"
	"fmt"
	"image/color"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

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

type StableChannelS struct {
	C chan string
	V string
}

type StableChannelI struct {
	C chan int
	V int
}

type Switcher struct {
	UDP          *net.UDPConn
	output       StableChannelI
	prevOutput   int
	settings     *app.Window
	outputTitles [4]StableChannelS
	ipAddr       StableChannelS
	key          StableChannelS
	config       *ini.File
}

func (sc *StableChannelS) send(msg string) {
	sc.V = msg
	sc.C <- sc.V
}
func (sc *StableChannelI) send(msg int) {
	sc.V = msg
	sc.C <- sc.V
}

func (switcher *Switcher) cycleOutput() {
	if switcher.UDP == nil {
		return
	}

	val, _ := switcher.config.Section("").Key("current_output").Int()
	x := val
	x++
	x %= 4

	switcher.sendUDP(x)
}

func (switcher *Switcher) muteToggle() bool {
	var val int
	cur, _ := switcher.config.Section("").Key("current_output").Int()

	if cur != -1 {
		switcher.prevOutput = cur
		println(switcher.prevOutput)
		val = -1
		go switcher.output.send(val)
		utils.Alert("Muted!", "Output has been muted.")
	} else {
		val = switcher.prevOutput
	}

	go switcher.output.send(val)
	go switcher.sendUDP(val)

	return val == -1
}

func (switcher *Switcher) connect() {
	noConn := func() {
		utils.Alert("Error!", "Could not connect to device! Please change IP in settings.")
	}

	userIP := switcher.config.Section("").Key("ip").String()

	s, _ := net.ResolveUDPAddr("udp4", userIP)
	c, err := net.DialUDP("udp4", nil, s)

	// if err != nil {
	// 	noConn()
	// 	return
	// }

	if err != nil || userIP != c.RemoteAddr().String() {
		noConn()
		switcher.UDP = nil
		return
	}

	fmt.Printf("The UDP server is %s\n", c.RemoteAddr().String())

	utils.Alert("Connected!", "Successfully connected to device!")

	switcher.UDP = c
}

func (switcher *Switcher) setupHotkeys() {
	mainthread.Init(func() {
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()

			k, _ := strconv.Atoi(switcher.key.V)
			hk := hotkey.New([]hotkey.Modifier{}, hotkey.Key(k))

			defer hk.Unregister()
			defer fmt.Printf("Hotkey %v is unregistered\n", hk)

			err := hk.Register()
			if err != nil {
				fmt.Printf("Error: %s", err.Error())
				return
			}

			for {
				<-hk.Keydown()
				switcher.cycleOutput()
			}
		}()
		wg.Wait()
	})
}

func (switcher *Switcher) sendUDP(command int) string {
	if switcher.UDP == nil {
		return ""
	}

	fmt.Printf("Sending: %d\n", command)

	_, err := switcher.UDP.Write([]byte(strconv.Itoa(command)))

	if err != nil {
		fmt.Println(err)
		return err.Error()
	}

	buffer := make([]byte, 512)
	n, _, err := switcher.UDP.ReadFromUDP(buffer)

	if err != nil {
		fmt.Println(err)
		return err.Error()
	}

	switcher.output.send(command)
	switcher.config.Section("").Key("current_output").SetValue(strconv.Itoa(command))

	if command > -1 && command < 5 {
		utils.Alert(
			"Output Changed!",
			fmt.Sprintf("Current output: %s", switcher.outputTitles[command].V),
		)
	}

	return string(buffer[0:n])
}

func importConfig(switcher *Switcher) {
	go func() {
		sec := switcher.config.Section("")

		for i := 0; i < 4; i++ {
			switcher.outputTitles[i].V = sec.Key(fmt.Sprintf("output%d", i+1)).String()
		}
		switcher.key.V = sec.Key("hotkey").String()
		switcher.ipAddr.V = sec.Key("ip").String()

		for {
			select {
			case v := <-switcher.outputTitles[0].C:
				sec.Key("output1").SetValue(v)
			case v := <-switcher.outputTitles[1].C:
				sec.Key("output2").SetValue(v)
			case v := <-switcher.outputTitles[2].C:
				sec.Key("output3").SetValue(v)
			case v := <-switcher.outputTitles[3].C:
				sec.Key("output4").SetValue(v)

			case v := <-switcher.ipAddr.C:
				sec.Key("ip").SetValue(v)

			case v := <-switcher.key.C:
				sec.Key("hotkey").SetValue(v)

				// case v := <-switcher.output.C:
				// 	sec.Key("current_output").SetValue(strconv.Itoa(v))
			}
		}
	}()
}

func (switcher *Switcher) setupConfig() {
	switcher.output.C = make(chan int)
	switcher.ipAddr.C = make(chan string)

	for i := range switcher.outputTitles {
		if switcher.outputTitles[i].C == nil {
			switcher.outputTitles[i].C = make(chan string)
			switcher.outputTitles[i].V = (fmt.Sprintf("Output %d", i+1))
		}
	}

	configPath := "config.ini"

	// Check if config exists, if not create it
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		os.Create(configPath)
	}

	cfg, err := ini.InsensitiveLoad(configPath)

	if err != nil {
		fmt.Printf("Error: %s", err.Error())
		return
	}

	sec := cfg.Section("")

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
		w := app.NewWindow(
			app.Title("Sound Brick"),
			app.Size(unit.Dp(400), unit.Dp(435)),
		)

		switcher.settings = w
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
				err := switcher.config.SaveTo("config.ini")
				if err != nil {
					fmt.Println(err)
				}

				if switcher.UDP == nil || switcher.UDP.RemoteAddr().String() != ipAddr.Text() {
					switcher.connect()
				}

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

				inputGen := func(we *widget.Editor, sc *StableChannelS, label string, widgets ...layout.FlexChild) layout.FlexChild {
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
								if content != sc.V && content != "" {
									sc.send(strings.TrimSpace(content))
								}
								// Set initial value
								if content == "" && !we.Focused() {
									we.SetText(sc.V)
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

				// Layout -------------------------------------------

				layout.Flex{
					Axis:    layout.Vertical,
					Spacing: layout.SpaceEnd,
				}.Layout(gtx,

					titleGen("Sound Brick", material.H4),

					spacer(10, "H"),

					titleGen("Labels", material.H6),
					inputGen(&outputNames[0], &switcher.outputTitles[0], "Output 1"),
					inputGen(&outputNames[1], &switcher.outputTitles[1], "Output 2"),
					inputGen(&outputNames[2], &switcher.outputTitles[2], "Output 3"),
					inputGen(&outputNames[3], &switcher.outputTitles[3], "Output 4"),

					spacer(20, "H"),

					titleGen("Connection", material.H6),
					inputGen(&ipAddr, &switcher.ipAddr, "IP Address"),

					spacer(20, "H"),

					titleGen("Controls", material.H6),
					inputGen(&key, &switcher.key, "Hotkey",
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
			outs[item].SetIcon(check.Data)
		}

		// Set initial icon
		cur, _ := switcher.config.Section("").Key("current_output").Int()
		setChecks(cur)

		for {
			select {
			case <-mMute.ClickedCh:
				if switcher.UDP == nil {
					return
				}
				if switcher.muteToggle() {
					mMute.SetTitle("Unmute")
				} else {
					mMute.SetTitle("Mute")
				}

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

			case v := <-switcher.output.C:
				if v > -1 && v < 5 {
					setChecks(v)
				}

			case <-mSettings.ClickedCh:
				go switcher.openSettings()

			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}, func() {
		switcher.exit()
	})
}

func (switcher *Switcher) exit() {
	fmt.Println("Closing...")

	switcher.config.SaveTo("config.ini")
	if switcher.UDP != nil {
		switcher.UDP.Close()
	}

	os.Exit(0)
}

func main() {
	client := &Switcher{}

	client.setupSettings()
	client.setupConfig()

	client.connect()

	client.setupTray()
	client.setupHotkeys()
}

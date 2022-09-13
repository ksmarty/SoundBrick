package main

import (
	"errors"
	"fmt"
	"image/color"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/electricbubble/go-toast"
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

func alert(title string, content string) {
	go toast.Push(content,
		toast.WithTitle(title),
		toast.WithAppID("Sound Brick"),
		toast.WithAudio(toast.Default),
		toast.WithShortDuration(),
		toast.WithIconRaw(icon.Data),
	)
}

func (switcher *Switcher) cycleOutput() {
	x := switcher.output.V
	x++
	x %= 4

	switcher.sendUDP(x)
}

func (switcher *Switcher) muteToggle() bool {
	var val int

	if switcher.output.V != -1 {
		switcher.prevOutput = switcher.output.V
		println(switcher.prevOutput)
		val = -1
		go switcher.output.send(val)
		alert("Muted!", "Output has been muted.")
	} else {
		val = switcher.prevOutput
	}

	go switcher.output.send(val)
	go switcher.sendUDP(val)

	return val == -1
}

func (switcher *Switcher) connect() {
	s, _ := net.ResolveUDPAddr("udp4", "192.168.1.71:4210")
	c, err := net.DialUDP("udp4", nil, s)

	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("The UDP server is %s\n", c.RemoteAddr().String())

	switcher.UDP = c
}

func (switcher *Switcher) setupHotkeys() {
	mainthread.Init(func() {
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()

			// https://keycode-visualizer.netlify.app/
			hk := hotkey.New([]hotkey.Modifier{}, hotkey.Key(220))

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
	fmt.Printf("Sending: %d\n", command)

	switcher.UDP.Write([]byte(strconv.Itoa(command)))

	buffer := make([]byte, 1024)
	n, _, err := switcher.UDP.ReadFromUDP(buffer)

	if err != nil {
		fmt.Println(err)
		return err.Error()
	}

	go switcher.output.send(command)
	if command > -1 && command < 5 {
		alert("Output Changed!", fmt.Sprintf("Current output: %s", switcher.outputTitles[command].V))
	}

	return string(buffer[0:n])
}

func (switcher *Switcher) setupMisc() {
	switcher.output.C = make(chan int)

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

	cfg.Section("").NewKey("output1", "Output 1")
	cfg.Section("").NewKey("output2", "Output 2")
	cfg.Section("").NewKey("output3", "Output 3")
	cfg.Section("").NewKey("output4", "Output 4")

	cfg.Section("").NewKey("current_output", "0")

	cfg.Section("").NewKey("ip", "")

	cfg.Section("").NewKey("hotkey", "220")

	switcher.config = cfg

	err = cfg.SaveTo(configPath)

	if err != nil {
		fmt.Printf("Error: %s", err.Error())
		return
	}
}

func (switcher *Switcher) setupSettings() {
	go func() {
		w := app.NewWindow(
			app.Title("Sound Brick"),
			app.Size(unit.Dp(400), unit.Dp(500)),
		)
		switcher.settings = w

		for i := range switcher.outputTitles {
			if switcher.outputTitles[i].C == nil {
				switcher.outputTitles[i].C = make(chan string)
				switcher.outputTitles[i].V = (fmt.Sprintf("Output %d", i+1))
			}
		}
	}()
}

func (switcher *Switcher) openSettings() {
	err := func() error {
		th := material.NewTheme(gofont.Collection())

		var ops op.Ops

		outputNames := [4]widget.Editor{}
		ipAddr := widget.Editor{}
		key := widget.Editor{}

		for e := range switcher.settings.Events() {
			switch e := e.(type) {
			case system.DestroyEvent:
				// Create new window when old one is destroyed
				switcher.setupSettings()
				return e.Err
			case system.FrameEvent:

				gtx := layout.NewContext(&ops, e)

				Colors := map[string]color.NRGBA{
					"bg":        {0x16, 0x16, 0x1a, 0xff},
					"focused":   {0x7f, 0x5a, 0xf0, 0xff},
					"unfocused": {0x01, 0x01, 0x01, 0xff},
					"secondary": {0x94, 0xa1, 0xb2, 0xff},
					"primary":   {0xff, 0xff, 0xfe, 0xff},
					"hint":      {0xa7, 0xa9, 0xbe, 0x30},
				}

				// Background
				paint.Fill(&ops, Colors["bg"])

				// Events -------------------------------------------

				// Layout -------------------------------------------

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

				inputGen := func(we *widget.Editor, sc *StableChannelS, label string) layout.FlexChild {
					return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						var borderColor color.NRGBA

						if we.Focused() {
							borderColor = Colors["focused"]
						} else {
							borderColor = Colors["unfocused"]
						}

						border := widget.Border{
							Color:        borderColor,
							CornerRadius: unit.Dp(3),
							Width:        unit.Dp(2),
						}

						return margins.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{
									Axis:      layout.Horizontal,
									Spacing:   layout.SpaceEnd,
									Alignment: layout.Middle,
									WeightSum: 0,
								}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										title := material.H6(th, label)
										title.Color = Colors["secondary"]
										return title.Layout(gtx)
									}),

									layout.Rigid(
										layout.Spacer{Width: unit.Dp(5)}.Layout,
									),

									layout.Flexed(100, func(gtx layout.Context) layout.Dimensions {
										ed := material.Editor(th, we, label)
										ed.Color = Colors["secondary"]
										ed.HintColor = Colors["hint"]
										we.SingleLine = true
										// Only update when text has changed
										if we.Text() != sc.V {
											// fmt.Printf("%s -> %s\n", sc.V, we.Text())
											go sc.send(we.Text())
										}
										// Set initial value
										if we.Text() == "" && !we.Focused() {
											we.SetText(sc.V)
										}
										return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return margins.Layout(gtx, ed.Layout)
										})
									}))
							},
						)
					})
				}

				titleGen := func(label string, size func(th *material.Theme, txt string) material.LabelStyle) layout.FlexChild {
					return marginLayout(func(gtx layout.Context) layout.Dimensions {
						title := size(th, label)
						title.Color = Colors["primary"]
						return title.Layout(gtx)
					})
				}

				spacer := func(size int, hw string) layout.FlexChild {
					var x layout.Spacer
					if hw == "H" {
						x.Height = unit.Dp(size)
					} else {
						x.Width = unit.Dp(size)
					}
					return layout.Rigid(x.Layout)
				}

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

					inputGen(&key, &switcher.key, "Hotkey"),
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
		systray.SetTitle("Sound Brick")
		systray.SetTooltip("Sound Brick")

		systray.AddMenuItem("Sound Brick", "Sound Brick")
		systray.AddSeparator()
		mSelect := systray.AddMenuItem("Select Output", "Select output")
		mMute := systray.AddMenuItem("Mute", "Mute devices")
		mSettings := systray.AddMenuItem("Settings", "Open settings")
		mQuit := systray.AddMenuItem("Quit", "Quit")

		outs, outsChs := [4]*systray.MenuItem{}, [4](chan struct{}){}
		for i := range outs {
			str := fmt.Sprintf("Output %d", i+1)
			outs[i] = mSelect.AddSubMenuItem(str, str)
			outsChs[i] = outs[i].ClickedCh
		}

		titlesChs := [4](chan string){}
		for i := range titlesChs {
			titlesChs[i] = switcher.outputTitles[i].C
		}

		// Combined channel for all output menu items
		outsGroup := utils.NewGroupE(outsChs)
		titlesGroup := utils.NewGroupV(titlesChs)

		// Add check icon to selected input
		setChecks := func(item int) {
			for _, v := range outs {
				v.SetIcon(blank.Data)
			}
			outs[item].SetIcon(check.Data)
		}

		// Set initial icon
		setChecks(switcher.output.V)

		for {
			select {
			case <-mMute.ClickedCh:
				if switcher.muteToggle() {
					mMute.SetTitle("Unmute")
				} else {
					mMute.SetTitle("Mute")
				}

			case v := <-outsGroup:
				switcher.sendUDP(v)
				setChecks(v)

			case v := <-switcher.output.C:
				if v > -1 && v < 5 {
					setChecks(v)
				}

			case v := <-titlesGroup:
				outs[v.Index].SetTitle(v.Msg)

			case <-mSettings.ClickedCh:
				go switcher.openSettings()

			case <-mQuit.ClickedCh:
				systray.Quit()
				fmt.Println("Closing...")
				return
			}
		}
	}, func() {
		switcher.exit()
	})
}

func (switcher *Switcher) exit() {
	switcher.UDP.Close()
	os.Exit(0)
}

func main() {
	client := &Switcher{}
	client.connect()

	client.setupMisc()
	client.setupSettings()
	client.setupTray()
	client.setupHotkeys()
}

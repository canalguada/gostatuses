package main

import (
	"fmt"
	"strings"
	"regexp"
	"os"
	"sync"
	"flag"
	"github.com/godbus/dbus/v5"
	// "github.com/canalguada/goprocfs/procmon"
)

// type (
//   DbusStatus = procmon.DbusStatus
// )

type DbusStatus struct {
	Label, Text string
}

type Color struct {
	Background string
	Highlight string
}

var reARGB = regexp.MustCompile(`(?i)^#[a-f0-9]{8}$`)

func (c Color) Rgb() Color {
	bg := c.Background
	hi := c.Highlight
	if reARGB.MatchString(bg) {
		bg = "#" + bg[3:]
	}
	if reARGB.MatchString(hi) {
		hi = "#" + hi[3:]
	}
	return Color{bg, hi}
}

type widgetStatus struct {
	DbusStatus
	Format string
}

func (ws *widgetStatus) update(value DbusStatus) {
	ws.Label = value.Label
	ws.Text = value.Text
}

var (
	kind = flag.String("type", "polybar", "widget style")
	border = flag.String("border", "none", "use border")
	highlight = flag.Bool("highlight", false, "use highlight")
	background = flag.Bool("background", false, "use background")
	text = flag.Bool("text", false, "show text only")
	padding = flag.Int("padding", 1, "padding")
	spacing = flag.Int("spacing", 1, "spacing")
	separator = flag.String("separator", " ", "use separator")
	once = flag.Bool("once", false, "print tag(s) once")
	debug = flag.Bool("debug", false, "debug")
	defaultColors = map[string]Color {
		"CpuPercent": Color{"#7fcc0000", "#c0392b"},
		"CpuFreq": Color{"#7fcc0000", "#c0392b"},
		"LoadAvg": Color{"#7fcc0000", "#c0392b"},
		"MemPercent": Color{"#5fff79c6", "#f012be"},
		"SwapUsed": Color{"#5fff79c6", "#f012be"},
		"NetDevice": Color{"#5f4e9a06", "#1cdc9a"},
		"DownSpeed": Color{"#5fffb86c", "#ff851b"},
		"DownTotal": Color{"#5fffb86c", "#ff851b"},
		"UpSpeed": Color{"#5fc4a000", "#fce947"},
		"UpTotal": Color{"#5fc4a000", "#fce947"},
		"Volume": Color{"#5f4e9a06", "#1cdc9a"},
		"Mpris": Color{"#5f4e9a06", "#1cdc9a"},
	}
)

type Widget struct {
	Statuses map[string]*widgetStatus
	ordered []string
	getter func(string, *widgetStatus) string
}

func (w *Widget) Update(tag string, s DbusStatus) {
	ws := w.Statuses[tag]
	ws.update(s)
}

func (w *Widget) Initialize(conn *dbus.Conn, iface, path string) error {
	obj := conn.Object(iface, dbus.ObjectPath(path))
	for _, tag := range w.ordered {
		var s DbusStatus
		property := fmt.Sprintf("%s.%s", iface, tag)
		if v, err := obj.GetProperty(property); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get", tag, "property:", err)
			return err
		} else if err := v.Store(&s); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get", tag, "property:", err)
			return err
		}
		if *debug {
			fmt.Println("Result from calling", tag,  "property:", s)
		}
		w.Update(tag, s)
	}
	return nil
}

func (w *Widget) HasTag(tag string) bool {
	_, found := w.Statuses[tag]
	return found
}

func (w *Widget) GetStatus(tag string) DbusStatus {
	return w.Statuses[tag].DbusStatus
}

func (w *Widget) get(tag string) string {
	ws := w.Statuses[tag]
	raw := (w.getter)(tag, ws)
	return strings.Replace(ws.Format, "[CONTENT]", raw, 1)

}

func (w *Widget) String() string {
	var items []string
	var pad string
	if *spacing >= 0 && *spacing < 5 {
		pad = strings.Repeat(*separator, *spacing)
	}
	for _, tag := range w.ordered {
		items = append(items, w.get(tag))
	}
	return strings.Join(items, pad)
}

// type widgetPointer interface {
//   Update(string, DbusStatus)
//   Initialize(*dbus.Conn, string, string) error
//   HasTag(string) bool
//   GetStatus(string) DbusStatus
//   String() string
// }

// Example: CpuPercent
// %{o#c0392b}%{+o}%{B#7fcc0000} %{F#c0392b} %{F-}  92% %{B-}%{-o}

func newPolybarWidget(tags []string) *Widget {
	w := &Widget{}
	w.Statuses = make(map[string]*widgetStatus)
	var s string
	if *padding >= 0 && *padding < 5 {
		s = strings.Repeat(` `, *padding)
	}
	for _, tag := range tags {
		if c, found := defaultColors[tag]; found {
			ws := &widgetStatus{Format: "[CONTENT]"}
			ws.Format = s + ws.Format + s
			if *background {
				ws.Format = "%{B" + c.Background + "}" + ws.Format + "%{B-}"
			}
			switch *border {
			case `overline`:
				ws.Format = "%{o" + c.Highlight + "}%{+o}" + ws.Format + "%{-o}"
			case `underline`:
				ws.Format = "%{u" + c.Highlight + "}%{+u}" + ws.Format + "%{-u}"
			}
			w.Statuses[tag] = ws
			w.ordered = append(w.ordered, tag)
		}
	}
	w.getter = func(tag string, ws *widgetStatus) string {
		raw := ws.Text
		if !(*text) && len(ws.Label) > 0 {
			if *highlight {
				raw = fmt.Sprintf(
					"%%{F%s}%s%%{F-} %s",
					defaultColors[tag].Highlight,
					ws.Label,
					raw,
				)
			} else {
				raw = fmt.Sprint(ws.Label, raw)
			}
		}
		return raw
	}
	return w
}

func newTmuxNovaWidget(tags []string) *Widget {
	w := &Widget{}
	w.Statuses = make(map[string]*widgetStatus)
	var s string
	if *padding >= 0 && *padding < 5 {
		s = strings.Repeat(` `, *padding)
	}
	for _, tag := range tags {
		if c, found := defaultColors[tag]; found {
			ws := &widgetStatus{Format: "[CONTENT]"}
			ws.Format = s + ws.Format + s
			var fg string = "default"
			var bg string = "default"
			if *highlight {
				fg = c.Rgb().Highlight  // "#ffffff"
			}
			if *background {
				bg = c.Rgb().Background  // "#000000"
			}
			ws.Format = "#[fg=" + fg + ",bg=" + bg + "]" + ws.Format
			ws.Format += "#[fg=default,bg=default]"
			w.Statuses[tag] = ws
			w.ordered = append(w.ordered, tag)
		}
	}
	w.getter = func(tag string, ws *widgetStatus) string {
		raw := ws.Text
		if !(*text) && len(ws.Label) > 0 {
			raw = fmt.Sprint(ws.Label, raw)
		}
		return raw
	}
	return w
}

type rule struct {
	Type string
	Member string
	Path string
	Interface string
}

func (r rule) String() string {
	return fmt.Sprintf(
		"type='%s',member='%s',path='%s',interface='%s'",
		r.Type,
		r.Member,
		r.Path,
		r.Interface,
	)
}

func main() {
	// args := os.Args[1:]
  //
	// if len(args) == 0 {
	//   os.Exit(1)
	// }
	flag.Parse()
	tags := flag.Args()
	if len(tags) == 0 {
		os.Exit(1)
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to connect to session bus:", err)
		os.Exit(1)
	}
	defer conn.Close()
	// Widget initialization
	var w *Widget
	switch *kind {
	case "polybar":
		w = newPolybarWidget(tags)
	case "tmux":
		w = newTmuxNovaWidget(tags)
	default:
		os.Exit(1)
	}
	if err := w.Initialize(
		conn,
		"com.github.canalguada.gostatuses",
		"/com/github/canalguada/gostatuses",
	); err != nil {
		os.Exit(1)
	}
	fmt.Println(w)
	// Exit when required
	if *once {
		os.Exit(0)
	}
	// Monitoring
	rules := []string{
		rule{
			`signal`,
			`PropertiesChanged`,
			`/com/github/canalguada/gostatuses`,
			`org.freedesktop.DBus.Properties`,
		}.String(),
	}
	var flag uint = 0
	call := conn.BusObject().Call(
		"org.freedesktop.DBus.Monitoring.BecomeMonitor",
		0,
		rules,
		flag,
	)
	if call.Err != nil {
		fmt.Fprintln(os.Stderr, "Failed to become monitor:", call.Err)
		os.Exit(1)
	}
	c := make(chan *dbus.Message, 10)
	conn.Eavesdrop(c)
	for v := range c {
		var updated bool
		var wg sync.WaitGroup
		// fmt.Printf("%#v\n", v.Body)
		changed := (v.Body[1]).(map[string]dbus.Variant)
		for tag, variant := range changed {
			wg.Add(1)
			go func(tag string, variant dbus.Variant) {
				defer wg.Done()
				var value DbusStatus
				// check if tracking this property
				if w.HasTag(tag) {
					// get value
					if err := variant.Store(&value); err == nil {
						// update widgetStatus
						if value != w.GetStatus(tag) {
							w.Update(tag, value)
							updated = true
						}
						// fmt.Printf("%s: '%s:%s'\n", tag, value.Icon, value.Text)
					}
				}
			}(tag, variant)
		}
		wg.Wait() // wait on the workers to finish
		// if updated, print Widget
		if updated {
			fmt.Println(w)
		}
	}
}

// vim: set ft=go fdm=indent ts=2 sw=2 tw=79 noet:

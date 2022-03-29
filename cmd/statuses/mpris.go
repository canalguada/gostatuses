package main

import (
	"fmt"
	"strconv"
	"strings"
	"sort"
	"os"
	"sync"
	"github.com/godbus/dbus/v5"
	mpris "github.com/Pauloo27/go-mpris"
)

var defaultStatus Status = Status{
	Content: Content{Label: mprisDefaults[`iconStopped`], Value: ""},
	Tag: `Mpris`,
}

var mprisDefaults = map[string]string{
	`truncate`: `…`,
	`iconPlaying`: ``,
	`iconPaused`: ``,
	`iconStopped`: ``,
	`iconNone`: ``,
}

func init() {
	defaultStatus.SetFormat("%s")
}

// Properties
type Properties struct {
	PlaybackStatus mpris.PlaybackStatus
	Metadata map[string]dbus.Variant
	Icon string
	NowPlaying string
}

func GetPlayerProperties(player *mpris.Player) (p Properties, err error) {
	playbackStatus, e := player.GetPlaybackStatus()
	if e != nil {
		err = e
		return
	}
	metadata, e := player.GetMetadata()
	if e != nil {
		err = e
		return
	}
	p = Properties{PlaybackStatus: playbackStatus, Metadata: metadata}
	return
}

func (p *Properties) GetIcon() (icon string) {
	switch p.PlaybackStatus {
	case mpris.PlaybackPlaying:
		icon = mprisDefaults[`iconPlaying`]
	case mpris.PlaybackPaused:
		icon = mprisDefaults[`iconPaused`]
	case mpris.PlaybackStopped:
		icon = mprisDefaults[`iconStopped`]
	default:
		icon = mprisDefaults[`iconNone`]
	}
	p.Icon = icon
	return
}

func truncate(text string, length int) (result string) {
	if len(text) > length {
		result = fmt.Sprintf("%s%s", text[:length], mprisDefaults[`truncate`])
	} else {
		result = text
	}
	return
}

func (p *Properties) GetArtist() (artist string) {
	for _, key := range []string{`xesam:artist`, `xesam:albumArtist`} {
		if variant, found := p.Metadata[key]; found {
			if value, ok := variant.Value().([]string); ok {
				if len(value) > 0 {
					artist = value[0]
					break
				}
			}
		}
	}
	return
}

func (p *Properties) GetTitle() (title string) {
	if variant, found := p.Metadata[`xesam:title`]; found {
		if value, ok := variant.Value().(string); ok {
			title = value
		}
	}
	return
}

func (p *Properties) GetUrl() (url string) {
	if variant, found := p.Metadata[`xesam:url`]; found {
		if value, ok := variant.Value().(string); ok {
			url = value
		}
	}
	return
}

func (p *Properties) GetComment() (comment string) {
	if variant, found := p.Metadata[`xesam:comment`]; found {
		if value, ok := variant.Value().(string); ok {
			comment = value
		}
	}
	return
}

func (p *Properties) GetNowPlaying() (nowPlaying string) {
	for _, key := range []string{`cmus:stream_title`, `vlc:nowplaying`} {
		if variant, found := p.Metadata[key]; found {
			if value, ok := variant.Value().(string); ok {
				nowPlaying = value
				break
			}
		}
	}
	if len(nowPlaying) == 0 {
		artist := p.GetArtist()
		title := p.GetTitle()
		url := p.GetUrl()
		comment := p.GetComment()
		// got title
		if len(title) > 0 {
			nowPlaying = title
			if len(artist) > 0 {
				if len(url) == 0 || !(strings.HasPrefix(url, `http`)) {
					nowPlaying = fmt.Sprintf("%s - %s", artist, nowPlaying)
				}
			}
		// no title
		} else if len(url) > 0 {
			nowPlaying = url
		} else if len(comment) > 0 {
			nowPlaying = comment
		}
	}
	p.NowPlaying = nowPlaying
	return
}
// Properties end

// weightOwner
type weightOwner struct {
	id int
	weight int
	owner string
	status string
}

func (w *weightOwner) setOwner(owner string) int {
	w.owner = owner
	tokens := strings.Split(owner, `.`)
	if value, err := strconv.Atoi(tokens[len(tokens) - 1]); err == nil {
		w.id = value
	}
	return w.id
}

func (w *weightOwner) setStatus(playbackStatus mpris.PlaybackStatus) int {
	w.status = string(playbackStatus)
	switch playbackStatus {
	case "Playing":
		w.weight = 2
	case "Paused":
		w.weight = 1
	}
	return w.weight
}

func getWeightOwner(owner string, player *mpris.Player) (w weightOwner) {
	w.setOwner(owner)
	if playbackStatus, err := player.GetPlaybackStatus(); err == nil {
		w.setStatus(playbackStatus)
	} else {
		w.status = `Stopped`
	}
	return
}

type byWeight []weightOwner
func (b byWeight) Len () int { return len(b) }
func (b byWeight) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byWeight) Less(i, j int) bool {
	switch {
	case b[i].weight < b[j].weight:
		return true
	case b[i].weight == b[j].weight:
		return b[i].id < b[j].id
	}
	return false
}
// weightOwner end

// Listener
type Listener struct {
	conn *dbus.Conn
	chanSignal chan *dbus.Signal
	// ignoredPlayers []string
	players map[string]*mpris.Player
	lastStatus Status
	statusOwner string
	ownerProperties Properties
	connected bool
	// TODO: try to isolate
	// chanStatus chan Status
}

func NewListener() *Listener {
	l := &Listener{}
	l.chanSignal = make(chan *dbus.Signal, 16)
	l.players = make(map[string]*mpris.Player)
	l.lastStatus = defaultStatus
	return l
}

func (l *Listener) Close() {
	l.disconnect()
}

func (l *Listener) handlePropertiesChanged(message *dbus.Signal) bool {
	busName := fmt.Sprintf("%s", message.Body[0])
	if busName != "org.mpris.MediaPlayer2.Player" {
		return false
	}
	if _, found := l.players[message.Sender]; found {
		if properties, ok := message.Body[1].(map[string]dbus.Variant); ok {
			for key, _ := range properties {
				switch key {
				case `PlaybackStatus`, `Volume`, `Metadata`:
					return true
				}
			}
		}
	}
	return false
}

// func (l *Listener) handleSeeked(message *dbus.Signal) bool{
//   return false
// }

func (l *Listener) handleNameOwnerChanged(message *dbus.Signal) bool {
	busName := fmt.Sprintf("%s", message.Body[0])
	if !(l.isValidPlayer(busName)) {
		return false
	}
	oldOwner := fmt.Sprintf("%s", message.Body[1])
	newOwner := fmt.Sprintf("%s", message.Body[2])
	if len(newOwner) > 0 {
		if len(oldOwner) > 0 {
			return l.changePlayerOwner(busName, oldOwner, newOwner)
		} else {
			return l.addPlayer(busName, newOwner)
		}
	} else if len(oldOwner) > 0 {
			return l.removePlayer(busName, oldOwner)
	}
	return false
}

func (l *Listener) HandleSignal(message *dbus.Signal, channel chan Status) {
	// TODO: suppress after debug
	// fmt.Printf("debug: got signal: %v sender: %v\n", message.Name, message.Sender)
	var flag bool
	switch message.Name {
	case "org.freedesktop.DBus.Properties.PropertiesChanged":
		flag = l.handlePropertiesChanged(message)
	// case "org.mpris.MediaPlayer2.Seeked":
	//   flag = l.handleSeeked(message)
	case "org.freedesktop.DBus.NameOwnerChanged":
		flag = l.handleNameOwnerChanged(message)
	}
	if flag {
		go l.RefreshStatus(channel)
	}
}

func (l *Listener) connect(conn *dbus.Conn, channel chan Status) {
	l.conn = conn
	l.connected = true
	l.getRunningPlayers(channel)
	// Start listenning to some signals on various interfaces
	fmt.Println("Connecting signals...")
	var matchOptions = map[string]map[string]string{
		`PropertiesChanged`: map[string]string{
			`opath`: `/org/mpris/MediaPlayer2`,
			`iface`: `org.freedesktop.DBus.Properties`,
		},
		// `Seeked`: map[string]string{
		//   `opath`: `/org/mpris/MediaPlayer2`,
		//   `iface`: `org.mpris.MediaPlayer2`,
		// },
		`NameOwnerChanged`: map[string]string{
			`opath`: `/org/freedesktop/DBus`,
			`iface`: `org.freedesktop.DBus`,
		},
	}
	for member, options := range matchOptions {
		if err := l.conn.AddMatchSignal(
			dbus.WithMatchObjectPath(dbus.ObjectPath(options[`opath`])),
			dbus.WithMatchInterface(options[`iface`]),
			dbus.WithMatchMember(member),
		); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	l.conn.Signal(l.chanSignal)
}

func (l *Listener) disconnect() {
	l.connected = false
	// Disconnect players if required with lib
}

func (l *Listener) isValidPlayer(name string) bool {
	return strings.HasPrefix(name, `org.mpris.MediaPlayer2`)
}

func (l *Listener) listBusNames() []string {
	// var s []string
	// err = l.conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&s)
	// if err != nil {
	//   fmt.Fprintln(os.Stderr, "Failed to get list of owned names:", err)
	//   os.Exit(1)
	// }
	// return s
	return l.conn.Names()
}

func (l *Listener) getNameOwner(name string) (owner string, err error) {
	iface := "org.freedesktop.DBus.GetNameOwner"
	err = l.conn.BusObject().Call(iface, 0, name).Store(&owner)
	return
}

func (l *Listener) getRunningPlayers(channel chan Status) {
	if names, err := mpris.List(l.conn); err == nil {
		for _, name := range names {
			if !(l.isValidPlayer(name)) {
				continue
			}
			if owner, err := l.getNameOwner(name); err == nil {
				l.addPlayer(name, owner)
			}
		}
	} else {
		fmt.Fprintln(os.Stderr, "failed to get list of owned players names:", err)
		os.Exit(1)
	}
	// TODO: suppress after debug
	// fmt.Printf("debug: initial players %+v\n", l.players)
	go l.RefreshStatus(channel)
}

func (l *Listener) addPlayer(busName, owner string) (flag bool) {
	// TODO: suppress after debug
	// fmt.Printf("debug: adding player %s[%s]\n", busName, owner)
	l.players[owner] = mpris.New(l.conn, busName)
	flag = true
	return
}

func (l *Listener) removePlayer(busName, owner string) (flag bool) {
	if _, found := l.players[owner]; found {
		// TODO: suppress after debug
		// fmt.Printf("debug: removing player %s[%s]\n", busName, owner)
		delete(l.players, owner)
		flag = true
	}
	return
}

func (l *Listener) changePlayerOwner(busName, oldOwner, newOwner string) (flag bool) {
	if l.removePlayer(busName, oldOwner) {
		flag = true
	}
	if l.addPlayer(busName, newOwner) {
		flag = true
	}
	return
}

func (l *Listener) getStatusOwner() (statusOwner string) {
	var owners []weightOwner
	for owner, player := range l.players {
		owners = append(owners, getWeightOwner(owner, player))
	}
	// TODO: suppress after debug
	// fmt.Printf("debug: owners %+v\n", owners)
	if len(owners) > 0 {
		sort.Sort(byWeight(owners))
		statusOwner = fmt.Sprintf("%s", owners[len(owners) - 1].owner)
	}
	l.statusOwner = statusOwner
	return
}

func (l *Listener) getPlayerStatus(owner string) (s Status, err error) {
	s = defaultStatus
	properties, e := GetPlayerProperties(l.players[owner])
	if e != nil {
		err = e
		return
	}
	l.ownerProperties = properties
	s.Label = l.ownerProperties.GetIcon()
	if s.Label == mprisDefaults[`iconStopped`] {
		return
	}
	s.Value = truncate(l.ownerProperties.GetNowPlaying(), 96)
	return
}

func (l *Listener) getStatus() (s Status) {
	if len(l.getStatusOwner()) > 0 {
		if status, err := l.getPlayerStatus(l.statusOwner); err == nil {
			s = status
			// TODO: suppress after debug
			// fmt.Printf("debug: current player status: %+v\n", s)
		}
	} else {
		s = defaultStatus
	}
	return
}

func (l *Listener) RefreshStatus(channel chan Status) {
	var flag bool
	s := l.getStatus()
	if s.Label != l.lastStatus.Label {
		l.lastStatus.Label = s.Label
		flag = true
	}
	if s.Value != l.lastStatus.Value {
		l.lastStatus.Value = s.Value
		flag = true
	}
	// TODO: suppress after debug
	// fmt.Printf("debug: updated status: %+v\n", s)
	// fmt.Printf("debug: last status: %+v\n", l.lastStatus)
	if flag {
		channel <- l.lastStatus
	}
}
// Listener end

func GetMprisResource() *Resource {
	rc := NewResource("mpris", "mpris")
		rc.SetData(`owner`, 0)
		s := rc.AddStatus(`Mpris`)
		s.SetFormat("%s")
		s.SetLabel(mprisDefaults[`iconStopped`])
		s.SetValue("")
	return rc
}

type MprisClient struct {
	client *Listener
	Channel chan *dbus.Signal
	Rc *Resource
}

func NewMprisClient() MprisClient {
	c := MprisClient{}
	c.client = NewListener()
	c.Channel = c.client.chanSignal
	// c.client.connect(channel)
	c.Rc = GetMprisResource()
	return c
}

func (c MprisClient) Connect(conn *dbus.Conn, chanStatus chan Status) {
	c.client.connect(conn, chanStatus)
}

func (c MprisClient) Close() {
	c.client.Close()
}

func (c MprisClient) UpdateMpris(message *dbus.Signal, chanStatus chan Status) {
	c.client.HandleSignal(message, chanStatus)
}

func (c MprisClient) Updater(chanStatus chan Status, wg sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case message := <-c.Channel:
			c.client.HandleSignal(message, chanStatus)
		}
	}
}

// vim: set ft=go fdm=indent ts=2 sw=2 tw=79 noet:

package main

import (
	"sync"
	"math"
	"github.com/pr11t/pulseaudio"
)

func GetPulseResource() *Resource {
	rc := NewResource("pulse", "pulse")
		rc.SetData(`mute`, 0)
		rc.SetData(`volume`, 0)
		s := rc.AddStatus(`Volume`)
		s.SetFormat("%3d")
		s.AddUnit(NewUnit(1, "%"))
		s.SetLabel(``)
	return rc
}

func GetVolumeIcon(value int, mute bool) (icon string) {
	if mute {
		icon = ``
	} else {
		switch {
		case value < 16:
			icon = ``
		case value >= 16 && value < 50:
			icon = ``
		case value >= 50:
			icon = ``
		}
	}
	return
}

type PulseClient struct {
	client *pulseaudio.Client
	Channel <-chan struct{}
	Rc *Resource
}

func NewPulseClient() PulseClient {
	c := PulseClient{}
	client, err := pulseaudio.NewClient()
	if err != nil {
		panic(err)
	}
	channel, err := client.Updates()
	if err != nil {
		panic(err)
	}
	c.client = client
	c.Channel = channel
	c.Rc = GetPulseResource()
	return c
}

func (c PulseClient) Close() {
	c.client.Close()
}

func (c PulseClient) UpdateVolume(chanStatus chan Status) (err error){
	mute, err := c.client.Mute()
	if err != nil {
		return
	}
	volume, err := c.client.Volume()
	if err != nil {
		return
	}
	value := int(math.Round(float64(volume * 100.0)))
	if mute != (c.Rc.GetData(`mute`) == 1) || value != c.Rc.GetData(`volume`) {
		if s, found := c.Rc.Statuses[`Volume`]; found {
			s.Label = GetVolumeIcon(value, mute)
			s.Value = value
			chanStatus <- *s
		}
		if mute {
			c.Rc.SetData(`mute`, 1)
		} else {
			c.Rc.SetData(`mute`, 0)
		}
		c.Rc.SetData(`volume`, value)
	}
	return
}

func (c PulseClient) Updater(chanStatus chan Status, wg sync.WaitGroup) {
	defer wg.Done()
	c.UpdateVolume(chanStatus)
	for {
		select {
		case <-c.Channel:
			c.UpdateVolume(chanStatus)
		}
	}
}

// vim: set ft=go fdm=indent ts=2 sw=2 tw=79 noet:

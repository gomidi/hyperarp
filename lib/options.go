package hyperarp

import (
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/channel"
)

type Option func(a *Arp)

func Tempo(bpm float64) Option {
	return func(a *Arp) {
		a.tempoBPM = bpm
	}
}

// NotePoolOctave sets the octave that defines the note pool, instead of the starting note
func NotePoolOctave(oct uint8) Option {
	return func(a *Arp) {
		a.notePoolOctave = oct
	}
}

// CCDirectionSwitch sets the controller for the direction switch
func CCDirectionSwitch(controller uint8) Option {
	return func(a *Arp) {
		a.directionSwitchHandler = func(msg midi.Message) (down bool, ok bool) {
			cc, is := msg.(channel.ControlChange)

			if !is || cc.Controller() != controller {
				return
			}

			ch := a.ControlChannel()
			if ch >= 0 && uint8(ch) != cc.Channel() {
				return
			}

			return cc.Value() > 0, true
		}
	}
}

// NoteDirectionSwitch sets the key for the direction switch
func NoteDirectionSwitch(key uint8) Option {
	return func(a *Arp) {
		a.directionSwitchHandler = func(msg midi.Message) (down bool, ok bool) {
			ch := a.ControlChannel()
			switch v := msg.(type) {
			case channel.NoteOn:
				if ch >= 0 && uint8(ch) != v.Channel() {
					return
				}
				if v.Key() != key {
					return
				}
				ok = true
				down = v.Velocity() > 0
			case channel.NoteOff:
				if ch >= 0 && uint8(ch) != v.Channel() {
					return
				}
				if v.Key() != key {
					return
				}
				ok = true
			case channel.NoteOffVelocity:
				if ch >= 0 && uint8(ch) != v.Channel() {
					return
				}
				if v.Key() != key {
					return
				}
				ok = true
			}
			return
		}
	}
}

// CCTimeInterval sets the controller for the time interval
func CCTimeInterval(controller uint8) Option {
	return func(a *Arp) {
		a.noteDistanceHandler = func(msg midi.Message) (dist float64, ok bool) {
			dist = -1
			cc, is := msg.(channel.ControlChange)

			if !is || cc.Controller() != controller {
				return
			}

			ch := a.ControlChannel()
			if ch >= 0 && uint8(ch) != cc.Channel() {
				return
			}

			ok = true
			if cc.Value() > 0 {
				dist = noteDistanceMap[cc.Value()%16]
			}
			return
		}
	}
}

// NoteTimeInterval sets the key for the time interval
func NoteTimeInterval(key uint8) Option {
	return func(a *Arp) {
		a.noteDistanceHandler = func(msg midi.Message) (dist float64, ok bool) {
			dist = -1
			ch := a.ControlChannel()
			switch v := msg.(type) {
			case channel.NoteOn:
				if ch >= 0 && uint8(ch) != v.Channel() {
					return
				}
				if v.Key() != key {
					return
				}
				ok = true
				if v.Velocity() > 0 {
					dist = noteDistanceMap[v.Velocity()%12]
				}
			case channel.NoteOff:
				if ch >= 0 && uint8(ch) != v.Channel() {
					return
				}
				if v.Key() != key {
					return
				}
				ok = true
			case channel.NoteOffVelocity:
				if ch >= 0 && uint8(ch) != v.Channel() {
					return
				}
				if v.Key() != key {
					return
				}
				ok = true
			}
			return
		}
	}
}

// CCStyle sets the controller for the playing style (staccato, legato, non-legato)
func CCStyle(controller uint8) Option {
	return func(a *Arp) {
		a.styleHandler = func(msg midi.Message) (val uint8, ok bool) {
			cc, is := msg.(channel.ControlChange)

			if !is || cc.Controller() != controller {
				return
			}

			ch := a.ControlChannel()
			if ch >= 0 && uint8(ch) != cc.Channel() {
				return
			}

			ok = true
			if cc.Value() > 0 {
				val = cc.Value()
			}
			return
		}
	}
}

// NoteStyle sets the key for the playing style (staccato, legato, non-legato)
func NoteStyle(key uint8) Option {
	return func(a *Arp) {
		a.styleHandler = func(msg midi.Message) (val uint8, ok bool) {
			ch := a.ControlChannel()
			switch v := msg.(type) {
			case channel.NoteOn:
				if ch >= 0 && uint8(ch) != v.Channel() {
					return
				}
				if v.Key() != key {
					return
				}
				ok = true
				if v.Velocity() > 0 {
					val = v.Velocity()
				}
			case channel.NoteOff:
				if ch >= 0 && uint8(ch) != v.Channel() {
					return
				}
				if v.Key() != key {
					return
				}
				ok = true
			case channel.NoteOffVelocity:
				if ch >= 0 && uint8(ch) != v.Channel() {
					return
				}
				if v.Key() != key {
					return
				}
				ok = true
			}
			return
		}
	}
}

// ControlChannel sets a separate MIDI channel for the control messages
func ControlChannel(ch uint8) Option {
	return func(a *Arp) {
		if ch < 16 {
			a.controlchannelIn = int8(ch)
		}
	}
}

// ChannelIn sets the midi channel to listen to (0-15)
func ChannelIn(ch uint8) Option {
	return func(a *Arp) {
		if ch < 16 {
			a.channelIn = int8(ch)
		}
	}
}

// Transpose sets the transposition for the midi
func Transpose(halfnotes int8) Option {
	return func(a *Arp) {
		a.transpose = halfnotes
	}
}

// ChannelOut sets the midi channel to write to
func ChannelOut(ch uint8) Option {
	return func(a *Arp) {
		if ch < 16 {
			a.channelOut = ch
		}
	}
}

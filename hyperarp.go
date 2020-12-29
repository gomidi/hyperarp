package hyperarp

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/cc"
	"gitlab.com/gomidi/midi/midimessage/channel"
	"gitlab.com/gomidi/midi/midimessage/realtime"
	"gitlab.com/gomidi/midi/reader"
	"gitlab.com/gomidi/midi/writer"
)

/*
idea:

the concept is similar to an arpeggiator, but with some differences


there are separate controls that can be controlled indepedantly from each other:

1. note-pool selection (may be a scale or some other notes, including a placeholder for rests) (maybe just pressed keys of a octave, then the velocities of the pressed notes will be tracked to know them when playing, polyaftertouch messages may alter them)
2. direction: up, down, repetition (with foot-pedal? pressed: repetition, otherwise switch between up and down)
3. note-time-distance: 16th, 8th, 4th, triplets, etc. (maybe controller knob)
4. note-length: legato, non-legato (2/3 of time to next note-time-distance, staccato: 1/5 of time to next note-time-distance
(maybe controller knob: staccato = 0-10, legato = 117-127 and non-legato anything > 10 and < 117
5. swing (%) (controller knob)
6. starting note: interrupts current processing to start new playing from new note (velocity will be tracked to
(7. rhythm (pattern of on and offs and velocities); maybe not needed, first implementation probably without)


multiple of these arps can be combined in a way that you can switch between them or
that they can interrupt each other (need different starting times or note-time-distances), last wins.
(may be pads on different midi channel)

the big question is: how to control that beast ;-)
*/

type note uint8

const (
	C   note = 0
	Cis      = iota
	D
	Dis
	E
	F
	Fis
	G
	Gis
	A
	Ais
	B
)

var noteDistanceMap = map[uint8]float64{
	0:  2.0,               // half notes
	1:  1.0,               // quarter notes
	2:  0.5,               // eighths
	3:  0.25,              // sixteenths
	4:  0.125,             // 32ths
	5:  2.0 / 3.0,         // half note tripplets
	6:  1.0 / 3.0,         // quarter note tripplets
	7:  0.5 / 3.0,         // eighths tripples
	8:  0.25 / 3.0,        // sixteenths tripples
	9:  0.125 / 3.0,       // 32ths tripples
	10: 1.0 * 3.0 / 2.0,   // dotted quarter notes
	11: 0.5 * 3.0 / 2.0,   // dotted eighths
	12: 0.25 * 3.0 / 2.0,  // dotted sixteenths
	13: 0.125 * 3.0 / 2.0, // dotted 32ths
	14: 1.0 / 5.0,         // quarter note quintuplets
	15: 0.5 / 5.0,         // eighths quintuplets
}

// calcNextNote calculates the next note that will be played
func (a *Arp) calcNextNote() (key, velocity uint8) {
	//fmt.Printf("calcNextNote\n")
	a.Lock("calcNextNote")
	dir := a.directionUp
	notes := a.notes
	vels := a.noteVelocities
	startkey := a.startingNote
	startVel := a.startVelocity
	lastNote := a.lastNote

	defer a.Unlock("calcNextNote")

	//fmt.Printf("direction is %v\n", dir)

	/*
		if dir == 0 { // repetition
			if lastNote == 0 {
				lastNote = startkey
			}
			vel := vels[note(lastNote%12)]

			if lastNote == startkey {
				vel = startVel
			}

			//fmt.Printf("note (repeat) is %v\n", lastNote)

			return lastNote, vel
		}
	*/

	var notePool []int // = make([]int, len(notes)+1)
	notePool = append(notePool, int(startkey%12))

	for nt, ok := range notes {
		if ok && int(uint8(nt)) != int(startkey%12) {
			notePool = append(notePool, int(uint8(nt)))
		}
	}

	sort.Ints(notePool)

	//fmt.Printf("notePool: %v\n", notePool)
	//fmt.Printf("notes: %v\n", notes)

	var lastIdx int = -1
	var lastOctave = int(lastNote / 12)

	for i, n := range notePool {
		if int(lastNote%12) == n {
			lastIdx = i
			break
		}
	}

	switch dir {
	case true:
		nextidx := (lastIdx + 1) % len(notePool)
		nextNote := notePool[nextidx]
		vel := vels[note(uint8(nextNote%12))]
		if uint8(nextNote) == startkey%12 {
			vel = startVel
		}
		nextNote = nextNote + (12 * lastOctave)
		if nextNote < int(lastNote) {
			nextNote += 12
		}

		if nextNote > (127 - int(startkey%12)) {
			//a.Lock("calcNextNote: begin with startKey")
			nextNote = int(startkey)
			a.lastNote = startkey
			//a.Unlock("calcNextNote: begin with startKey")
		}

		//fmt.Printf("note (up) is %v\n", nextNote)
		return uint8(nextNote), vel
	case false:
		if lastIdx == 0 {
			lastIdx = len(notePool)
		}
		nextidx := (lastIdx - 1) % len(notePool)
		nextNote := notePool[nextidx]
		vel := vels[note(uint8(nextNote%12))]
		if uint8(nextNote) == startkey%12 {
			vel = startVel
		}
		nextNote = nextNote + (12 * lastOctave)
		if nextNote > int(lastNote) {
			nextNote -= 12
		}

		if nextNote < int(startkey%12) {
			//a.Lock("calcNextNote: begin with startKey")
			nextNote = int(startkey)
			a.lastNote = startkey
			//a.Unlock("calcNextNote: begin with startKey")
		}

		if nextNote < 0 {
			nextNote = 0
		}

		//fmt.Printf("note (down) is %v\n", nextNote)
		return uint8(nextNote), vel
	default:
		panic("unreachable")
	}
}

type Arp struct {
	tempoBPM         float64
	directionUp      bool // -1 down, 0 repeat, 1 up
	isRunning        bool
	notes            map[note]bool
	noteVelocities   map[note]uint8
	runningNote      int8
	noteDistance     float64 // 1 = quarter note, 0.5 = eigths etc.
	startingNote     uint8
	startVelocity    uint8
	style            int // -1 staccato, 0 non-legato, 1 legato
	lastNote         uint8
	notePoolOctave   uint8
	swing            float32 // %
	transpose        int8
	channelIn        int8 // -1 = all channels
	controlchannelIn int8 // -1 = same as channelIn
	channelOut       uint8
	in               midi.In
	out              midi.Out
	wr               *writer.Writer
	sync.RWMutex

	start             chan [2]uint8
	stop              chan bool
	stopped           chan bool
	noteDist          chan time.Duration
	noteLen           chan time.Duration
	finishScheduler   chan bool
	finishListener    chan bool
	finishedScheduler chan bool
	finishedListener  chan bool
	messages          chan midi.Message
	nextArpNote       chan bool

	noteDistanceHandler    func(midi.Message) (dist float64, ok bool)
	directionSwitchHandler func(midi.Message) (down bool, ok bool)
	styleHandler           func(midi.Message) (val uint8, ok bool)
}

type Option func(a *Arp)

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

// New returns a new Arp, receiving from the given midi.In port and writing to the given midi.Out port
func New(in midi.In, out midi.Out, opts ...Option) *Arp {
	a := &Arp{
		in:                in,
		out:               out,
		notePoolOctave:    0,
		channelIn:         -1,
		channelOut:        0,
		start:             make(chan [2]uint8, 100),
		stop:              make(chan bool),
		stopped:           make(chan bool),
		noteDist:          make(chan time.Duration),
		noteLen:           make(chan time.Duration),
		messages:          make(chan midi.Message, 100),
		nextArpNote:       make(chan bool),
		finishScheduler:   make(chan bool),
		finishListener:    make(chan bool),
		finishedScheduler: make(chan bool),
		finishedListener:  make(chan bool),
		//playnotes:         make(chan playnote),
		//stopPlaynotes:     make(chan bool),
	}

	CCDirectionSwitch(cc.GeneralPurposeButton1Switch)(a)
	CCTimeInterval(cc.GeneralPurposeSlider1)(a)
	CCStyle(cc.GeneralPurposeSlider2)(a)

	for _, opt := range opts {
		opt(a)
	}

	a.Reset()
	return a
}

func (a *Arp) ControlChannel() int8 {
	if a.controlchannelIn < 0 {
		return a.channelIn
	}
	return a.controlchannelIn
}

func (a *Arp) Reset() {
	a.Lock("Reset")
	a.notes = map[note]bool{}
	a.noteVelocities = map[note]uint8{}
	a.tempoBPM = 120.00
	a.noteDistance = 0.5
	a.directionUp = true
	a.runningNote = -1
	a.Unlock("Reset")
}

func (a *Arp) SetTempo(bpm float64) {
	a.Lock("SetTempo")
	a.tempoBPM = bpm
	a.Unlock("SetTempo")
}

func (a *Arp) SwitchDirection(down bool) {
	//fmt.Println("switching direction")
	a.Lock("SwitchDirection")
	a.directionUp = !down
	a.Unlock("SwitchDirection")
}

func (a *Arp) AddNote(key, velocity uint8) {
	a.Lock("AddNote")
	a.notes[note(key%12)] = true
	a.noteVelocities[note(key%12)] = velocity
	a.Unlock("AddNote")
}

func (a *Arp) SetNoteVelocity(key, velocity uint8) {
	a.Lock("SetNoteVelocity")
	a.noteVelocities[note(key%12)] = velocity
	a.Unlock("SetNoteVelocity")
}

func (a *Arp) RemoveNote(key uint8) {
	a.Lock("RemoveNote")
	if _, has := a.notes[note(key%12)]; has {
		delete(a.notes, note(key%12))
	}
	a.Unlock("RemoveNote")
}

func (a *Arp) StartWithNote(key, velocity uint8) {
	a.Lock("StartWithNote")
	a.startingNote = key
	a.lastNote = key
	a.startVelocity = velocity
	a.isRunning = true
	a.Unlock("StartWithNote")
	a.start <- [2]uint8{key, velocity}
}

func (a *Arp) StopWithNote(key uint8) {
	a.RLock()
	startingNote := a.startingNote
	a.RUnLock()

	if startingNote == key {
		a.Stop()
	}
}

// TODO maybe remove
func (a *Arp) SetStartNoteVelocity(velocity uint8) {
	a.Lock("SetStartNoteVelocity")
	a.startVelocity = velocity
	a.Unlock("SetStartNoteVelocity")
}

func (a *Arp) SetStyleStaccato() {
	a.Lock("SetStyleStaccato")
	a.style = -1
	a.Unlock("SetStyleStaccato")
	a.calcNoteLen()
}

func (a *Arp) SetStyleNonLegato() {
	a.Lock("SetStyleNonLegato")
	a.style = 0
	a.Unlock("SetStyleNonLegato")
	a.calcNoteLen()
}

func (a *Arp) SetStyleLegato() {
	a.Lock("SetStyleLegato")
	a.style = 1
	a.Unlock("SetStyleLegato")
	a.calcNoteLen()
}

func (a *Arp) SetSwing(percent float32) {
	// TODO implement
	a.Lock("SetSwing")
	a.swing = percent
	a.Unlock("SetSwing")
}

func (a *Arp) SetNoteDistance(dist float64) {
	a.Lock("SetNoteDistance")
	a.noteDistance = dist
	a.Unlock("SetNoteDistance")
	a.calcNoteDistance()
	a.calcNoteLen()
}

func (a *Arp) WriteNoteOn(key, velocity uint8) error {
	//fmt.Printf("before write noteon %v\n", key)
	a.Lock("WriteNoteOn")
	a.lastNote = key
	running := a.runningNote
	if running > -1 {
		writer.NoteOff(a.wr, uint8(running))
	}
	a.runningNote = int8(key)
	err := writer.NoteOn(a.wr, key, velocity)
	a.Unlock("WriteNoteOn")
	//fmt.Printf("after write noteon %v\n", key)
	return err
}

func (a *Arp) WriteNoteOff(key uint8) error {
	a.Lock("WriteNoteOff")
	//fmt.Printf("before write noteoff %v\n", key)
	a.runningNote = -1
	err := writer.NoteOff(a.wr, key)
	//fmt.Printf("after write noteoff %v\n", key)
	a.Unlock("WriteNoteOff")
	return err
}

func (a *Arp) Silence() (did bool) {
	//fmt.Println("NoteOffRunning requested")
	a.Lock("Silence")
	a.wr.Silence(int8(a.channelOut), false)
	a.Unlock("Silence")
	return true
}

func (a *Arp) WriteMsg(msg midi.Message) error {
	err := a.wr.Write(msg)
	return err
}

// calcNoteDistance calculates the time until the next note will start and sends it to the noteDist channel
func (a *Arp) calcNoteDistance() {
	a.noteDist <- a.__calcNoteDistance()
}

func (a *Arp) __calcNoteDistance() time.Duration {
	return time.Duration(int(math.Round(a._calcNoteDistance()))) * time.Microsecond
}

// _calcNoteDistance calculates the note distance in microseconds
func (a *Arp) _calcNoteDistance() float64 {
	/*
		tempoBPM * qn =  60 sec
		durQn = 60000000 microsec / tempoBPM
		dist = n * dur qn
	*/
	a.RLock()
	factor := a.noteDistance
	bpm := a.tempoBPM
	a.RUnlock()
	return factor * float64(60000000) / bpm
}

// calcNoteLen calculates the current length of a note, based on the time distance and playing style and sends it to the noteLen channel
func (a *Arp) calcNoteLen() {
	a.noteLen <- a._calcNoteLen()
}

func (a *Arp) _calcNoteLen() time.Duration {

	dist := a._calcNoteDistance()
	a.RLock()
	style := a.style
	a.RUnlock()
	var l float64

	switch style {
	case -1: // staccato
		l = dist * 1.0 / 5.0
	case 0: // non-legato
		l = dist * 2.0 / 3.0
	case 1: // legato
		l = dist - 10
	default:
		panic("unreachable")
	}

	return time.Duration(int(math.Round(l))) * time.Microsecond
}

/*
type playnote struct {
	key      uint8
	velocity uint8
	dur      time.Duration
	wait     time.Duration
}
*/

func (a *Arp) scheduleNoteOff(k uint8, l time.Duration) {
	//	fmt.Printf("scheduling note off %v\n", k)
	time.Sleep(l)
	a.WriteNoteOff(k)
	//	fmt.Printf("note off %v written\n", k)
}

func (a *Arp) play() {
	var t *time.Timer
	var nt [2]uint8
	var mx sync.RWMutex

	noteDist := a.__calcNoteDistance()
	noteLen := a._calcNoteLen()

	stopTimer := func() {
		mx.Lock()
		if t != nil {
			//fmt.Println("try stopping timer")
			t.Stop()
			//fmt.Println("stopped timer")
		} else {
			//fmt.Println("timer was nil")
		}
		mx.Unlock()
	}

	go func() {
	loop:
		for {
			select {
			case noteDist = <-a.noteDist:
			case noteLen = <-a.noteLen:
			case <-a.finishScheduler:
				//fmt.Println("finishing")
				a.Silence()
				break loop
			case <-a.nextArpNote:
				//fmt.Println("nextArpNote requested")
				stopTimer()
				//fmt.Printf("scheduling nextArpNote after: %s\n", noteDist.String())
				mx.Lock()
				//fmt.Println("setting new timer")
				t = time.AfterFunc(noteDist, func() {
					//fmt.Println("running timer")
					key, velocity := a.calcNextNote()
					a.WriteNoteOn(key, velocity)
					//fmt.Printf("nextArpNote played %v\n", key)
					go func() {
						//fmt.Println("requesting nextArpNote")
						a.nextArpNote <- true
					}()
					go a.scheduleNoteOff(key, noteLen)

				})
				mx.Unlock()
				//fmt.Println("new timer was set")
			case nt = <-a.start:
				stopTimer()
				a.WriteNoteOn(nt[0], nt[1])
				//fmt.Printf("start note played %v\n", nt[0])
				go func() {
					//fmt.Println("requesting nextArpNote after start")
					a.nextArpNote <- true
				}()
				go a.scheduleNoteOff(nt[0], noteLen)
			case <-a.stop:
				//fmt.Println("stop called")
				stopTimer()
				a.stopped <- true
			default:
				//fmt.Printf("sleeping %v\n", noteDist)
			}
		}

		//fmt.Println("send finished")
		a.finishedScheduler <- true
	}()

}

func (a *Arp) Lock(by string) {
	//fmt.Println("locking by " + by)
	a.RWMutex.Lock()
}

func (a *Arp) RLock() {
	//fmt.Println("rlocking")
	a.RWMutex.RLock()
}

func (a *Arp) Unlock(by string) {
	//fmt.Println("unlocking by " + by)
	a.RWMutex.Unlock()
}

func (a *Arp) RUnLock() {
	//fmt.Println("runlocking")
	a.RWMutex.RUnlock()
}

func (a *Arp) handleMessage(msg midi.Message) {
	if chMsg, isCh := msg.(channel.Message); isCh {
		if a.channelIn >= 0 && uint8(a.channelIn) != chMsg.Channel() {
			a.WriteMsg(msg) // pass through
			return
		}

		if dist, ok := a.noteDistanceHandler(msg); ok {
			if dist == 0.0 {
				dist = 1.0
			}

			if dist >= 0 {
				// TODO maybe that is better served by special note to distance mapping in fixed steps
				// e.g. 1/4, 1/8, 1/16, tripplets etc. could also be mapped to program changes (but they could also be interesting for the instruments behind)
				a.SetNoteDistance(dist)
				//fmt.Printf("setting note distance to %v\n", dist)
			}
			return
		}

		if val, ok := a.directionSwitchHandler(msg); ok {
			a.SwitchDirection(val)
			//fmt.Printf("SwitchDirection\n")
			return
		}

		if val, ok := a.styleHandler(msg); ok {
			switch {
			case val < 40:
				a.SetStyleStaccato()
				//fmt.Printf("setting style\n")
			case val > 80:
				a.SetStyleLegato()
				//fmt.Printf("setting style\n")
			case val > 0:
				a.SetStyleNonLegato()
				//fmt.Printf("setting style\n")
			default:

			}
			return
		}

		switch v := msg.(type) {
		case channel.NoteOn:
			if v.Key()/12 == a.notePoolOctave {
				if v.Velocity() > 0 {
					a.AddNote(v.Key()%12, v.Velocity())
				} else {
					a.RemoveNote(v.Key() % 12)
				}
			} else {
				if v.Velocity() > 0 {
					a.StartWithNote(v.Key(), v.Velocity())
				} else {
					a.StopWithNote(v.Key())
				}
			}
		case channel.NoteOff:
			if v.Key()/12 == a.notePoolOctave {
				a.RemoveNote(v.Key() % 12)
			} else {
				a.StopWithNote(v.Key())
			}
		case channel.NoteOffVelocity:
			if v.Key()/12 == a.notePoolOctave {
				a.RemoveNote(v.Key() % 12)
			} else {
				a.StopWithNote(v.Key())
			}
		case channel.ControlChange:
			/*
				switch v.Controller() {
				case cc.GeneralPurposeSlider3: // swing
					a.SetSwing(float32(v.Value()) / float32(127.0))
				default:
					writer.ControlChange(a.wr, v.Controller(), v.Value())
				}
			*/
			writer.ControlChange(a.wr, v.Controller(), v.Value())

		case channel.PolyAftertouch:
			if v.Key()/12 == a.notePoolOctave {
				a.SetNoteVelocity(v.Key()%12, v.Pressure())
			} else {
				a.SetStartNoteVelocity(v.Pressure())
			}
		case channel.Aftertouch:
			writer.Aftertouch(a.wr, v.Pressure())
		case channel.ProgramChange:
			writer.ProgramChange(a.wr, v.Program())
		case channel.Pitchbend:
			writer.Pitchbend(a.wr, v.Value())
		default:
			panic("unreachable")
		}

		return
	}

	switch msg {
	case realtime.TimingClock:
		// TODO calculate the tempo from the clock
		a.WriteMsg(msg)
	case realtime.Tick:
		// TODO calculate the tempo from the clock
		a.WriteMsg(msg)
	default:
		a.WriteMsg(msg)
	}

	return
}

func (a *Arp) _transpose(msg midi.Message, transp int8) midi.Message {
	switch v := msg.(type) {
	case channel.NoteOn:
		if a.channelIn >= 0 && uint8(a.channelIn) != v.Channel() {
			return msg // pass through
		}

		_key := int8(v.Key()) + transp
		if _key < 0 {
			_key = 0
		}

		if _key > 127 {
			_key = 127
		}

		return channel.Channel(v.Channel()).NoteOn(uint8(_key), v.Velocity())

	case channel.NoteOff:
		if a.channelIn >= 0 && uint8(a.channelIn) != v.Channel() {
			return msg // pass through
		}

		_key := int8(v.Key()) + transp
		if _key < 0 {
			_key = 0
		}

		if _key > 127 {
			_key = 127
		}

		return channel.Channel(v.Channel()).NoteOff(uint8(_key))

	case channel.NoteOffVelocity:
		if a.channelIn >= 0 && uint8(a.channelIn) != v.Channel() {
			return msg // pass through
		}

		_key := int8(v.Key()) + transp
		if _key < 0 {
			_key = 0
		}

		if _key > 127 {
			_key = 127
		}

		return channel.Channel(v.Channel()).NoteOff(uint8(_key))

	default:
		return msg
	}
}

func (a *Arp) Run() error {
	if !a.in.IsOpen() {
		return fmt.Errorf("midi in port no %v (%s) is not opened, please open before calling arp.Run", a.in.Number(), a.in.String())
	}

	if !a.out.IsOpen() {
		return fmt.Errorf("midi out port no %v (%s) is not opened, please open before calling arp.Run", a.out.Number(), a.out.String())
	}

	a.Lock("Run")
	a.wr = writer.New(a.out)
	//a.wr.ConsolidateNotes(false)
	a.wr.SetChannel(a.channelOut) // set default writing channel

	//var wg sync.WaitGroup

	transp := a.transpose

	go func() {
	loop:
		for {
			select {
			case <-a.finishListener:
				break loop
			case msg := <-a.messages:
				//fmt.Printf("got message\n")
				if transp == 0 {
					a.handleMessage(msg)
				} else {
					a.handleMessage(a._transpose(msg, transp))
				}
			default:
			}
		}
		a.finishedListener <- true
	}()

	rd := reader.New(
		reader.NoLogger(),
		reader.Each(func(p *reader.Position, msg midi.Message) {
			a.messages <- msg
		}),
	)
	a.Unlock("Run")
	a.play()
	//time.Sleep(20 * time.Millisecond)
	//a.calcNoteDistance()
	//a.calcNoteLen()
	go rd.ListenTo(a.in)
	return nil
}

func (a *Arp) Stop() {
	a.RLock()
	running := a.isRunning
	a.RUnlock()
	if !running {
		//fmt.Println("not running any more")
		return
	}
	a.stop <- true
	_ = <-a.stopped
	a.Lock("Stop")
	a.isRunning = false
	a.Unlock("Stop")
}

func (a *Arp) Close() error {
	//fmt.Println("stop listening")
	err := a.in.StopListening()
	//fmt.Println("done: stop listening")
	//fmt.Println("stop arp")
	a.Stop()
	//fmt.Println("done: stop arp")
	//fmt.Println("request finish listener")
	a.finishListener <- true
	<-a.finishedListener
	//fmt.Println("got finished listener")
	//fmt.Println("request finish scheduler")
	a.finishScheduler <- true
	<-a.finishedScheduler
	//fmt.Println("got finished scheduler")
	//time.Sleep(20 * time.Millisecond)
	a.wr.Silence(-1, true)
	return err
}

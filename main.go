package main

import (
	"fmt"
	"os"
	"os/signal"

	driver "gitlab.com/gomidi/rtmididrv"

	hyperarp "gitlab.com/gomidi/hyperarp/lib"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/cc"
	config "gitlab.com/metakeule/config"
)

var CONFIG = config.MustNew("hyperarp", VERSION, "hyper arpeggiator")

var (
	inArg                = CONFIG.NewInt32("in", "number of the input MIDI port (use hyperarp list to see the available MIDI ports)", config.Required, config.Shortflag('i'))
	outArg               = CONFIG.NewInt32("out", "number of the output MIDI port (use hyperarp list to see the available MIDI ports)", config.Required, config.Shortflag('o'))
	transposeArg         = CONFIG.NewInt32("transpose", "transpose (number of semitones)", config.Default(int32(0)), config.Shortflag('t'))
	tempoArg             = CONFIG.NewFloat32("tempo", "tempo (BPM)", config.Default(float32(120.0)), config.Shortflag('b'))
	ccDirectionSwitchArg = CONFIG.NewInt32("ccdir", "controller number for the direction switch", config.Default(int32(cc.GeneralPurposeButton1Switch)))
	ccTimeIntervalArg    = CONFIG.NewInt32("cctiming", "controller number to set the timing interval", config.Default(int32(cc.GeneralPurposeSlider1)))
	ccStyleArg           = CONFIG.NewInt32("ccstyle", "controller number to select the playing style (staccato, non-legato, legato)", config.Default(int32(cc.GeneralPurposeSlider2)))

	noteDirectionSwitchArg = CONFIG.NewInt32("notedir", "note (key) for the direction switch")
	noteTimeIntervalArg    = CONFIG.NewInt32("notetiming", "note (key) for the timing interval")
	noteStyleArg           = CONFIG.NewInt32("notestyle", "note (key) for the playing style (staccato, non-legato, legato)")

	controlChannelArg = CONFIG.NewInt32("ctrlch", "channel for control messages (only needed if not the same as the input channel")

	listCmd = CONFIG.MustCommand("list", "show the available MIDI ports").Skip("in").Skip("out").Skip("transpose").Skip("tempo").Skip("ccdir").Skip("cctiming").Skip("ccstyle").Skip("notedir").Skip("notetiming").Skip("notestyle").Skip("ctrlch")
)

func main() {
	err := run()
	if err != nil {

		fmt.Fprintf(os.Stderr, "ERROR: %s\n\n", err.Error())
		os.Exit(1)
		return
	}
	os.Exit(0)
}

func run() error {
	drv, err := driver.New()

	if err != nil {
		return err
	}

	defer drv.Close()

	err = CONFIG.Run()

	if err != nil {
		fmt.Fprint(os.Stderr, CONFIG.Usage())
		listMIDIDevices(drv)
		return err
	}

	if CONFIG.ActiveCommand() == listCmd {
		listMIDIDevices(drv)
		return nil
	}

	// make sure to close all open ports at the end
	defer drv.Close()

	var inPort midi.In = nil
	in := inArg.Get()

	inPort, err = midi.OpenIn(drv, int(in), "")
	if err != nil {
		return err
	}

	var outPort midi.Out = nil
	out := outArg.Get()
	outPort, err = midi.OpenOut(drv, int(out), "")
	if err != nil {
		return err
	}

	defer inPort.Close()
	defer outPort.Close()

	opts := []hyperarp.Option{
		hyperarp.CCDirectionSwitch(uint8(ccDirectionSwitchArg.Get())),
		hyperarp.CCTimeInterval(uint8(ccTimeIntervalArg.Get())),
		hyperarp.CCStyle(uint8(ccStyleArg.Get())),
		hyperarp.Tempo(float64(tempoArg.Get())),
	}

	if noteDirectionSwitchArg.IsSet() {
		opts = append(opts, hyperarp.NoteDirectionSwitch(uint8(noteDirectionSwitchArg.Get())))
	}

	if noteTimeIntervalArg.IsSet() {
		opts = append(opts, hyperarp.NoteTimeInterval(uint8(noteTimeIntervalArg.Get())))
	}

	if noteStyleArg.IsSet() {
		opts = append(opts, hyperarp.NoteStyle(uint8(noteStyleArg.Get())))
	}

	if controlChannelArg.IsSet() {
		opts = append(opts, hyperarp.ControlChannel(uint8(controlChannelArg.Get())))
	}

	tr := int8(transposeArg.Get())

	if tr != 0 {
		opts = append(opts, hyperarp.Transpose(tr))
	}

	arp := hyperarp.New(inPort, outPort, opts...)

	go arp.Run()
	defer arp.Close()

	sigchan := make(chan os.Signal, 10)

	// listen for ctrl+c
	go signal.Notify(sigchan, os.Interrupt)

	// interrupt has happend
	<-sigchan
	fmt.Println("\n--interrupted!")
	return nil
}

func listMIDIDevices(d midi.Driver) {
	ins, _ := d.Ins()

	fmt.Print("\n--- MIDI input ports ---\n\n")

	for _, port := range ins {
		fmt.Printf("[%d] %#v\n", port.Number(), port.String())
	}

	outs, _ := d.Outs()

	fmt.Print("\n--- MIDI output ports ---\n\n")

	for _, port := range outs {
		fmt.Printf("[%d] %#v\n", port.Number(), port.String())
	}

	fmt.Println("\n\n")

	return
}

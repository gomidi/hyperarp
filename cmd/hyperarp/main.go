package main

import (
	"fmt"
	"os"
	"os/signal"

	driver "gitlab.com/gomidi/rtmididrv"

	"gitlab.com/gomidi/hyperarp"
	"gitlab.com/gomidi/midi"
	config "gitlab.com/metakeule/config"
)

var CONFIG = config.MustNew("hyperarp", hyperarp.VERSION, "hyper arpeggiator")

var (
	inArg        = CONFIG.NewInt32("in", "number of the input device", config.Required, config.Shortflag('i'))
	outArg       = CONFIG.NewInt32("out", "number of the output device", config.Required, config.Shortflag('o'))
	transposeArg = CONFIG.NewInt32("transpose", "transpose (half notes)", config.Default(int32(0)), config.Shortflag('t'))
	listCmd      = CONFIG.MustCommand("list", "list devices").Relax("in").Relax("out")
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

	opts := []hyperarp.Option{}

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

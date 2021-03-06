package hyperarp_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"gitlab.com/gomidi/hyperarp"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/cc"
	"gitlab.com/gomidi/midi/reader"
	"gitlab.com/gomidi/midi/testdrv"
	"gitlab.com/gomidi/midi/writer"
)

type cable struct {
	midi.Driver
	in  midi.In
	out midi.Out
}

func newCable(name string) *cable {
	var c cable
	c.Driver = testdrv.New("fake cable: " + name)
	ins, _ := c.Driver.Ins()
	outs, _ := c.Driver.Outs()
	c.in, c.out = ins[0], outs[0]
	c.in.Open()
	c.out.Open()
	return &c
}

type arpTester struct {
	arp *hyperarp.Arp
	rd  *reader.Reader
	*writer.Writer
	bf       bytes.Buffer
	cable1   *cable
	cable2   *cable
	lastTime time.Time
}

func newArpTester() *arpTester {
	var at arpTester
	at.cable1 = newCable("write to arp")
	at.cable2 = newCable("read from arp")
	at.lastTime = time.Unix(0, 0)
	at.rd = reader.New(
		reader.NoLogger(),
		reader.Each(func(p *reader.Position, msg midi.Message) {
			now := time.Now()
			if at.lastTime.Unix() == 0 {
				at.bf.WriteString(msg.String() + "\n")
			} else {
				dur := now.Sub(at.lastTime)
				at.bf.WriteString(fmt.Sprintf("[%v] %s\n", dur.Milliseconds(), msg.String()))
			}
			at.lastTime = time.Now()
		}),
	)

	at.arp = hyperarp.New(at.cable1.in, at.cable2.out)
	at.Writer = writer.New(at.cable1.out)
	return &at
}

func (at *arpTester) Run() {
	go at.rd.ListenTo(at.cable2.in)
	go at.arp.Run()
}

func (at *arpTester) Close() {
	at.arp.Close()
	at.cable1.Close()
	at.cable2.Close()
}

func (at *arpTester) Result() string {
	return at.bf.String()
}

// This example reads from the first input and and writes to the first output port
func TestFirst(t *testing.T) {
	var a *arpTester

	var tests = []struct {
		fn       func()
		descr    string
		expected string
	}{
		{
			func() {
				writer.NoteOn(a, 70, 100)
				time.Sleep(200 * time.Millisecond)
				writer.NoteOff(a, 70)
			},
			"note 70",
			"channel.NoteOn channel 0 key 70 velocity 100\n[167] channel.NoteOff channel 0 key 70\n",
		},
		{
			func() {
				writer.Pitchbend(a, 1000)
				time.Sleep(10 * time.Millisecond)
			},
			"pitchbend passthrough",
			"channel.Pitchbend channel 0 value 1000 absValue 9192\n",
		},
		{
			func() {
				writer.Pitchbend(a, 100)
				writer.Aftertouch(a, 100)
				time.Sleep(10 * time.Millisecond)
			},
			"pitchbend and aftertouch passthrough",
			"channel.Pitchbend channel 0 value 100 absValue 8292\n[0] channel.Aftertouch channel 0 pressure 100\n",
		},
		{
			func() {
				writer.ControlChange(a, cc.GeneralPurposeSlider1, 3)
				writer.NoteOn(a, hyperarp.D, 100)
				writer.NoteOn(a, uint8(12+hyperarp.E), 120)
				time.Sleep(500 * time.Millisecond)
				writer.NoteOff(a, hyperarp.D)
				writer.NoteOff(a, uint8(12+hyperarp.E))
				time.Sleep(10 * time.Millisecond)
			},
			"2 arp notes upward",
			`channel.NoteOn channel 0 key 16 velocity 120
[83] channel.NoteOff channel 0 key 16
[41] channel.NoteOn channel 0 key 26 velocity 100
[83] channel.NoteOff channel 0 key 26
[41] channel.NoteOn channel 0 key 28 velocity 120
[83] channel.NoteOff channel 0 key 28
[41] channel.NoteOn channel 0 key 38 velocity 100
[83] channel.NoteOff channel 0 key 38
`,
		},
		{
			func() {
				writer.ControlChange(a, cc.GeneralPurposeSlider1, 3)
				writer.NoteOn(a, hyperarp.D, 100)
				writer.NoteOn(a, hyperarp.G, 80)
				writer.NoteOn(a, uint8(12+hyperarp.E), 120)
				time.Sleep(530 * time.Millisecond)
				writer.NoteOff(a, hyperarp.D)
				writer.NoteOff(a, hyperarp.G)
				writer.NoteOff(a, uint8(12+hyperarp.E))
			},
			"3 arp notes upward",
			`channel.NoteOn channel 0 key 16 velocity 120
[83] channel.NoteOff channel 0 key 16
[41] channel.NoteOn channel 0 key 19 velocity 80
[83] channel.NoteOff channel 0 key 19
[41] channel.NoteOn channel 0 key 26 velocity 100
[83] channel.NoteOff channel 0 key 26
[41] channel.NoteOn channel 0 key 28 velocity 120
[83] channel.NoteOff channel 0 key 28
`,
		},
		{
			func() {
				writer.ControlChange(a, cc.GeneralPurposeSlider1, 3)
				writer.CcOn(a, cc.GeneralPurposeButton1Switch)
				time.Sleep(time.Microsecond)
				writer.NoteOn(a, hyperarp.D, 100)
				writer.NoteOn(a, uint8(12+hyperarp.E), 120)
				time.Sleep(500 * time.Millisecond)
				writer.NoteOff(a, hyperarp.D)
				writer.NoteOff(a, uint8(12+hyperarp.E))
				//time.Sleep(time.Microsecond)
				writer.CcOff(a, cc.GeneralPurposeButton1Switch)
				time.Sleep(10 * time.Millisecond)
			},
			"2 arp notes downward",
			`channel.NoteOn channel 0 key 16 velocity 120
[83] channel.NoteOff channel 0 key 16
[41] channel.NoteOn channel 0 key 14 velocity 100
[83] channel.NoteOff channel 0 key 14
[41] channel.NoteOn channel 0 key 4 velocity 120
[83] channel.NoteOff channel 0 key 4
[41] channel.NoteOn channel 0 key 16 velocity 100
[83] channel.NoteOff channel 0 key 16
`,
		},
	}

	for i, test := range tests {
		/*
			if i != 0 {
				continue
			}
		*/

		//fmt.Printf("running test [%v]\n", i)
		a = newArpTester()
		a.Run()

		//fmt.Println("let it start (400ms)")
		time.Sleep(100 * time.Millisecond)

		test.fn()

		//fmt.Println("waiting for result (2400ms)")
		//fmt.Printf("after test %v\n", i)
		//time.Sleep(2400 * time.Millisecond)
		//time.Sleep(1000 * time.Millisecond)

		got := a.Result()
		//fmt.Printf("got result %q\n", got)

		//fmt.Println("before close")
		a.Close()
		//fmt.Println("after close")

		if got != test.expected {
			t.Errorf("[%v] %q\ngot:\n%s\n\nexpected:\n%s", i, test.descr, got, test.expected)
		}

		time.Sleep(100 * time.Millisecond)
	}

}

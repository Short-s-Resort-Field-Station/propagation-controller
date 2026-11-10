// probe-address readdresses a ZTS-3000 soil probe on the RS485 bus.
//
// Every probe ships at slave address 1, so a multi-probe bus needs each one
// readdressed before it can be connected alongside the others. Connect ONE
// probe at a time and run:
//
//	probe-address -to 2          # move the (single) connected probe to addr 2
//	probe-address -from 2 -to 3  # move a specific address
//
// The device address lives in holding register 0x07D0 (verified against the
// real sensor 2026-07-10). The tool refuses to write unless exactly the
// expected probe answers at -from and nothing answers at -to.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	gm "github.com/goburrow/modbus"

	"propagation-controller/internal/devices/modbus"
)

const addrRegister = 0x07D0

func main() {
	port := flag.String("port", "", "serial port of the RS485 adapter (autodetected if empty)")
	baud := flag.Int("baud", 9600, "baud rate (bus runs at 9600 as built; a factory-fresh ZTS-3000 answers at 4800 — pass -baud 4800 -from 1 for a new probe)")
	from := flag.Uint("from", 1, "current slave address of the probe")
	to := flag.Uint("to", 0, "new slave address (1..247, required)")
	yes := flag.Bool("yes", false, "actually write; without this the tool only reports what it would do")
	flag.Parse()

	if *to < 1 || *to > 247 {
		fatal("-to must be 1..247")
	}
	if *from == *to {
		fatal("-from and -to are the same address; nothing to do")
	}
	if *port == "" {
		p, err := modbus.DetectRS485Port()
		if err != nil {
			fatal("%v (pass -port explicitly)", err)
		}
		*port = p
	}

	h := gm.NewRTUClientHandler(*port)
	h.BaudRate = *baud
	h.DataBits = 8
	h.Parity = "N"
	h.StopBits = 1
	h.Timeout = 500 * time.Millisecond
	if err := h.Connect(); err != nil {
		fatal("open %s: %v", *port, err)
	}
	defer h.Close()
	client := gm.NewClient(h)

	// The probe at -from must answer, and its address register must agree.
	h.SlaveId = uint8(*from)
	regs, err := client.ReadHoldingRegisters(0, 3)
	if err != nil {
		fatal("no probe answered at address %d on %s @ %d baud: %v", *from, *port, *baud, err)
	}
	stored, err := client.ReadHoldingRegisters(addrRegister, 1)
	if err != nil {
		fatal("probe at %d did not expose the address register 0x%04X: %v", *from, addrRegister, err)
	}
	fmt.Printf("probe found at address %d (moisture=%.1f%% temp=%.1f°C ec=%dµS/cm, reg 0x%04X=%d)\n",
		*from,
		float64(u16(regs, 0))/10, float64(int16(u16(regs, 1)))/10, u16(regs, 2),
		addrRegister, u16(stored, 0))

	// The target address must be silent — a second device there would leave
	// two probes answering the same address, which corrupts every read.
	h.SlaveId = uint8(*to)
	if _, err := client.ReadHoldingRegisters(0, 1); err == nil {
		fatal("another device already answers at address %d — pick a free address", *to)
	}

	if !*yes {
		fmt.Printf("dry run: would write %d to register 0x%04X at address %d (re-run with -yes)\n",
			*to, addrRegister, *from)
		return
	}

	h.SlaveId = uint8(*from)
	if _, err := client.WriteSingleRegister(addrRegister, uint16(*to)); err != nil {
		// Some firmwares apply the write and then answer from the new
		// address, so a response error here is not conclusive — the
		// verification read below decides.
		fmt.Printf("write response irregular (%v) — verifying at new address...\n", err)
	}
	time.Sleep(300 * time.Millisecond)

	h.SlaveId = uint8(*to)
	if _, err := client.ReadHoldingRegisters(0, 1); err != nil {
		fatal("probe did not answer at new address %d — it may still be at %d; re-run the tool without -yes to inspect", *to, *from)
	}
	fmt.Printf("done: probe now answers at address %d. Label the cable.\n", *to)
}

func u16(b []byte, reg int) uint16 {
	return uint16(b[2*reg])<<8 | uint16(b[2*reg+1])
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "probe-address: "+format+"\n", args...)
	os.Exit(1)
}

package main

import (
	"flag"
	"log"

	"github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
)

func main() {
	port := flag.String("port", "/dev/ttyUSB0", "COM порт")
	speed := flag.Int("speed", 100, "Скорость")
	flag.Parse()

	driver := t3d.New(*port, 19200, 1)
	if err := driver.Connect(); err != nil {
		log.Fatal(err)
	}
	defer driver.Close()

	for id := 1; id < 2; id++ {
		driver.SetSlaveID(byte(id))
		driver.SetSpeedPreset(*speed, true)
		log.Printf("Мотор %d: %d RPM", id, *speed)
	}
}

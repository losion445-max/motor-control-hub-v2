package main

import (
	"flag"
	"log"
	"time"

	"github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
)

func main() {
	port := flag.String("port", "/dev/ttyUSB0", "COM порт")
	baud := flag.Int("baud", 19200, "Скорость")
	speed := flag.Int("speed", 100, "Скорость (0 или 100)")
	flag.Parse()

	if *speed != 0 && *speed != 100 {
		log.Fatal("Скорость должна быть 0 или 100")
	}

	driver := t3d.New(*port, *baud, 1)
	if err := driver.Connect(); err != nil {
		log.Fatalf("Ошибка подключения: %v", err)
	}
	defer driver.Close()

	start := time.Now()

	for id := 1; id <= 4; id++ {
		driver.SetSlaveID(byte(id))

		// Прямой вызов без SetSpeedPreset для минимальной задержки
		if *speed == 0 {
			if err := driver.ServoDisable(); err != nil {
				log.Printf("❌ Мотор %d: ошибка выключения: %v", id, err)
				continue
			}
			log.Printf("✅ Мотор %d: выключен", id)
		} else {
			// Устанавливаем скорость 100 и включаем
			if err := driver.SetSpeedPreset(100, true); err != nil {
				log.Printf("❌ Мотор %d: ошибка: %v", id, err)
				continue
			}
			log.Printf("✅ Мотор %d: скорость 100 RPM", id)
		}
	}

	log.Printf("⏱️ Все операции заняли: %v", time.Since(start))
}

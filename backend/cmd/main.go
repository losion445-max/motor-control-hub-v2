// CLI для управления драйвером серводвигателя HLTNC T3D через Modbus RTU / RS-485.
//
// Использование:
//   go run cmd/main.go                            — сканировать slave 1..32
//   go run cmd/main.go -slave 1                   — опросить slave 1
//   go run cmd/main.go -slave 1 -read-status      — статус мотора (FC04)
//   go run cmd/main.go -slave 1 -read-params      — параметры (FC03)
//   go run cmd/main.go -slave 1 -write-param -addr 4 -val 1   — записать P-004=1
//   go run cmd/main.go -slave 1 -save-eeprom      — сохранить в EEPROM (FC41)
//   go run cmd/main.go -slave 1 -servo-enable     — включить servo (FC42)
//   go run cmd/main.go -slave 1 -servo-disable    — выключить servo (FC42)
//   go run cmd/main.go -slave 1 -read-one -addr 4 — прочитать один FC03 регистр
//   go run cmd/main.go -slave 1 -scan-raw  -from-addr 0 -to-addr 50  — FC03 диапазон
//   go run cmd/main.go -slave 1 -scan-status -from-addr 0 -to-addr 40 — FC04 диапазон

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
)

// regInfo описывает регистр для вывода в таблице.
type regInfo struct {
	addr uint16
	name string
	desc string
}

// Параметры для вывода при -read-params (FC03).
var paramList = []regInfo{
	{t3d.ParamControlMode, "P-004", "Режим управления (0=позиция, 1=скорость, 2=момент)"},
	{t3d.ParamSpeedKp, "P-005", "Kp контура скорости (Гц)"},
	{t3d.ParamSpeedTi, "P-006", "Ti контура скорости (мс)"},
	{t3d.ParamTorqueFilter, "P-007", "Фильтр момента (×0.1 мс)"},
	{t3d.ParamPositionKp, "P-009", "Kp контура положения (1/с)"},
	{t3d.ParamSpeedSource, "P-025", "Источник команды скорости (0=аналог, 1=пресеты, 3=импульс)"},
	{t3d.ParamTorqueSource, "P-026", "Источник команды момента"},
	{t3d.ParamGearNumerH, "P-028", "Электронный редуктор: числитель старший"},
	{t3d.ParamGearNumerL, "P-029", "Электронный редуктор: числитель младший"},
	{t3d.ParamGearDenom, "P-030", "Электронный редуктор: знаменатель"},
	{t3d.ParamPulseMode, "P-035", "Режим импульса (0=pulse+dir, 1=CW/CCW, 2=AB)"},
	{t3d.ParamPulseDir, "P-036", "Направление импульса (0=норм, 1=реверс)"},
	{t3d.ParamAccelTime, "P-060", "Время разгона (мс/1000 об/мин)"},
	{t3d.ParamDecelTime, "P-061", "Время торможения (мс/1000 об/мин)"},
	{t3d.ParamMaxSpeed, "P-075", "Максимальная скорость (об/мин)"},
	{t3d.ParamJogSpeed, "P-076", "Скорость JOG (об/мин)"},
	{t3d.ParamServoOnMode, "P-098", "Принудительный servo-on (0=внешний SON, 1=всегда)"},
	{t3d.ParamDI1Func, "P-100", "Функция DI1"},
	{t3d.ParamDI2Func, "P-101", "Функция DI2"},
	{t3d.ParamDI3Func, "P-102", "Функция DI3"},
	{t3d.ParamDI4Func, "P-103", "Функция DI4"},
	{t3d.ParamDO1Func, "P-108", "Функция DO1"},
	{t3d.ParamDO2Func, "P-109", "Функция DO2"},
	{t3d.ParamDO3Func, "P-110", "Функция DO3"},
	{t3d.ParamDO4Func, "P-111", "Функция DO4"},
	{t3d.ParamInternalSpd1, "P-137", "Внутренняя скорость 1 (об/мин)"},
	{t3d.ParamInternalSpd2, "P-138", "Внутренняя скорость 2 (об/мин)"},
	{t3d.ParamInternalSpd3, "P-139", "Внутренняя скорость 3 (об/мин)"},
	{t3d.ParamInternalSpd4, "P-140", "Внутренняя скорость 4 (об/мин)"},
	{t3d.ParamInternalSpd5, "P-141", "Внутренняя скорость 5 (об/мин)"},
	{t3d.ParamInternalSpd6, "P-142", "Внутренняя скорость 6 (об/мин)"},
	{t3d.ParamInternalSpd7, "P-143", "Внутренняя скорость 7 (об/мин)"},
	{t3d.ParamInternalSpd8, "P-144", "Внутренняя скорость 8 (об/мин)"},
	{t3d.ParamEncoderLines, "P-172", "Линии энкодера (default=2500 → 10000 имп/об)"},
	{t3d.ParamEncoderBits, "P-184", "Разрядность энкодера (17 или 23 бит)"},
	{t3d.ParamSlaveID, "P-181", "Slave ID (1..32)"},
	{t3d.ParamBaudRate, "P-182", "Baud rate (0=4800..5=115200)"},
	{t3d.ParamDataFormat, "P-183", "Формат данных (0=8N1, 1=8E1, 2=8O1...)"},
	{t3d.ParamPolePairs, "P-201", "Число пар полюсов"},
	{t3d.ParamRatedCurrent, "P-204", "Номинальный ток (А)"},
	{t3d.ParamRatedSpeed, "P-207", "Номинальная скорость (об/мин)"},
}

func main() {
	port := flag.String("port", t3d.DefaultPort, "RS-485 порт")
	baud := flag.Int("baud", t3d.DefaultBaud, "Baud rate")
	slaveID := flag.Int("slave", 0, "Slave ID для опроса (0 = сканировать все 1..32)")
	fromSlave := flag.Int("from", 1, "Начало диапазона сканирования")
	toSlave := flag.Int("to", 32, "Конец диапазона сканирования")
	readStatus := flag.Bool("read-status", false, "Читать статусные регистры FC04")
	readParams := flag.Bool("read-params", false, "Читать параметры FC03")
	writeParam := flag.Bool("write-param", false, "Записать параметр FC06 (-addr, -val)")
	writeMulti := flag.Bool("write-multi", false, "Записать два регистра FC10 (-addr, -val-hi, -val-lo)")
	saveEEPROM := flag.Bool("save-eeprom", false, "FC41: сохранить параметры в EEPROM")
	servoEnable := flag.Bool("servo-enable", false, "FC42: включить servo")
	servoDisable := flag.Bool("servo-disable", false, "FC42: выключить servo")
	readSingle := flag.Bool("read-one", false, "Прочитать один FC03 регистр (-addr)")
	scanRaw := flag.Bool("scan-raw", false, "Сканировать FC03 адреса (-from-addr до -to-addr)")
	scanStatus := flag.Bool("scan-status", false, "Сканировать FC04 адреса (-from-addr до -to-addr)")
	fromAddr := flag.Int("from-addr", 0, "Начальный адрес для сканирования")
	toAddr := flag.Int("to-addr", 20, "Конечный адрес для сканирования")
	writeAddr := flag.Int("addr", 0, "Адрес регистра")
	writeVal := flag.Int("val", 0, "Значение для FC06")
	writeValHi := flag.Int("val-hi", 0, "Старший регистр для FC10")
	writeValLo := flag.Int("val-lo", 0, "Младший регистр для FC10")
	flag.Parse()

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  HLTNC T3D — Modbus RTU RS-485")
	fmt.Println("  Порт:", *port, "| Baud:", *baud, "| 8E1")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	initID := byte(1)
	if *slaveID != 0 {
		initID = byte(*slaveID)
	}

	drv := t3d.New(*port, *baud, initID)
	if err := drv.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		fmt.Fprintln(os.Stderr, "\nПроверьте:")
		fmt.Fprintln(os.Stderr, "  ls /dev/ttyUSB*          — виден ли адаптер?")
		fmt.Fprintln(os.Stderr, "  sudo usermod -aG dialout $USER  — права на порт?")
		os.Exit(1)
	}
	defer drv.Close()
	fmt.Printf("✅ Порт %s открыт\n\n", *port)

	// ── Прочитать один FC03 регистр ──────────────────────────────────────
	if *readSingle {
		requireSlave(*slaveID)
		v, err := drv.ReadParam(uint16(*writeAddr))
		dieOn(err)
		fmt.Printf("FC03 addr=%d → %d (0x%04X)\n", *writeAddr, v, v)
		return
	}

	// ── FC41 EEPROM ───────────────────────────────────────────────────────
	if *saveEEPROM {
		requireSlave(*slaveID)
		fmt.Printf("💾 FC41 сохранение в EEPROM (slave=%d)...\n", *slaveID)
		dieOn(drv.SaveEEPROM())
		fmt.Println("✅ Готово. Ждите 5 секунд перед отключением питания.")
		time.Sleep(5 * time.Second)
		return
	}

	// ── FC42 Servo Enable / Disable ───────────────────────────────────────
	if *servoEnable || *servoDisable {
		requireSlave(*slaveID)
		if *servoEnable {
			fmt.Printf("⚡ FC42 Servo ENABLE (slave=%d)\n", *slaveID)
			dieOn(drv.ServoEnable())
			fmt.Println("✅ Servo включён.")
		} else {
			fmt.Printf("⚡ FC42 Servo DISABLE (slave=%d)\n", *slaveID)
			dieOn(drv.ServoDisable())
			fmt.Println("✅ Servo выключен.")
		}
		return
	}

	// ── FC10 мультирегистровая запись ─────────────────────────────────────
	if *writeMulti {
		requireSlave(*slaveID)
		fmt.Printf("✏️  FC10 запись: addr=%d hi=%d lo=%d\n", *writeAddr, *writeValHi, *writeValLo)
		dieOn(drv.WriteParams(uint16(*writeAddr), []uint16{uint16(*writeValHi), uint16(*writeValLo)}))
		fmt.Println("✅ Записано.")
		return
	}

	// ── FC06 запись одного параметра ──────────────────────────────────────
	if *writeParam {
		requireSlave(*slaveID)
		fmt.Printf("✏️  FC06 запись: P-%03d = %d\n", *writeAddr, *writeVal)
		dieOn(drv.WriteParam(uint16(*writeAddr), uint16(*writeVal)))
		fmt.Println("✅ Записано. (Не забудьте -save-eeprom если нужно сохранить)")
		return
	}

	// ── Сканирование диапазона FC03 или FC04 ─────────────────────────────
	if *scanRaw || *scanStatus {
		requireSlave(*slaveID)
		fc := "FC03"
		if *scanStatus {
			fc = "FC04"
		}
		fmt.Printf("📊 Сканирование %s addr=%d..%d (slave=%d)\n\n", fc, *fromAddr, *toAddr, *slaveID)
		for a := uint16(*fromAddr); a <= uint16(*toAddr); a++ {
			var v uint16
			var err error
			if *scanStatus {
				v, err = drv.ReadInputReg(a)
			} else {
				v, err = drv.ReadParam(a)
			}
			if err != nil {
				fmt.Printf("  addr=%-4d  ERR: %v\n", a, err)
			} else {
				fmt.Printf("  addr=%-4d  val=%-6d  0x%04X\n", a, v, v)
			}
		}
		fmt.Println("\nГотово.")
		return
	}

	// ── Определить список slave для опроса ───────────────────────────────
	targets := []int{}
	if *slaveID != 0 {
		targets = []int{*slaveID}
	} else {
		targets = scanSlaves(drv, *fromSlave, *toSlave)
	}
	if len(targets) == 0 {
		fmt.Println("❌ Устройства не найдены.")
		fmt.Println("\nПроверьте:")
		fmt.Println("  P-181 ≠ -1  (по умолчанию -1 = коммуникация ВЫКЛЮЧЕНА)")
		fmt.Println("  P-182 = 2   (19200 bps)")
		fmt.Println("  P-183 = 1   (8E1)")
		return
	}

	// ── Опрос каждого slave ───────────────────────────────────────────────
	for _, id := range targets {
		fmt.Printf("\n━━━ SLAVE ID=%d ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n", id)
		drv.SetSlaveID(byte(id))

		doStatus := *readStatus || (!*readParams && !*readStatus)
		doParams := *readParams || (!*readParams && !*readStatus)

		if doStatus {
			printStatus(drv)
		}
		if doParams {
			printParams(drv)
		}
	}

	fmt.Println("\n✅ Готово.")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func scanSlaves(drv *t3d.Driver, from, to int) []int {
	fmt.Printf("🔍 Сканирование slave ID %d..%d...\n", from, to)
	var found []int
	for id := from; id <= to; id++ {
		drv.SetSlaveID(byte(id))
		_, err := drv.ReadInputReg(t3d.StatusSpeed)
		if err == nil {
			fmt.Printf("  ✅ Найден slave ID=%d\n", id)
			found = append(found, id)
		} else {
			fmt.Printf("  · ID=%d: нет ответа\n", id)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return found
}

func printStatus(drv *t3d.Driver) {
	s, err := drv.ReadStatus()
	if err != nil {
		fmt.Println("  [ERR] ReadStatus:", err)
		return
	}

	fault := fmt.Sprintf("%d", s.FaultCode)
	if s.FaultCode == 0 {
		fault = "0 (OK)"
	}

	fmt.Println("\n┌─ СТАТУС (FC04) ─────────────────────────────────────────")
	fmt.Printf("│  Скорость           : %8d об/мин\n", s.SpeedRPM)
	fmt.Printf("│  Скорость (точная)  : %8.1f об/мин\n", float64(s.SpeedPrecise10)/10)
	fmt.Printf("│  Позиция (32-bit)   : %8d имп\n", s.Position32)
	fmt.Printf("│  Момент             : %8d %% от ном.\n", s.TorquePct)
	fmt.Printf("│  Ток                : %8.1f А\n", float64(s.CurrentA10)/10)
	fmt.Printf("│  Задание скорости   : %8d об/мин\n", s.SpeedRefRPM)
	fmt.Printf("│  Задание момента    : %8d %%\n", s.TorqueRefPct)
	fmt.Printf("│  DI состояние       :   0b%08b  (бит0=DI1)\n", s.DIState)
	fmt.Printf("│  DO состояние       :     0b%06b  (бит0=DO1)\n", s.DOState)
	fmt.Printf("│  Код аварии         : %s\n", fault)
	fmt.Printf("│  Т° радиатора       : %8d °C\n", s.HeatsinkTempC)
	fmt.Printf("│  Т° модуля          : %8d °C\n", s.ModuleTempC)
	fmt.Printf("│  Напряжение шины DC : %8d В\n", s.BusVoltageV)
	fmt.Println("└─────────────────────────────────────────────────────────")
}

func printParams(drv *t3d.Driver) {
	fmt.Println("\n┌─ ПАРАМЕТРЫ (FC03) ──────────────────────────────────────")
	for _, r := range paramList {
		v, err := drv.ReadParam(r.addr)
		if err != nil {
			fmt.Printf("│  %-6s  addr=%-4d  ERR: %v\n", r.name, r.addr, err)
			continue
		}
		fmt.Printf("│  %-6s  addr=%-4d  val=%-6d (0x%04X)  — %s\n",
			r.name, r.addr, v, v, r.desc)
	}
	fmt.Println("└─────────────────────────────────────────────────────────")
}

func requireSlave(slaveID int) {
	if slaveID == 0 {
		fmt.Fprintln(os.Stderr, "❌ Укажите -slave ID")
		os.Exit(1)
	}
}

func dieOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
}

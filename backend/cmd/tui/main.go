// Интерактивный TUI для управления HLTNC T3D через Modbus RTU.
//
// Запуск:
//
//	go run cmd/tui/main.go
//	go run cmd/tui/main.go -slave 1 -port /dev/ttyUSB0
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
)

const (
	tickNormal   = 250 * time.Millisecond
	tickRotating = 50 * time.Millisecond

	pulsesPerRev = 10000 // 2500 линий × 4
)

// ── Стили ─────────────────────────────────────────────────────────────────────

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("4")).Padding(0, 1)
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	valStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	okStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	errStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	keyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("8")).Padding(0, 0)
	hintStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	inputStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("0")).Padding(0, 1)
	barDoneStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	barTodoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// ── UI режимы ─────────────────────────────────────────────────────────────────

type uiMode int

const (
	modeNormal uiMode = iota
	modePulseInput
	modeRotating
)

// ── Модель ────────────────────────────────────────────────────────────────────

type model struct {
	drv   *t3d.Driver
	slave int
	port  string

	status  *t3d.Status
	readErr error
	cfg     *t3d.Config

	servoOn  bool
	speedSpt int

	// поворот по импульсам
	mode            uiMode
	inputBuf        string
	rotStartPos     int64
	rotTargetPulses int64

	info    string
	infoErr bool
}

type (
	tickMsg        struct{ fast bool }
	fetchResultMsg struct {
		s   *t3d.Status
		err error
	}
	cfgResultMsg struct {
		c   *t3d.Config
		err error
	}
	rotStartedMsg struct {
		startPos int64
		err      error
	}
)

func newModel(drv *t3d.Driver, slave int, port string) model {
	cfg, _ := drv.ReadConfig()
	speedSpt := 0
	if cfg != nil {
		speedSpt = int(cfg.InternalSpd1)
	}
	return model{
		drv:      drv,
		slave:    slave,
		port:     port,
		cfg:      cfg,
		speedSpt: speedSpt,
		info:     "F=Авто-настройка  E=Servo ON  стрелки=скорость  P=поворот по импульсам",
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(doFetch(m.drv), doTick(false))
}

func doTick(fast bool) tea.Cmd {
	interval := tickNormal
	if fast {
		interval = tickRotating
	}
	return tea.Tick(interval, func(_ time.Time) tea.Msg { return tickMsg{fast: fast} })
}

func doFetch(drv *t3d.Driver) tea.Cmd {
	return func() tea.Msg {
		s, err := drv.ReadStatus()
		return fetchResultMsg{s, err}
	}
}

func doReadCfg(drv *t3d.Driver) tea.Cmd {
	return func() tea.Msg {
		c, err := drv.ReadConfig()
		return cfgResultMsg{c, err}
	}
}

func doStartRotation(drv *t3d.Driver, pulses int64, speedRPM int) tea.Cmd {
	return func() tea.Msg {
		actualSpeed := speedRPM
		if pulses < 0 {
			actualSpeed = -speedRPM
		}
		if err := drv.SetSpeedPreset(actualSpeed, true); err != nil {
			return rotStartedMsg{err: err}
		}
		// Читаем стартовую позицию уже после запуска (мотор только начинает разгон)
		st, err := drv.ReadStatus()
		if err != nil {
			return rotStartedMsg{err: err}
		}
		return rotStartedMsg{startPos: int64(st.Position32)}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case fetchResultMsg:
		m.readErr = msg.err
		if msg.err == nil {
			m.status = msg.s
			// Проверяем достижение цели при вращении
			if m.mode == modeRotating {
				traveled := int64(m.status.Position32) - m.rotStartPos
				if abs64(traveled) >= abs64(m.rotTargetPulses) {
					if err := m.drv.ServoDisable(); err != nil {
						m.setErr("Стоп: " + err.Error())
					} else {
						m.servoOn = false
						m.setOK(fmt.Sprintf("✓ Поворот завершён: пройдено %+d имп (%.2f об)",
							traveled, float64(abs64(traveled))/pulsesPerRev))
					}
					m.mode = modeNormal
				}
			}
		}
		return m, nil

	case cfgResultMsg:
		if msg.err == nil {
			m.cfg = msg.c
		}
		return m, nil

	case rotStartedMsg:
		if msg.err != nil {
			m.setErr("Поворот: " + msg.err.Error())
			m.mode = modeNormal
			return m, doTick(false)
		}
		m.rotStartPos = msg.startPos
		m.servoOn = true
		m.mode = modeRotating
		m.setOK(fmt.Sprintf("Поворот начат: %+d имп  |  Esc = стоп", m.rotTargetPulses))
		return m, doTick(true)

	case tickMsg:
		return m, tea.Batch(doFetch(m.drv), doTick(m.mode == modeRotating))

	case tea.KeyMsg:
		switch m.mode {

		// ── Ввод импульсов ───────────────────────────────────────────────────
		case modePulseInput:
			switch msg.String() {
			case "esc":
				m.mode = modeNormal
				m.inputBuf = ""
				m.setOK("Отмена")
			case "enter":
				cmd := m.confirmPulseInput()
				return m, cmd
			case "backspace":
				if len(m.inputBuf) > 0 {
					m.inputBuf = m.inputBuf[:len(m.inputBuf)-1]
				}
			default:
				ch := msg.String()
				// принимаем только цифры и минус (минус только первым символом)
				if ch == "-" && m.inputBuf == "" {
					m.inputBuf += ch
				} else if len(ch) == 1 && ch >= "0" && ch <= "9" {
					m.inputBuf += ch
				}
			}
			return m, nil

		// ── Вращение в процессе ──────────────────────────────────────────────
		case modeRotating:
			if msg.String() == "esc" || msg.String() == "q" {
				if err := m.drv.ServoDisable(); err != nil {
					m.setErr("Стоп: " + err.Error())
				} else {
					m.servoOn = false
					curPos := int64(0)
					if m.status != nil {
						curPos = int64(m.status.Position32)
					}
					traveled := curPos - m.rotStartPos
					m.setOK(fmt.Sprintf("Отменено на %+d имп", traveled))
				}
				m.mode = modeNormal
			}
			return m, nil

		// ── Нормальный режим ─────────────────────────────────────────────────
		default:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit

			case "e":
				if err := m.drv.ServoEnable(); err != nil {
					m.setErr("ServoEnable: " + err.Error())
				} else {
					m.servoOn = true
					m.setOK("Servo включён")
				}

			case "d":
				if err := m.drv.ServoDisable(); err != nil {
					m.setErr("ServoDisable: " + err.Error())
				} else {
					m.servoOn = false
					m.setOK("Servo выключен")
				}

			case "f":
				spd := m.speedSpt
				if spd == 0 {
					spd = 300
				}
				if err := m.drv.SetupSpeedMode(spd); err != nil {
					m.setErr("Setup: " + err.Error())
					return m, nil
				}
				m.speedSpt = spd
				m.servoOn = true
				if err := m.drv.SaveEEPROM(); err != nil {
					m.setErr("Setup OK, EEPROM: " + err.Error())
				} else {
					m.setOK("Режим скорости настроен и сохранён в EEPROM")
				}
				return m, doReadCfg(m.drv)

			case "r":
				return m, doReadCfg(m.drv)

			case "up", "+":
				m.speedSpt = clamp(m.speedSpt+100, -3000, 3000)
				m.applySpeed()
			case "down", "-":
				m.speedSpt = clamp(m.speedSpt-100, -3000, 3000)
				m.applySpeed()
			case "pgup":
				m.speedSpt = clamp(m.speedSpt+500, -3000, 3000)
				m.applySpeed()
			case "pgdown":
				m.speedSpt = clamp(m.speedSpt-500, -3000, 3000)
				m.applySpeed()
			case "0":
				m.speedSpt = 0
				m.applySpeed()

			case "p":
				m.inputBuf = ""
				m.mode = modePulseInput
				m.setOK("Введите число импульсов (+ = вперёд, - = реверс), Enter = старт")

			case "s":
				if err := m.drv.SaveEEPROM(); err != nil {
					m.setErr("SaveEEPROM: " + err.Error())
				} else {
					m.setOK("Параметры сохранены в EEPROM")
				}
			}
			return m, nil
		}
	}
	return m, nil
}

func (m *model) applySpeed() {
	if err := m.drv.SetSpeedPreset(m.speedSpt, m.servoOn); err != nil {
		m.setErr("Speed: " + err.Error())
	} else {
		m.setOK(fmt.Sprintf("Скорость -> %+d об/мин", m.speedSpt))
	}
}

func (m *model) confirmPulseInput() (cmd tea.Cmd) {
	pulses, err := strconv.ParseInt(strings.TrimSpace(m.inputBuf), 10, 64)
	if err != nil || pulses == 0 {
		m.setErr("Неверный ввод — введите целое число импульсов")
		m.mode = modeNormal
		m.inputBuf = ""
		return nil
	}
	speed := m.speedSpt
	if speed < 0 {
		speed = -speed
	}
	if speed == 0 {
		speed = 100
	}
	m.rotTargetPulses = pulses
	m.inputBuf = ""
	m.mode = modeNormal // временно, до подтверждения rotStartedMsg
	m.setOK(fmt.Sprintf("Запускаю поворот %+d имп @ %d об/мин...", pulses, speed))
	return doStartRotation(m.drv, pulses, speed)
}

func (m *model) setOK(s string)  { m.info = s; m.infoErr = false }
func (m *model) setErr(s string) { m.info = s; m.infoErr = true }

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func progressBar(pct, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := width * pct / 100
	empty := width - filled
	return barDoneStyle.Render(strings.Repeat("█", filled)) +
		barTodoStyle.Render(strings.Repeat("░", empty))
}

// ── Рендер ────────────────────────────────────────────────────────────────────

func (m model) View() string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n\n", titleStyle.Render(
		fmt.Sprintf(" T3D Control  |  Slave %d  |  %s ", m.slave, m.port),
	))

	// ─ Статус ─────────────────────────────────────────────────────────────
	fmt.Fprintf(&b, "%s\n", sectionStyle.Render("  СТАТУС"))

	switch {
	case m.readErr != nil:
		fmt.Fprintf(&b, "  %s\n", errStyle.Render("Нет связи: "+m.readErr.Error()))
	case m.status == nil:
		fmt.Fprintf(&b, "  %s\n", warnStyle.Render("Ожидание ответа..."))
	default:
		s := m.status
		lbl := func(name string) string {
			return labelStyle.Render(fmt.Sprintf("  %-14s: ", name))
		}
		val := func(format string, args ...any) string {
			return valStyle.Render(fmt.Sprintf(format, args...))
		}
		fmt.Fprintf(&b, "%s%s%s\n",
			lbl("Скорость"),
			val("%+d об/мин", s.SpeedRPM),
			dimStyle.Render(fmt.Sprintf("  (задание %+d)", m.speedSpt)),
		)
		fmt.Fprintf(&b, "%s%s%s\n",
			lbl("Позиция"),
			val("%d", s.Position32),
			dimStyle.Render(fmt.Sprintf(" имп  (%.3f об)", float64(s.Position32)/pulsesPerRev)),
		)
		fmt.Fprintf(&b, "%s%s   %s%s\n",
			lbl("Момент"), val("%d %%", s.TorquePct),
			labelStyle.Render("Ток: "), val("%.1f А", float64(s.CurrentA10)/10),
		)
		fmt.Fprintf(&b, "%s%s   %s%s\n",
			lbl("Т° радиатора"), val("%d °C", s.HeatsinkTempC),
			labelStyle.Render("Т° модуля: "), val("%d °C", s.ModuleTempC),
		)
		faultVal := okStyle.Render("0  (OK)")
		if s.FaultCode != 0 {
			faultVal = errStyle.Render(fmt.Sprintf("E%d", s.FaultCode))
		}
		fmt.Fprintf(&b, "%s%s   %s%s\n",
			lbl("Шина DC"), val("%d В", s.BusVoltageV),
			labelStyle.Render("Авария: "), faultVal,
		)
		fmt.Fprintf(&b, "%s%s%s%s\n",
			lbl("DI / DO"),
			val("0b%08b", s.DIState),
			dimStyle.Render(" / "),
			val("0b%06b", s.DOState),
		)
	}

	b.WriteString("\n")

	// ─ Поворот по импульсам ───────────────────────────────────────────────
	switch m.mode {

	case modePulseInput:
		fmt.Fprintf(&b, "%s\n", sectionStyle.Render("  ПОВОРОТ ПО ИМПУЛЬСАМ"))
		fmt.Fprintf(&b, "  %s\n", dimStyle.Render(
			fmt.Sprintf("1 оборот = %d имп  |  скорость: %d об/мин  |  Esc = отмена",
				pulsesPerRev, func() int {
					s := m.speedSpt
					if s < 0 {
						s = -s
					}
					if s == 0 {
						s = 100
					}
					return s
				}()),
		))
		b.WriteString("\n")
		cursor := "_"
		fmt.Fprintf(&b, "  %s %s\n",
			labelStyle.Render("Импульсы:"),
			inputStyle.Render(m.inputBuf+cursor),
		)
		b.WriteString("\n")
		fmt.Fprintf(&b, "  %s  %s  %s\n",
			keyStyle.Render("[0..9]")+" "+hintStyle.Render("цифра"),
			keyStyle.Render("[-]")+" "+hintStyle.Render("реверс"),
			keyStyle.Render("[Enter]")+" "+hintStyle.Render("старт"),
		)
		b.WriteString("\n")

	case modeRotating:
		traveled := int64(0)
		if m.status != nil {
			traveled = int64(m.status.Position32) - m.rotStartPos
		}
		total := m.rotTargetPulses
		pct := 0
		if total != 0 {
			pct = int(abs64(traveled) * 100 / abs64(total))
			if pct > 100 {
				pct = 100
			}
		}

		fmt.Fprintf(&b, "%s  %s\n",
			sectionStyle.Render("  ПОВОРОТ"),
			errStyle.Render("[Esc = стоп]"),
		)
		fmt.Fprintf(&b, "  %s%s%s\n",
			labelStyle.Render("Цель:     "),
			valStyle.Render(fmt.Sprintf("%+d имп", total)),
			dimStyle.Render(fmt.Sprintf("  (%.3f об)", float64(abs64(total))/pulsesPerRev)),
		)
		fmt.Fprintf(&b, "  %s%s%s\n",
			labelStyle.Render("Пройдено: "),
			valStyle.Render(fmt.Sprintf("%+d имп", traveled)),
			dimStyle.Render(fmt.Sprintf("  %d%%", pct)),
		)
		fmt.Fprintf(&b, "  [%s] %d%%\n\n", progressBar(pct, 40), pct)

	default:
		// ─ Конфигурация ───────────────────────────────────────────────────
		fmt.Fprintf(&b, "%s\n", sectionStyle.Render("  КОНФИГУРАЦИЯ  [F=авто-настройка  R=обновить]"))

		if m.cfg == nil {
			fmt.Fprintf(&b, "  %s\n", warnStyle.Render("Не прочитана — нажмите R"))
		} else {
			c := m.cfg
			mark := func(ok bool) string {
				if ok {
					return okStyle.Render("[OK]")
				}
				return errStyle.Render("[!!]")
			}
			modeOK := c.ControlMode == t3d.ModeSpeed
			srcOK := c.SpeedSource == t3d.SpeedSourceInternal
			sonOK := c.ServoOnMode == 1
			di1OK := c.DI1Func == 10

			fmt.Fprintf(&b, "  %s P-004=%s   %s P-098=%s\n",
				mark(modeOK), valStyle.Render(modeLabel(c.ControlMode)),
				mark(sonOK), valStyle.Render(servoOnLabel(c.ServoOnMode)),
			)
			fmt.Fprintf(&b, "  %s P-025=%s   %s P-100=%s\n",
				mark(srcOK), valStyle.Render(speedSrcLabel(c.SpeedSource)),
				mark(di1OK), valStyle.Render(di1Label(c.DI1Func)),
			)
			if !modeOK || !srcOK || !sonOK || !di1OK {
				fmt.Fprintf(&b, "  %s\n", warnStyle.Render("Параметры не настроены — нажмите F"))
			}
		}
		b.WriteString("\n")
	}

	// ─ Управление ─────────────────────────────────────────────────────────
	if m.mode == modeNormal {
		fmt.Fprintf(&b, "%s\n", sectionStyle.Render("  УПРАВЛЕНИЕ"))
		servoStr := errStyle.Render("○ OFF")
		if m.servoOn {
			servoStr = okStyle.Render("● ON ")
		}
		modeStr := "?"
		if m.cfg != nil {
			modeStr = modeLabel(m.cfg.ControlMode)
		}
		fmt.Fprintf(&b, "  Servo: %s    Режим: %s    Задание: %s\n\n",
			servoStr,
			valStyle.Render(modeStr),
			valStyle.Render(fmt.Sprintf("%+d об/мин", m.speedSpt)),
		)

		type hint struct{ k, d string }
		row1 := []hint{{"E", "Servo ON"}, {"D", "Servo OFF"}, {"↑/↓", "±100 об/мин"}, {"PgUp/Dn", "±500 об/мин"}}
		row2 := []hint{{"0", "Стоп"}, {"P", "Поворот имп"}, {"F", "Авто-настройка"}, {"S", "EEPROM"}, {"Q", "Выход"}}
		renderHints := func(hints []hint) string {
			var parts []string
			for _, h := range hints {
				parts = append(parts, keyStyle.Render("["+h.k+"]")+" "+hintStyle.Render(h.d))
			}
			return "  " + strings.Join(parts, "   ")
		}
		fmt.Fprintf(&b, "%s\n%s\n\n", renderHints(row1), renderHints(row2))
	}

	// ─ Последнее действие ─────────────────────────────────────────────────
	if m.infoErr {
		fmt.Fprintf(&b, "  %s\n", errStyle.Render(m.info))
	} else {
		fmt.Fprintf(&b, "  %s\n", hintStyle.Render(m.info))
	}

	return b.String()
}

// ── Метки ─────────────────────────────────────────────────────────────────────

func modeLabel(mode uint16) string {
	switch mode {
	case t3d.ModePosition:
		return "Позиция (0)"
	case t3d.ModeSpeed:
		return "Скорость (1)"
	case t3d.ModeTorque:
		return "Момент (2)"
	default:
		return fmt.Sprintf("Режим %d", mode)
	}
}

func servoOnLabel(v uint16) string {
	switch v {
	case 0:
		return "Внешний SON (0)"
	case 1:
		return "Всегда ON (1)"
	default:
		return fmt.Sprintf("%d", v)
	}
}

func speedSrcLabel(v uint16) string {
	switch v {
	case 0:
		return "Аналог (0)"
	case 1:
		return "Внутр.пресет (1)"
	case 2:
		return "Аналог+Пресет (2)"
	case 3:
		return "Импульс (3)"
	default:
		return fmt.Sprintf("%d", v)
	}
}

func di1Label(v uint16) string {
	switch v {
	case 1:
		return "SON (1)"
	case 10:
		return "SP1 (10)"
	case 11:
		return "SP2 (11)"
	default:
		return fmt.Sprintf("ф-ция %d", v)
	}
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	port := flag.String("port", t3d.DefaultPort, "RS-485 порт")
	baud := flag.Int("baud", t3d.DefaultBaud, "Baud rate")
	slave := flag.Int("slave", int(t3d.DefaultSlaveID), "Slave ID")
	flag.Parse()

	drv := t3d.New(*port, *baud, byte(*slave))
	if err := drv.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "Проверьте:")
		fmt.Fprintln(os.Stderr, "  ls /dev/ttyUSB*              -- виден ли адаптер?")
		fmt.Fprintln(os.Stderr, "  sudo usermod -aG dialout $USER -- права на порт?")
		os.Exit(1)
	}
	defer drv.Close()

	prog := tea.NewProgram(newModel(drv, *slave, *port), tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
